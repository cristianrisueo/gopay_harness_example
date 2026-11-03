package staking

import (
	"context"
	"fmt"
	"time"

	"github.com/cristianrisueo/gopay/internal/pool"
	"github.com/cristianrisueo/gopay/internal/wallet"
	"github.com/cristianrisueo/gopay/pkg/currency"
	"github.com/shopspring/decimal"
)

// Service defines the interface for staking operations
type Service interface {
	CreateStake(ctx context.Context, userID, amountStr string, curr currency.Currency) (*StakeResponse, error)
	IncreaseStake(ctx context.Context, userID, positionID, additionalAmountStr string) (*StakeResponse, error)
	GetUserPositions(ctx context.Context, userID string, status *PositionStatus) (*UserPositionsResponse, error)
	WithdrawStake(ctx context.Context, userID, positionID string) (*WithdrawResponse, error)
	GetAvailableBalance(ctx context.Context, userID string, curr currency.Currency) (decimal.Decimal, error)
}

// Type assertion to ensure StakingService implements Service
var _ Service = (*StakingService)(nil)

// StakingService handles business logic for staking operations
type StakingService struct {
	stakingRepo   Repository
	walletService wallet.Service
	poolService   pool.Service
}

// NewService creates a new StakingService
func NewService(
	stakingRepo Repository,
	walletService wallet.Service,
	poolService pool.Service,
) *StakingService {
	return &StakingService{
		stakingRepo:   stakingRepo,
		walletService: walletService,
		poolService:   poolService,
	}
}

// CreateStake creates a new staking position for a user
func (ss *StakingService) CreateStake(
	ctx context.Context,
	userID, amountStr string,
	curr currency.Currency,
) (*StakeResponse, error) {
	// Parses amount to decimal
	amount, err := decimal.NewFromString(amountStr)
	if err != nil {
		return nil, ErrInvalidAmount
	}

	// Validates amount >= 1000 (business rule)
	minAmount := decimal.NewFromInt(1000)
	if amount.LessThan(minAmount) {
		return nil, ErrInvalidAmount
	}

	// Validates currency
	if !curr.IsValid() {
		return nil, ErrInvalidCurrency
	}

	// Gets user's available balance (total - staked) for that currency
	availableBalance, err := ss.GetAvailableBalance(ctx, userID, curr)
	if err != nil {
		return nil, fmt.Errorf("failed to get available balance: %w", err)
	}

	// Checks if user has sufficient available balance
	if amount.GreaterThan(availableBalance) {
		return nil, ErrInsufficientBalance
	}

	// Gets the current APY from that currency's liquidity pool
	apyRate, err := ss.poolService.GetCurrentAPY(ctx, convertCurrencyToPoolCurrency(curr))
	if err != nil {
		return nil, fmt.Errorf("failed to get current APY: %w", err)
	}

	// Calculates the lockup end (1 year from now)
	lockupEnd := time.Now().AddDate(1, 0, 0)

	// Creates staking position and updates pool liquidity atomically
	position, err := ss.stakingRepo.CreatePosition(ctx, userID, curr, amount, apyRate, lockupEnd)
	if err != nil {
		return nil, fmt.Errorf("failed to create staking position with liquidity: %w", err)
	}

	return &StakeResponse{
		PositionID: position.ID,
		Amount:     position.Amount.String(),
		Currency:   position.Currency,
		APYRate:    position.APYRate.String(),
		LockupEnd:  position.LockupEnd,
		Status:     position.Status,
		CreatedAt:  position.CreatedAt,
	}, nil
}

// IncreaseStake increases the amount of an existing staking position
func (ss *StakingService) IncreaseStake(
	ctx context.Context,
	userID, positionID, additionalAmountStr string,
) (*StakeResponse, error) {
	// Parses the amount to decimal
	additionalAmount, err := decimal.NewFromString(additionalAmountStr)
	if err != nil {
		return nil, ErrInvalidAmount
	}

	// Validates the amount > 0
	if additionalAmount.LessThanOrEqual(decimal.Zero) {
		return nil, ErrInvalidAmount
	}

	// Gets the existing position
	position, err := ss.stakingRepo.GetPositionByID(ctx, positionID)
	if err != nil {
		return nil, err
	}

	// Validates that the position belongs to the user
	if position.UserID != userID {
		return nil, ErrUnauthorized
	}

	// Validate the position is active
	if position.Status != StatusActive {
		return nil, ErrPositionNotActive
	}

	// Gets user's available balance
	availableBalance, err := ss.GetAvailableBalance(ctx, userID, position.Currency)
	if err != nil {
		return nil, fmt.Errorf("failed to get available balance: %w", err)
	}

	// Checks if the user has sufficient available balance to update the position
	if additionalAmount.GreaterThan(availableBalance) {
		return nil, ErrInsufficientBalance
	}

	// Update position amount and pool liquidity atomically
	err = ss.stakingRepo.UpdatePositionAmount(ctx, positionID, additionalAmount, position.Currency)
	if err != nil {
		return nil, fmt.Errorf("failed to update position amount with liquidity: %w", err)
	}

	// Retrieves the updated position
	updatedPosition, err := ss.stakingRepo.GetPositionByID(ctx, positionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get updated position: %w", err)
	}

	return &StakeResponse{
		PositionID: updatedPosition.ID,
		Amount:     updatedPosition.Amount.String(),
		Currency:   updatedPosition.Currency,
		APYRate:    updatedPosition.APYRate.String(),
		LockupEnd:  updatedPosition.LockupEnd,
		Status:     updatedPosition.Status,
		CreatedAt:  updatedPosition.CreatedAt,
	}, nil
}

// GetUserPositions retrieves all staking positions for a user and the total staked
func (ss *StakingService) GetUserPositions(
	ctx context.Context,
	userID string,
	status *PositionStatus,
) (*UserPositionsResponse, error) {
	// Gets all positions of the user (status = nil means all statuses)
	positions, err := ss.stakingRepo.GetPositionsByUser(ctx, userID, status)
	if err != nil {
		return nil, fmt.Errorf("failed to get user positions: %w", err)
	}

	// Builds a PositionDetail slice of the len of retrieved rows
	positionDetails := make([]PositionDetail, 0, len(positions))
	for _, pos := range positions {
		// Calculate if the user can withdraw that position
		canWithdraw := pos.Status == StatusActive && time.Now().After(pos.LockupEnd)

		// Calculate the remaining days for withdrawing the position
		daysRemaining := 0
		if pos.Status == StatusActive && time.Now().Before(pos.LockupEnd) {
			daysRemaining = int(time.Until(pos.LockupEnd).Hours() / 24)
		}

		// Creates a position details struct with the data retrieved
		positionDetails = append(positionDetails, PositionDetail{
			ID:                  pos.ID,
			Currency:            pos.Currency,
			Amount:              pos.Amount.String(),
			AccumulatedInterest: pos.AccumulatedInterest.String(),
			APYRate:             pos.APYRate.String(),
			LockupEnd:           pos.LockupEnd,
			Status:              pos.Status,
			CanWithdraw:         canWithdraw,
			DaysRemaining:       daysRemaining,
			CreatedAt:           pos.CreatedAt,
		})
	}

	// Calculates the total amount staked by currency
	totalEUR, err := ss.stakingRepo.GetTotalStakedByUser(ctx, userID, currency.EUR)
	if err != nil {
		return nil, fmt.Errorf("failed to get total staked EUR: %w", err)
	}

	totalUSD, err := ss.stakingRepo.GetTotalStakedByUser(ctx, userID, currency.USD)
	if err != nil {
		return nil, fmt.Errorf("failed to get total staked USD: %w", err)
	}

	totalJPY, err := ss.stakingRepo.GetTotalStakedByUser(ctx, userID, currency.JPY)
	if err != nil {
		return nil, fmt.Errorf("failed to get total staked JPY: %w", err)
	}

	return &UserPositionsResponse{
		Positions: positionDetails,
		TotalStaked: TotalStaked{
			EUR: totalEUR.String(),
			USD: totalUSD.String(),
			JPY: totalJPY.String(),
		},
	}, nil
}

// WithdrawStake withdraws a staking position after lockup period
func (ss *StakingService) WithdrawStake(
	ctx context.Context,
	userID, positionID string,
) (*WithdrawResponse, error) {
	// Gets the position
	position, err := ss.stakingRepo.GetPositionByID(ctx, positionID)
	if err != nil {
		return nil, err
	}

	// Validates that the position belongs to user
	if position.UserID != userID {
		return nil, ErrUnauthorized
	}

	// Validates that the position is active
	if position.Status != StatusActive {
		return nil, ErrPositionNotActive
	}

	// Validate the lockup period has passed
	if time.Now().Before(position.LockupEnd) {
		return nil, ErrStakingLocked
	}

	// Calculates total to return (principal + accumulated interest)
	totalToReturn := position.Amount.Add(position.AccumulatedInterest)

	// Withdraws position, updates pool liquidity, and deposits to wallet atomically
	err = ss.stakingRepo.WithdrawPosition(
		ctx,
		positionID,
		position.Amount,
		totalToReturn,
		position.Currency,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to withdraw position with liquidity: %w", err)
	}

	// Gets the updated wallet balance of the user
	balanceResponse, err := ss.walletService.GetBalance(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get updated balance: %w", err)
	}

	// Gets the balance for the specific currency to return the response
	var newBalance string

	switch position.Currency {
	case currency.EUR:
		newBalance = balanceResponse.EUR.String()
	case currency.USD:
		newBalance = balanceResponse.USD.String()
	case currency.JPY:
		newBalance = balanceResponse.JPY.String()
	}

	return &WithdrawResponse{
		WithdrawnAmount:  totalToReturn.String(),
		Principal:        position.Amount.String(),
		Interest:         position.AccumulatedInterest.String(),
		NewWalletBalance: newBalance,
	}, nil
}

// GetAvailableBalance returns the available balance for a user for a currency
// Note: Staked funds are already deducted from wallet, so available = wallet balance
func (ss *StakingService) GetAvailableBalance(
	ctx context.Context,
	userID string,
	curr currency.Currency,
) (decimal.Decimal, error) {
	// Gets the wallet balance
	balanceResponse, err := ss.walletService.GetBalance(ctx, userID)
	if err != nil {
		return decimal.Zero, fmt.Errorf("failed to get wallet balance: %w", err)
	}

	// Gets the balance of that specific currency
	var availableBalance decimal.Decimal
	switch curr {
	case currency.EUR:
		availableBalance = balanceResponse.EUR
	case currency.USD:
		availableBalance = balanceResponse.USD
	case currency.JPY:
		availableBalance = balanceResponse.JPY
	default:
		return decimal.Zero, ErrInvalidCurrency
	}

	// Returns the wallet balance (staked funds are already deducted from wallet)
	return availableBalance, nil
}

// convertCurrencyToPoolCurrency converts currency.Currency to pool.PoolCurrency
func convertCurrencyToPoolCurrency(c currency.Currency) pool.PoolCurrency {
	return pool.PoolCurrency(string(c))
}
