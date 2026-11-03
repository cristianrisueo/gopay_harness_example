package credit

import (
	"context"
	"fmt"
	"time"

	"github.com/cristianrisueo/gopay/internal/pool"
	"github.com/cristianrisueo/gopay/internal/user"
	"github.com/cristianrisueo/gopay/internal/wallet"
	"github.com/cristianrisueo/gopay/pkg/currency"
	"github.com/cristianrisueo/gopay/pkg/llm"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// Service defines the interface for credit operations
type Service interface {
	RequestCredit(ctx context.Context, userID string, req *CreditRequest) (*Credit, error)
	GetCredit(ctx context.Context, userID, creditID string) (*Credit, error)
	ListMyCredits(ctx context.Context, userID string, status *CreditStatus) ([]*Credit, error)
	AcceptCredit(ctx context.Context, userID, creditID string) error
	MakePayment(ctx context.Context, userID, creditID string) (*Payment, error)
	GetCreditHistory(ctx context.Context, userID string) (*CreditHistory, error)
}

// Type assertion to ensure CreditService implements Service
var _ Service = (*CreditService)(nil)

// CreditService handles credit business logic
type CreditService struct {
	creditRepo    Repository
	userRepo      user.Repository
	walletService wallet.Service
	poolService   pool.Service
	llmClient     *llm.Client
	logger        *zap.Logger
}

// NewService creates a new CreditService instance
func NewService(
	creditRepo Repository,
	userRepo user.Repository,
	walletService wallet.Service,
	poolService pool.Service,
	llmClient *llm.Client,
	logger *zap.Logger,
) *CreditService {
	return &CreditService{
		creditRepo:    creditRepo,
		userRepo:      userRepo,
		walletService: walletService,
		poolService:   poolService,
		llmClient:     llmClient,
		logger:        logger,
	}
}

// RequestCredit handles a new credit request with LLM evaluation
func (cs *CreditService) RequestCredit(
	ctx context.Context,
	userID string,
	req *CreditRequest,
) (*Credit, error) {
	// Parses amount to decimal
	amount, err := decimal.NewFromString(req.Amount)
	if err != nil {
		return nil, ErrInvalidAmount
	}

	// Validates amount is positive
	if amount.LessThanOrEqual(decimal.Zero) {
		return nil, ErrInvalidAmount
	}

	// Validates currency
	curr := currency.Currency(req.Currency)
	if !curr.IsValid() {
		return nil, ErrInvalidCurrency
	}

	// Validates credit duration
	if req.DurationMonths < 1 || req.DurationMonths > 60 {
		return nil, ErrInvalidAmount
	}

	// Validates user exists and is active
	userProfile, err := cs.userRepo.GetByID(ctx, userID)
	if err != nil {
		if err == user.ErrUserNotFound {
			return nil, ErrUserNotFound
		}

		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	if userProfile.DeletedAt != nil {
		return nil, ErrUserNotFound
	}

	// Checks if pool has sufficient liquidity
	poolStats, err := cs.poolService.GetPoolStatsByCurrency(ctx, pool.PoolCurrency(curr))
	if err != nil {
		return nil, fmt.Errorf("failed to get pool stats: %w", err)
	}

	// Parses pool stats strings to decimals
	availableLiquidity, err := decimal.NewFromString(poolStats.AvailableLiquidity)
	if err != nil {
		return nil, fmt.Errorf("failed to parse pool liquidity: %w", err)
	}

	totalLiquidity, err := decimal.NewFromString(poolStats.TotalLiquidity)
	if err != nil {
		return nil, fmt.Errorf("failed to parse total liquidity: %w", err)
	}

	utilizationRate, err := decimal.NewFromString(poolStats.UtilizationRate)
	if err != nil {
		return nil, fmt.Errorf("failed to parse utilization rate: %w", err)
	}

	if amount.GreaterThan(availableLiquidity) {
		return nil, ErrInsufficientLiquidity
	}

	// Gets or creates user credit history
	creditHistory, err := cs.creditRepo.GetUserCreditHistory(ctx, userID)
	if err != nil {
		// If not found, create new credit history
		creditHistory, err = cs.creditRepo.CreateCreditHistory(ctx, userID)
		if err != nil {
			return nil, fmt.Errorf("failed to create credit history: %w", err)
		}
	}

	// Validates user has no defaulted credits
	if creditHistory.CreditsDefaulted > 0 {
		return nil, ErrUserHasDefaultedCredit
	}

	// Validates user has < 3 active credits
	if creditHistory.ActiveCreditsCount >= 3 {
		return nil, ErrMaxActiveCreditsExceeded
	}

	// Gets user wallet balances
	walletBalance, err := cs.walletService.GetBalance(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get wallet balance: %w", err)
	}

	// Calculate account age in days
	accountAgeDays := int(time.Since(userProfile.CreatedAt).Hours() / 24)

	// Prepares LLM evaluation request
	llmRequest := &llm.CreditEvaluationRequest{
		Request: llm.CreditRequest{
			Amount:         amount.InexactFloat64(),
			Currency:       string(curr),
			DurationMonths: req.DurationMonths,
			Purpose:        req.Purpose,
		},
		UserProfile: llm.UserProfile{
			UserID:           userID,
			Username:         userProfile.Username,
			Email:            userProfile.Email,
			RegistrationDate: userProfile.CreatedAt,
			AccountAgeDays:   accountAgeDays,
		},
		CreditHistory: llm.CreditHistory{
			CreditsAsked:       creditHistory.CreditsAsked,
			CreditsPaid:        creditHistory.CreditsPaid,
			CreditsDefaulted:   creditHistory.CreditsDefaulted,
			TotalBorrowed:      creditHistory.TotalBorrowed.InexactFloat64(),
			TotalRepaid:        creditHistory.TotalRepaid.InexactFloat64(),
			OnTimePaymentRate:  creditHistory.OnTimePaymentRate.InexactFloat64(),
			ActiveCreditsCount: creditHistory.ActiveCreditsCount,
		},
		FinancialStatus: llm.FinancialStatus{
			WalletBalanceEUR: walletBalance.EUR.InexactFloat64(),
			WalletBalanceUSD: walletBalance.USD.InexactFloat64(),
			WalletBalanceJPY: walletBalance.JPY.InexactFloat64(),
		},
		PoolStatus: llm.PoolStatus{
			Currency:           string(curr),
			TotalLiquidity:     totalLiquidity.InexactFloat64(),
			AvailableLiquidity: availableLiquidity.InexactFloat64(),
			UtilizationRate:    utilizationRate.InexactFloat64(),
		},
	}

	// Calls LLM for credit evaluation
	llmResponse, err := cs.llmClient.EvaluateCredit(ctx, llmRequest)
	if err != nil {
		cs.logger.Error("LLM evaluation failed", zap.Error(err), zap.String("user_id", userID))
		return nil, ErrLLMEvaluation
	}

	// Determines credit status based on LLM decision
	var creditStatus CreditStatus
	var riskLevel *RiskLevel
	var rejectionReason *string
	var apr decimal.Decimal
	var collateralPercent decimal.Decimal

	if llmResponse.Decision == "APPROVED" {
		creditStatus = StatusPending

		// Gets risk level and terms from LLM response
		if llmResponse.CreditTerms != nil {
			risk := RiskLevel(llmResponse.CreditTerms.RiskLevel)
			riskLevel = &risk

			// Parses APR and collateral percentage from LLM response
			apr = decimal.NewFromFloat(llmResponse.CreditTerms.APR)
			collateralPercent = decimal.NewFromFloat(llmResponse.CreditTerms.CollateralRequiredPercentage)
		}
	} else {
		creditStatus = StatusRejected
		rejectionReason = llmResponse.RejectionReason
		apr = decimal.Zero
		collateralPercent = decimal.Zero
	}

	// Creates the credit record
	newCredit := &Credit{
		UserID:            userID,
		Amount:            amount,
		Currency:          curr,
		Status:            creditStatus,
		APR:               apr,
		DurationMonths:    req.DurationMonths,
		RepaymentType:     RepaymentTypeAmortized,
		CollateralPercent: collateralPercent,
		CollateralLocked:  decimal.Zero, // Will be locked when credit is accepted
		RejectionReason:   rejectionReason,
		RiskLevel:         riskLevel,
		LLMResponse:       llmResponse,
	}

	credit, err := cs.creditRepo.CreateCredit(ctx, newCredit)
	if err != nil {
		return nil, fmt.Errorf("failed to create credit: %w", err)
	}

	// Updates the reviewed_at timestamp after LLM evaluation
	err = cs.creditRepo.UpdateCreditReviewed(
		ctx,
		credit.ID,
		credit.Status,
		credit.RiskLevel,
		credit.LLMResponse,
		credit.RejectionReason,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to update reviewed_at: %w", err)
	}

	// Gets the updated credit with reviewed_at timestamp
	updatedCredit, err := cs.creditRepo.GetCreditByID(ctx, credit.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get updated credit: %w", err)
	}

	cs.logger.Info("Credit request processed",
		zap.String("credit_id", updatedCredit.ID),
		zap.String("user_id", userID),
		zap.String("status", string(creditStatus)),
	)

	return updatedCredit, nil
}

// GetCredit retrieves a credit by ID, verifying ownership
func (cs *CreditService) GetCredit(
	ctx context.Context,
	userID, creditID string,
) (*Credit, error) {
	// Gets the credit by ID
	credit, err := cs.creditRepo.GetCreditByID(ctx, creditID)
	if err != nil {
		if err == ErrCreditNotFound {
			return nil, ErrCreditNotFound
		}

		return nil, fmt.Errorf("failed to get credit: %w", err)
	}

	// Verifies ownership of the credit
	if credit.UserID != userID {
		return nil, ErrCreditNotFound
	}

	return credit, nil
}

// ListMyCredits lists all credits for a user with optional status filter
func (cs *CreditService) ListMyCredits(
	ctx context.Context,
	userID string,
	status *CreditStatus,
) ([]*Credit, error) {
	credits, err := cs.creditRepo.ListCreditsByUserID(ctx, userID, status)
	if err != nil {
		return nil, fmt.Errorf("failed to list credits: %w", err)
	}

	return credits, nil
}

// AcceptCredit accepts an approved credit with collateral and payment schedule
func (cs *CreditService) AcceptCredit(
	ctx context.Context,
	userID, creditID string,
) error {
	// Gets and validates credit
	credit, err := cs.creditRepo.GetCreditByID(ctx, creditID)
	if err != nil {
		if err == ErrCreditNotFound {
			return ErrCreditNotFound
		}
		return fmt.Errorf("failed to get credit: %w", err)
	}

	// Verifies ownership
	if credit.UserID != userID {
		return ErrCreditNotFound
	}

	// Validates credit status is PENDING
	if credit.Status != StatusPending {
		return ErrCreditNotApproved
	}

	// Validates LLM response exists with payment schedule
	if credit.LLMResponse == nil {
		return ErrLLMResponse
	}

	// Type assertion for LLMResponse
	llmResp, ok := credit.LLMResponse.(*llm.CreditEvaluationResponse)
	if !ok || llmResp == nil {
		return ErrLLMResponse
	}

	if llmResp.RepaymentSchedule == nil || len(llmResp.RepaymentSchedule) == 0 {
		return ErrLLMResponse
	}

	if llmResp.CreditTerms == nil {
		return ErrLLMResponse
	}

	// Parses payment schedule from LLM response
	paymentSchedule := make([]Payment, 0, len(llmResp.RepaymentSchedule))
	for _, item := range llmResp.RepaymentSchedule {
		// Parses due date
		dueDate, err := time.Parse("2006-01-02", item.DueDate)
		if err != nil {
			return fmt.Errorf("failed to parse due date: %w", err)
		}

		payment := Payment{
			ID:              uuid.New().String(),
			CreditID:        credit.ID,
			PaymentNumber:   item.PaymentNumber,
			PrincipalAmount: decimal.NewFromFloat(item.PrincipalAmount),
			InterestAmount:  decimal.NewFromFloat(item.InterestAmount),
			TotalAmount:     decimal.NewFromFloat(item.TotalAmount),
			DueDate:         dueDate,
			Status:          PaymentPending,
		}
		paymentSchedule = append(paymentSchedule, payment)
	}

	// Calculates collateral amount with exchange rate if needed
	collateralCurrency := currency.Currency(llmResp.CreditTerms.CollateralCurrency)
	if !collateralCurrency.IsValid() {
		return ErrInvalidCurrency
	}

	collateralAmount := decimal.NewFromFloat(llmResp.CreditTerms.CollateralAmount)

	// Validates user has sufficient balance for collateral
	walletBalance, err := cs.walletService.GetBalance(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get wallet balance: %w", err)
	}

	var availableCollateral decimal.Decimal
	switch collateralCurrency {
	case currency.EUR:
		availableCollateral = walletBalance.EUR
	case currency.USD:
		availableCollateral = walletBalance.USD
	case currency.JPY:
		availableCollateral = walletBalance.JPY
	default:
		return ErrInvalidCurrency
	}

	if collateralAmount.GreaterThan(availableCollateral) {
		return ErrInsufficientCollateral
	}

	// Validates pool still has sufficient liquidity
	poolStats, err := cs.poolService.GetPoolStatsByCurrency(ctx, pool.PoolCurrency(credit.Currency))
	if err != nil {
		return fmt.Errorf("failed to get pool stats: %w", err)
	}

	availableLiquidity, err := decimal.NewFromString(poolStats.AvailableLiquidity)
	if err != nil {
		return fmt.Errorf("failed to parse pool liquidity: %w", err)
	}

	if credit.Amount.GreaterThan(availableLiquidity) {
		return ErrInsufficientLiquidity
	}

	// Creates collateral record
	collateral := &Collateral{
		ID:       uuid.New().String(),
		CreditID: credit.ID,
		UserID:   userID,
		Currency: collateralCurrency,
		Amount:   collateralAmount,
	}

	// Calls repository transaction to accept credit atomically
	err = cs.creditRepo.AcceptCreditTx(ctx, credit, paymentSchedule, collateral)
	if err != nil {
		cs.logger.Error("Failed to accept credit",
			zap.Error(err),
			zap.String("credit_id", creditID),
			zap.String("user_id", userID),
		)
		return err
	}

	cs.logger.Info("Credit accepted successfully",
		zap.String("credit_id", creditID),
		zap.String("user_id", userID),
		zap.String("amount", credit.Amount.String()),
	)

	return nil
}

// MakePayment processes a credit payment
func (cs *CreditService) MakePayment(
	ctx context.Context,
	userID, creditID string,
) (*Payment, error) {
	// Gets and validates credit
	credit, err := cs.creditRepo.GetCreditByID(ctx, creditID)
	if err != nil {
		if err == ErrCreditNotFound {
			return nil, ErrCreditNotFound
		}
		return nil, fmt.Errorf("failed to get credit: %w", err)
	}

	// Verifies ownership
	if credit.UserID != userID {
		return nil, ErrCreditNotFound
	}

	// Validates credit status is ACTIVE
	if credit.Status != StatusActive {
		return nil, ErrCreditNotApproved
	}

	// Gets the next pending payment
	payment, err := cs.creditRepo.GetNextPendingPayment(ctx, creditID)
	if err != nil {
		if err == ErrPaymentNotDue {
			return nil, ErrPaymentNotDue
		}
		return nil, fmt.Errorf("failed to get pending payment: %w", err)
	}

	// Validates user has sufficient balance in credit currency
	walletBalance, err := cs.walletService.GetBalance(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get wallet balance: %w", err)
	}

	var availableBalance decimal.Decimal
	switch credit.Currency {
	case currency.EUR:
		availableBalance = walletBalance.EUR
	case currency.USD:
		availableBalance = walletBalance.USD
	case currency.JPY:
		availableBalance = walletBalance.JPY
	default:
		return nil, ErrInvalidCurrency
	}

	if payment.TotalAmount.GreaterThan(availableBalance) {
		return nil, ErrInsufficientBalance
	}

	// Calls repository transaction to process payment atomically
	err = cs.creditRepo.MakePaymentTx(ctx, payment, credit)
	if err != nil {
		cs.logger.Error("Failed to make payment",
			zap.Error(err),
			zap.String("credit_id", creditID),
			zap.String("user_id", userID),
			zap.String("payment_id", payment.ID),
		)
		return nil, err
	}

	// Updates payment status to return
	now := time.Now()
	payment.Status = PaymentPaid
	payment.PaidDate = &now

	cs.logger.Info("Payment processed successfully",
		zap.String("credit_id", creditID),
		zap.String("user_id", userID),
		zap.String("payment_id", payment.ID),
		zap.Int("payment_number", payment.PaymentNumber),
		zap.String("amount", payment.TotalAmount.String()),
	)

	return payment, nil
}

// GetCreditHistory retrieves the credit history for a user
func (cs *CreditService) GetCreditHistory(
	ctx context.Context,
	userID string,
) (*CreditHistory, error) {
	history, err := cs.creditRepo.GetUserCreditHistory(ctx, userID)
	if err != nil {
		if err == ErrCreditHistoryNotFound {
			return nil, ErrCreditHistoryNotFound
		}

		return nil, fmt.Errorf("failed to get credit history: %w", err)
	}

	return history, nil
}
