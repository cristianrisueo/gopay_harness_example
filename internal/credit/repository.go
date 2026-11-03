package credit

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/cristianrisueo/gopay/pkg/currency"
	"github.com/cristianrisueo/gopay/pkg/llm"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Repository defines the interface for credit data access
type Repository interface {
	// Credit operations
	CreateCredit(ctx context.Context, credit *Credit) (*Credit, error)
	GetCreditByID(ctx context.Context, creditID string) (*Credit, error)
	ListCreditsByUserID(ctx context.Context, userID string, status *CreditStatus) ([]*Credit, error)
	UpdateCreditStatus(ctx context.Context, creditID string, status CreditStatus) error

	UpdateCreditReviewed(
		ctx context.Context,
		creditID string,
		status CreditStatus,
		riskLevel *RiskLevel,
		llmResponse any,
		rejectionReason *string,
	) error

	// Credit history operations
	CreateCreditHistory(ctx context.Context, userID string) (*CreditHistory, error)
	GetUserCreditHistory(ctx context.Context, userID string) (*CreditHistory, error)
	UpdateCreditHistory(ctx context.Context, history *CreditHistory) error

	// Payment operations
	CreatePayment(ctx context.Context, payment *Payment) (*Payment, error)
	GetNextPendingPayment(ctx context.Context, creditID string) (*Payment, error)
	GetPaymentByID(ctx context.Context, paymentID string) (*Payment, error)
	ListPaymentsByCreditID(ctx context.Context, creditID string) ([]*Payment, error)
	UpdatePaymentStatus(ctx context.Context, paymentID string, status PaymentStatus, paidDate *time.Time) error

	// Collateral operations
	CreateCollateral(ctx context.Context, collateral *Collateral) (*Collateral, error)
	GetCollateralsByUserID(ctx context.Context, userID string) ([]*Collateral, error)
	GetCollateralsByCreditID(ctx context.Context, creditID string) ([]*Collateral, error)
	ReleaseCollateral(ctx context.Context, creditID string, releaseTime time.Time) error

	// Atomic operations
	AcceptCreditTx(ctx context.Context, credit *Credit, paymentSchedule []Payment, collateral *Collateral) error
	MakePaymentTx(ctx context.Context, payment *Payment, credit *Credit) error
}

// PostgresRepository implements Repository interface
type PostgresRepository struct {
	db *sql.DB
}

// NewPostgresRepository creates a new PostgreSQL repository
func NewPostgresRepository(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

// Type assertion to ensure interface compliance
var _ Repository = (*PostgresRepository)(nil)

//* Credits logic

// CreateCredit inserts a new credit record
func (pr *PostgresRepository) CreateCredit(
	ctx context.Context,
	credit *Credit,
) (*Credit, error) {
	// Checks if credit ID and dates are provided or sets them
	if credit.ID == "" {
		credit.ID = uuid.New().String()
	}

	now := time.Now().UTC()

	if credit.RequestedAt.IsZero() {
		credit.RequestedAt = now
	}

	if credit.CreatedAt.IsZero() {
		credit.CreatedAt = now
	}

	if credit.UpdatedAt.IsZero() {
		credit.UpdatedAt = now
	}

	// Serialize LLMResponse to JSON
	var llmResponseJSON []byte
	var err error
	if credit.LLMResponse != nil {
		llmResponseJSON, err = json.Marshal(credit.LLMResponse)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal LLM response: %w", err)
		}
	}

	// Creates the query
	query := `
		INSERT INTO credits (
			id, user_id, amount, currency, status, apr, duration_months, repayment_type,
			collateral_percentage, collateral_locked, rejection_reason, risk_level,
			requested_at, reviewed_at, given_at, completed_at, llm_response, created_at, updated_at
		)
		VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
			$11, $12, $13, $14, $15, $16, $17, $18, $19
		)
		RETURNING
			id, user_id, amount, currency, status, apr, duration_months, repayment_type,
			collateral_percentage, collateral_locked, rejection_reason, risk_level,
			requested_at, reviewed_at, given_at, completed_at, llm_response, created_at, updated_at
	`

	// Executes the query and returns the scanned row to a Credit struct
	row := pr.db.QueryRowContext(ctx, query,
		credit.ID,
		credit.UserID,
		credit.Amount.String(),
		credit.Currency,
		credit.Status,
		credit.APR.String(),
		credit.DurationMonths,
		credit.RepaymentType,
		credit.CollateralPercent.String(),
		credit.CollateralLocked.String(),
		credit.RejectionReason,
		credit.RiskLevel,
		credit.RequestedAt,
		credit.ReviewedAt,
		credit.GivenAt,
		credit.CompletedAt,
		llmResponseJSON,
		credit.CreatedAt,
		credit.UpdatedAt,
	)

	return pr.scanCredit(row)
}

// GetCreditByID retrieves a credit by ID
func (pr *PostgresRepository) GetCreditByID(
	ctx context.Context,
	creditID string,
) (*Credit, error) {
	query := `
		SELECT 
			id, user_id, amount, currency, status, apr, duration_months, repayment_type,
			collateral_percentage, collateral_locked, rejection_reason, risk_level,
			requested_at, reviewed_at, given_at, completed_at, llm_response, created_at, updated_at
		FROM 
			credits
		WHERE 
			id = $1
	`

	row := pr.db.QueryRowContext(ctx, query, creditID)
	return pr.scanCredit(row)
}

// ListCreditsByUserID retrieves all credits by userID, optionally filtered by status
func (pr *PostgresRepository) ListCreditsByUserID(
	ctx context.Context,
	userID string,
	status *CreditStatus,
) ([]*Credit, error) {
	// Query and arguments variable
	var query string
	var args []any

	query = `
		SELECT 
			id, user_id, amount, currency, status, apr, duration_months, repayment_type,
			collateral_percentage, collateral_locked, rejection_reason, risk_level,
			requested_at, reviewed_at, given_at, completed_at, llm_response, created_at, updated_at
		FROM 
			credits
		WHERE 
			user_id = $1
	`

	// Appends the userID and status if provided to arguments
	args = append(args, userID)

	if status != nil {
		query += ` AND status = $2`
		args = append(args, *status)
	}

	query += ` ORDER BY requested_at DESC`

	// Executes the query
	rows, err := pr.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query credits: %w", err)
	}

	defer rows.Close()

	// Maps every retrieved row into a Credit struct
	var credits []*Credit
	for rows.Next() {
		credit, err := pr.scanCreditRow(rows)
		if err != nil {
			return nil, err
		}

		credits = append(credits, credit)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating credits: %w", err)
	}

	return credits, nil
}

// UpdateCreditStatus updates the status of a credit
func (pr *PostgresRepository) UpdateCreditStatus(
	ctx context.Context,
	creditID string,
	status CreditStatus,
) error {
	// Creates the query
	query := `
		UPDATE credits
		SET status = $1, updated_at = $2
		WHERE id = $3
	`

	// Executes the query
	result, err := pr.db.ExecContext(ctx, query, status, time.Now().UTC(), creditID)
	if err != nil {
		return fmt.Errorf("failed to update credit status: %w", err)
	}

	// Checks if any credit has been updated
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get affected rows: %w", err)
	}

	if rows == 0 {
		return ErrCreditNotFound
	}

	return nil
}

// UpdateCreditReviewed updates credit after LLM review
func (pr *PostgresRepository) UpdateCreditReviewed(
	ctx context.Context,
	creditID string,
	status CreditStatus,
	riskLevel *RiskLevel,
	llmResponse any,
	rejectionReason *string,
) error {
	// Marshals the LLM response to JSON
	var llmResponseJSON []byte
	var err error
	if llmResponse != nil {
		llmResponseJSON, err = json.Marshal(llmResponse)
		if err != nil {
			return fmt.Errorf("failed to marshal LLM response: %w", err)
		}
	}

	// Creates the query
	query := `
		UPDATE
			credits
		SET
			status = $1, risk_level = $2, llm_response = $3,
			rejection_reason = $4, reviewed_at = $5, updated_at = $6
		WHERE
			id = $7
	`

	now := time.Now().UTC()

	// Executes the query
	result, err := pr.db.ExecContext(
		ctx,
		query,
		status,
		riskLevel,
		llmResponseJSON,
		rejectionReason,
		now,
		now,
		creditID,
	)

	if err != nil {
		return fmt.Errorf("failed to update credit review: %w", err)
	}

	// Checks if any credit row has been modified
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get affected rows: %w", err)
	}

	if rows == 0 {
		return ErrCreditNotFound
	}

	return nil
}

//* Credits history logic

// CreateCreditHistory creates initial credit history for a user
func (pr *PostgresRepository) CreateCreditHistory(
	ctx context.Context,
	userID string,
) (*CreditHistory, error) {
	// Creates the query
	query := `
		INSERT INTO credit_history (
			id, user_id, credits_asked, credits_paid, credits_defaulted, total_amount_borrowed,
			total_amount_repaid, on_time_payment_rate, active_credits_count, first_credit_date, 
			last_default_date, credit_score, created_at, updated_at
		)
		VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14
		)
		RETURNING 
			id, user_id, credits_asked, credits_paid, credits_defaulted, total_amount_borrowed,
			total_amount_repaid, on_time_payment_rate, active_credits_count, first_credit_date, 
			last_default_date, credit_score, created_at, updated_at
	`

	// Executes the query and returns the row scanned to a CreditHistory struct
	row := pr.db.QueryRowContext(ctx, query,
		uuid.New().String(),
		userID,
		1,                                // credits_asked
		0,                                // credits_paid
		0,                                // credits_defaulted
		decimal.Zero.String(),            // total_amount_borrowed
		decimal.Zero.String(),            // total_amount_repaid
		decimal.NewFromInt(100).String(), // on_time_payment_rate (100%)
		0,                                // active_credits_count
		time.Now().UTC(),                 // first_credit_date
		nil,                              // last_default_date
		nil,                              // credit_score
		time.Now().UTC(),
		time.Now().UTC(),
	)

	return pr.scanCreditHistory(row)
}

// GetUserCreditHistory retrieves credit history for a user
func (pr *PostgresRepository) GetUserCreditHistory(
	ctx context.Context,
	userID string,
) (*CreditHistory, error) {
	// Creates the query, executes it and returns the row scanned to a CreditHistory struct
	query := `
		SELECT 
			id, user_id, credits_asked, credits_paid, credits_defaulted, total_amount_borrowed,
			total_amount_repaid, on_time_payment_rate, active_credits_count, first_credit_date, 
			last_default_date, credit_score, created_at, updated_at
		FROM 
			credit_history
		WHERE 
			user_id = $1
	`

	row := pr.db.QueryRowContext(ctx, query, userID)
	return pr.scanCreditHistory(row)
}

// UpdateCreditHistory updates a user credit's history data
func (pr *PostgresRepository) UpdateCreditHistory(
	ctx context.Context,
	history *CreditHistory,
) error {
	// Cretes the query
	query := `
		UPDATE 
			credit_history
		SET 
			credits_asked = $1, credits_paid = $2, credits_defaulted = $3, total_amount_borrowed = $4,
			total_amount_repaid = $5, on_time_payment_rate = $6, active_credits_count = $7,
			first_credit_date = $8, last_default_date = $9, credit_score = $10, updated_at = $11
		WHERE 
			user_id = $12
	`

	// Checks if credit score is provided
	var creditScore any
	if history.CreditScore != nil {
		creditScore = history.CreditScore.String()
	}

	// Executes the query
	result, err := pr.db.ExecContext(ctx, query,
		history.CreditsAsked,
		history.CreditsPaid,
		history.CreditsDefaulted,
		history.TotalBorrowed.String(),
		history.TotalRepaid.String(),
		history.OnTimePaymentRate.String(),
		history.ActiveCreditsCount,
		history.FirstCreditDate,
		history.LastDefaultDate,
		creditScore,
		time.Now().UTC(),
		history.UserID,
	)

	if err != nil {
		return fmt.Errorf("failed to update credit history: %w", err)
	}

	// Checks if the query has modified any record
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get affected rows: %w", err)
	}

	if rows == 0 {
		return ErrCreditHistoryNotFound
	}

	return nil
}

//* Payments logic

// CreatePayment inserts a payment record
func (pr *PostgresRepository) CreatePayment(
	ctx context.Context,
	payment *Payment,
) (*Payment, error) {
	// Checks if payment ID and dates are provided or sets them
	if payment.ID == "" {
		payment.ID = uuid.New().String()
	}

	now := time.Now().UTC()

	if payment.CreatedAt.IsZero() {
		payment.CreatedAt = now
	}

	if payment.UpdatedAt.IsZero() {
		payment.UpdatedAt = now
	}

	// Creates the query
	query := `
		INSERT INTO credit_payments (
			id, credit_id, payment_number, principal_amount, interest_amount,
			total_amount, due_date, paid_date, status, created_at, updated_at
		)
		VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
		)
		RETURNING 
			id, credit_id, payment_number, principal_amount, interest_amount,
			total_amount, due_date, paid_date, status, created_at, updated_at
	`

	// Executes the query and returns the row mapped to a Payment struct
	row := pr.db.QueryRowContext(ctx, query,
		payment.ID,
		payment.CreditID,
		payment.PaymentNumber,
		payment.PrincipalAmount.String(),
		payment.InterestAmount.String(),
		payment.TotalAmount.String(),
		payment.DueDate,
		payment.PaidDate,
		payment.Status,
		payment.CreatedAt,
		payment.UpdatedAt,
	)

	return pr.scanPayment(row)
}

// GetNextPendingPayment retrieves the next pending payment for a credit
func (pr *PostgresRepository) GetNextPendingPayment(
	ctx context.Context,
	creditID string,
) (*Payment, error) {
	// Executes the query and returns the retrieved data mapped to a Payment struct
	query := `
		SELECT 
			id, credit_id, payment_number, principal_amount, interest_amount,
			total_amount, due_date, paid_date, status, created_at, updated_at
		FROM 
			credit_payments
		WHERE 
			credit_id = $1 AND status = $2
		ORDER BY payment_number ASC 
		LIMIT 1
	`

	row := pr.db.QueryRowContext(ctx, query, creditID, PaymentPending)
	return pr.scanPayment(row)
}

// GetPaymentByID retrieves a payment by ID
func (pr *PostgresRepository) GetPaymentByID(
	ctx context.Context,
	paymentID string,
) (*Payment, error) {
	// Executes the query and returns the retrieved data mapped to a Payment struct
	query := `
		SELECT 
			id, credit_id, payment_number, principal_amount, interest_amount,
			total_amount, due_date, paid_date, status, created_at, updated_at
		FROM 
			credit_payments
		WHERE 
			id = $1
	`

	row := pr.db.QueryRowContext(ctx, query, paymentID)
	return pr.scanPayment(row)
}

// ListPaymentsByCreditID retrieves all payments for a credit
func (pr *PostgresRepository) ListPaymentsByCreditID(
	ctx context.Context,
	creditID string,
) ([]*Payment, error) {
	// Creates the query
	query := `
		SELECT 
			id, credit_id, payment_number, principal_amount, interest_amount,
			total_amount, due_date, paid_date, status, created_at, updated_at
		FROM 
			credit_payments
		WHERE 
			credit_id = $1
		ORDER BY payment_number ASC
	`

	// Executes the query
	rows, err := pr.db.QueryContext(ctx, query, creditID)
	if err != nil {
		return nil, fmt.Errorf("failed to query payments: %w", err)
	}

	defer rows.Close()

	// Slice of payments
	var payments []*Payment

	// Scans every retrieved row into a Payment struct and adds it to the payments slice
	for rows.Next() {
		payment, err := pr.scanPaymentRow(rows)
		if err != nil {
			return nil, err
		}

		payments = append(payments, payment)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating payments: %w", err)
	}

	return payments, nil
}

// UpdatePaymentStatus updates a payment's status
func (pr *PostgresRepository) UpdatePaymentStatus(
	ctx context.Context,
	paymentID string,
	status PaymentStatus,
	paidDate *time.Time,
) error {
	// Creates the query
	query := `
		UPDATE 
			credit_payments
		SET 
			status = $1, paid_date = $2, updated_at = $3
		WHERE id = $4
	`

	// Executes the query
	result, err := pr.db.ExecContext(
		ctx,
		query,
		status,
		paidDate,
		time.Now().UTC(),
		paymentID,
	)

	if err != nil {
		return fmt.Errorf("failed to update payment status: %w", err)
	}

	// Checks if any payment row has been updated
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get affected rows: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("payment not found")
	}

	return nil
}

//* Collateral operations

// CreateCollateral inserts a collateral record
func (pr *PostgresRepository) CreateCollateral(
	ctx context.Context,
	collateral *Collateral,
) (*Collateral, error) {
	// Checks if the collateral ID and dates are provided
	if collateral.ID == "" {
		collateral.ID = uuid.New().String()
	}

	now := time.Now().UTC()

	if collateral.LockedAt.IsZero() {
		collateral.LockedAt = now
	}

	if collateral.CreatedAt.IsZero() {
		collateral.CreatedAt = now
	}

	if collateral.UpdatedAt.IsZero() {
		collateral.UpdatedAt = now
	}

	// Creates the query
	query := `
		INSERT INTO credit_collateral (
			id, credit_id, user_id, currency, amount, 
			locked_at, released_at, created_at, updated_at
		)
		VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9
		)
		RETURNING 
			id, credit_id, user_id, currency, amount, 
			locked_at, released_at, created_at, updated_at
	`

	// Executes the query and returns the row mapped to Collateral struct
	row := pr.db.QueryRowContext(
		ctx,
		query,
		collateral.ID,
		collateral.CreditID,
		collateral.UserID,
		collateral.Currency,
		collateral.Amount.String(),
		collateral.LockedAt,
		collateral.ReleasedAt,
		collateral.CreatedAt,
		collateral.UpdatedAt,
	)

	return pr.scanCollateral(row)
}

// GetCollateralsByUserID retrieves blocked collateral of a user.
// If released_at != null is not collateral anymore
func (pr *PostgresRepository) GetCollateralsByUserID(
	ctx context.Context,
	userID string,
) ([]*Collateral, error) {
	// Creates the query
	query := `
		SELECT 
			id, credit_id, user_id, currency, amount, 
			locked_at, released_at, created_at, updated_at
		FROM 
			credit_collateral
		WHERE 
			user_id = $1 AND released_at IS NULL
	`

	// Executes the query
	rows, err := pr.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query collateral: %w", err)
	}

	defer rows.Close()

	// Slice of collateral
	var collaterals []*Collateral

	// Maps each retrieved row into a Collateral struct and adds it to the slice
	for rows.Next() {
		collateral, err := pr.scanCollateralRow(rows)
		if err != nil {
			return nil, err
		}

		collaterals = append(collaterals, collateral)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating collateral: %w", err)
	}

	return collaterals, nil
}

// GetCollateralsByCreditID retrieves all the collateral for a credit
func (pr *PostgresRepository) GetCollateralsByCreditID(
	ctx context.Context,
	creditID string,
) ([]*Collateral, error) {
	// Creates the query
	query := `
		SELECT 
			id, credit_id, user_id, currency, amount, 
			locked_at, released_at, created_at, updated_at
		FROM 
			credit_collateral
		WHERE 
			credit_id = $1
	`

	// Executes the query
	rows, err := pr.db.QueryContext(ctx, query, creditID)
	if err != nil {
		return nil, fmt.Errorf("failed to query collateral: %w", err)
	}

	defer rows.Close()

	// Slice of collateral
	var collaterals []*Collateral

	// Maps each retrieved row into a Collateral struct and adds it to the slice
	for rows.Next() {
		collateral, err := pr.scanCollateralRow(rows)
		if err != nil {
			return nil, err
		}

		collaterals = append(collaterals, collateral)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating collateral: %w", err)
	}

	return collaterals, nil
}

// ReleaseCollateral updates the collateral as released
func (pr *PostgresRepository) ReleaseCollateral(
	ctx context.Context,
	creditID string,
	releaseTime time.Time,
) error {
	// Creates the query
	query := `
		UPDATE 
			credit_collateral
		SET 
			released_at = $1, updated_at = $2
		WHERE 
			credit_id = $3 AND released_at IS NULL
	`

	// Executes the query
	result, err := pr.db.ExecContext(
		ctx,
		query,
		releaseTime,
		time.Now().UTC(),
		creditID,
	)

	if err != nil {
		return fmt.Errorf("failed to release collateral: %w", err)
	}

	// Checks if any row has been affected
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get affected rows: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("no collateral to release")
	}

	return nil
}

//* ACID transactions

// AcceptCreditTx atomically accepts a credit with all necessary updates
func (pr *PostgresRepository) AcceptCreditTx(
	ctx context.Context,
	credit *Credit,
	paymentSchedule []Payment,
	collateral *Collateral,
) error {
	// Begins the DB transaction
	tx, err := pr.db.BeginTx(ctx, &sql.TxOptions{
		Isolation: sql.LevelReadCommitted,
	})
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	// Ensures rollback if anything fails
	defer func() {
		if err != nil {
			if rollbackErr := tx.Rollback(); rollbackErr != nil {
				err = fmt.Errorf("%w (rollback failed: %v)", err, rollbackErr)
			}
		}
	}()

	// 1. Locks and re-validates credit status
	var creditStatus string
	creditQuery := `
		SELECT status FROM credits WHERE id = $1 FOR UPDATE
	`
	err = tx.QueryRowContext(ctx, creditQuery, credit.ID).Scan(&creditStatus)
	if err != nil {
		if err == sql.ErrNoRows {
			return ErrCreditNotFound
		}
		return fmt.Errorf("failed to lock credit: %w", err)
	}

	if creditStatus != string(StatusPending) {
		return ErrCreditNotApproved
	}

	// 2. Locks wallet and validates collateral balance
	var balanceEUR, balanceUSD, balanceJPY string
	walletQuery := `
		SELECT balance_eur, balance_usd, balance_jpy
		FROM wallets
		WHERE user_id = $1
		FOR UPDATE
	`
	err = tx.QueryRowContext(ctx, walletQuery, credit.UserID).Scan(&balanceEUR, &balanceUSD, &balanceJPY)
	if err != nil {
		return fmt.Errorf("failed to lock wallet: %w", err)
	}

	// Gets collateral balance based on collateral currency
	var collateralBalanceStr string
	switch collateral.Currency {
	case currency.EUR:
		collateralBalanceStr = balanceEUR
	case currency.USD:
		collateralBalanceStr = balanceUSD
	case currency.JPY:
		collateralBalanceStr = balanceJPY
	default:
		return ErrInvalidCurrency
	}

	collateralBalance, err := decimal.NewFromString(collateralBalanceStr)
	if err != nil {
		return fmt.Errorf("failed to parse collateral balance: %w", err)
	}

	if collateralBalance.LessThan(collateral.Amount) {
		return ErrInsufficientCollateral
	}

	// 3. Locks pool and validates available liquidity
	var poolID string
	var totalLiquidity, totalUsed string
	poolQuery := `
		SELECT id, total_liquidity, total_used
		FROM liquidity_pools
		WHERE currency = $1
		FOR UPDATE
	`
	err = tx.QueryRowContext(ctx, poolQuery, string(credit.Currency)).Scan(&poolID, &totalLiquidity, &totalUsed)
	if err != nil {
		return fmt.Errorf("failed to lock pool: %w", err)
	}

	totalLiq, _ := decimal.NewFromString(totalLiquidity)
	totalU, _ := decimal.NewFromString(totalUsed)
	availableLiquidity := totalLiq.Sub(totalU)

	if credit.Amount.GreaterThan(availableLiquidity) {
		return ErrInsufficientLiquidity
	}

	// 4. Updates credit status to ACTIVE
	updateCreditQuery := `
		UPDATE credits
		SET status = $1, given_at = NOW(), updated_at = NOW()
		WHERE id = $2
	`
	_, err = tx.ExecContext(ctx, updateCreditQuery, string(StatusActive), credit.ID)
	if err != nil {
		return fmt.Errorf("failed to update credit status: %w", err)
	}

	// 5. Inserts payment schedule (batch)
	for _, payment := range paymentSchedule {
		paymentQuery := `
			INSERT INTO credit_payments (
				id, credit_id, payment_number, principal_amount, interest_amount,
				total_amount, due_date, status, created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW())
		`
		_, err = tx.ExecContext(ctx, paymentQuery,
			payment.ID,
			credit.ID,
			payment.PaymentNumber,
			payment.PrincipalAmount.String(),
			payment.InterestAmount.String(),
			payment.TotalAmount.String(),
			payment.DueDate,
			string(PaymentPending),
		)
		if err != nil {
			return fmt.Errorf("failed to insert payment %d: %w", payment.PaymentNumber, err)
		}
	}

	// 6. Inserts collateral record
	collateralQuery := `
		INSERT INTO credit_collateral (
			id, credit_id, user_id, currency, amount, locked_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, NOW(), NOW(), NOW())
	`
	_, err = tx.ExecContext(ctx, collateralQuery,
		collateral.ID,
		credit.ID,
		credit.UserID,
		string(collateral.Currency),
		collateral.Amount.String(),
	)
	if err != nil {
		return fmt.Errorf("failed to insert collateral: %w", err)
	}

	// 7. Deducts collateral from wallet
	var deductCollateralQuery string
	switch collateral.Currency {
	case currency.EUR:
		deductCollateralQuery = "UPDATE wallets SET balance_eur = balance_eur - $1, updated_at = NOW() WHERE user_id = $2"
	case currency.USD:
		deductCollateralQuery = "UPDATE wallets SET balance_usd = balance_usd - $1, updated_at = NOW() WHERE user_id = $2"
	case currency.JPY:
		deductCollateralQuery = "UPDATE wallets SET balance_jpy = balance_jpy - $1, updated_at = NOW() WHERE user_id = $2"
	}
	_, err = tx.ExecContext(ctx, deductCollateralQuery, collateral.Amount.String(), credit.UserID)
	if err != nil {
		return fmt.Errorf("failed to deduct collateral: %w", err)
	}

	// 8. Adds loan amount to wallet
	var addLoanQuery string
	switch credit.Currency {
	case currency.EUR:
		addLoanQuery = "UPDATE wallets SET balance_eur = balance_eur + $1, updated_at = NOW() WHERE user_id = $2"
	case currency.USD:
		addLoanQuery = "UPDATE wallets SET balance_usd = balance_usd + $1, updated_at = NOW() WHERE user_id = $2"
	case currency.JPY:
		addLoanQuery = "UPDATE wallets SET balance_jpy = balance_jpy + $1, updated_at = NOW() WHERE user_id = $2"
	}
	_, err = tx.ExecContext(ctx, addLoanQuery, credit.Amount.String(), credit.UserID)
	if err != nil {
		return fmt.Errorf("failed to add loan amount: %w", err)
	}

	// 9. Updates pool (allocates liquidity - marks as used)
	updatePoolQuery := `
		UPDATE liquidity_pools
		SET total_used = total_used + $1, updated_at = NOW()
		WHERE id = $2
	`
	_, err = tx.ExecContext(ctx, updatePoolQuery, credit.Amount.String(), poolID)
	if err != nil {
		return fmt.Errorf("failed to update pool: %w", err)
	}

	// 10. Updates credit history
	updateHistoryQuery := `
		UPDATE credit_history
		SET active_credits_count = active_credits_count + 1,
		    total_amount_borrowed = total_amount_borrowed + $1,
		    updated_at = NOW()
		WHERE user_id = $2
	`
	_, err = tx.ExecContext(ctx, updateHistoryQuery, credit.Amount.String(), credit.UserID)
	if err != nil {
		return fmt.Errorf("failed to update credit history: %w", err)
	}

	// Commits the transaction
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// MakePaymentTx atomically processes a payment with all necessary updates
func (pr *PostgresRepository) MakePaymentTx(
	ctx context.Context,
	payment *Payment,
	credit *Credit,
) error {
	// Begins the DB transaction
	tx, err := pr.db.BeginTx(ctx, &sql.TxOptions{
		Isolation: sql.LevelReadCommitted,
	})
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	// Ensures rollback if anything fails
	defer func() {
		if err != nil {
			if rollbackErr := tx.Rollback(); rollbackErr != nil {
				err = fmt.Errorf("%w (rollback failed: %v)", err, rollbackErr)
			}
		}
	}()

	// 1. Locks and re-validates payment status
	var paymentStatus string
	paymentQuery := `
		SELECT status FROM credit_payments WHERE id = $1 FOR UPDATE
	`
	err = tx.QueryRowContext(ctx, paymentQuery, payment.ID).Scan(&paymentStatus)
	if err != nil {
		if err == sql.ErrNoRows {
			return ErrPaymentNotDue
		}
		return fmt.Errorf("failed to lock payment: %w", err)
	}

	if paymentStatus != string(PaymentPending) {
		return ErrPaymentNotDue
	}

	// 2. Locks and re-validates credit status
	var creditStatus string
	creditQuery := `
		SELECT status FROM credits WHERE id = $1 FOR UPDATE
	`
	err = tx.QueryRowContext(ctx, creditQuery, credit.ID).Scan(&creditStatus)
	if err != nil {
		return fmt.Errorf("failed to lock credit: %w", err)
	}

	if creditStatus != string(StatusActive) {
		return ErrCreditNotApproved
	}

	// 3. Locks wallet and validates sufficient balance
	var balanceEUR, balanceUSD, balanceJPY string
	walletQuery := `
		SELECT balance_eur, balance_usd, balance_jpy
		FROM wallets
		WHERE user_id = $1
		FOR UPDATE
	`
	err = tx.QueryRowContext(ctx, walletQuery, credit.UserID).Scan(&balanceEUR, &balanceUSD, &balanceJPY)
	if err != nil {
		return fmt.Errorf("failed to lock wallet: %w", err)
	}

	var currentBalanceStr string
	switch credit.Currency {
	case currency.EUR:
		currentBalanceStr = balanceEUR
	case currency.USD:
		currentBalanceStr = balanceUSD
	case currency.JPY:
		currentBalanceStr = balanceJPY
	default:
		return ErrInvalidCurrency
	}

	currentBalance, err := decimal.NewFromString(currentBalanceStr)
	if err != nil {
		return fmt.Errorf("failed to parse balance: %w", err)
	}

	if currentBalance.LessThan(payment.TotalAmount) {
		return ErrInsufficientBalance
	}

	// 4. Locks pool
	var poolID string
	poolQuery := `
		SELECT id FROM liquidity_pools WHERE currency = $1 FOR UPDATE
	`
	err = tx.QueryRowContext(ctx, poolQuery, string(credit.Currency)).Scan(&poolID)
	if err != nil {
		return fmt.Errorf("failed to lock pool: %w", err)
	}

	// 5. Deducts payment from wallet
	var deductPaymentQuery string
	switch credit.Currency {
	case currency.EUR:
		deductPaymentQuery = "UPDATE wallets SET balance_eur = balance_eur - $1, updated_at = NOW() WHERE user_id = $2"
	case currency.USD:
		deductPaymentQuery = "UPDATE wallets SET balance_usd = balance_usd - $1, updated_at = NOW() WHERE user_id = $2"
	case currency.JPY:
		deductPaymentQuery = "UPDATE wallets SET balance_jpy = balance_jpy - $1, updated_at = NOW() WHERE user_id = $2"
	}
	_, err = tx.ExecContext(ctx, deductPaymentQuery, payment.TotalAmount.String(), credit.UserID)
	if err != nil {
		return fmt.Errorf("failed to deduct payment: %w", err)
	}

	// 6. Updates pool (returns principal to pool - interest stays as revenue)
	updatePoolQuery := `
		UPDATE liquidity_pools
		SET total_used = total_used - $1, updated_at = NOW()
		WHERE id = $2
	`
	_, err = tx.ExecContext(ctx, updatePoolQuery, payment.PrincipalAmount.String(), poolID)
	if err != nil {
		return fmt.Errorf("failed to update pool: %w", err)
	}

	// 7. Marks payment as PAID
	updatePaymentQuery := `
		UPDATE credit_payments
		SET status = $1, paid_date = NOW(), updated_at = NOW()
		WHERE id = $2
	`
	_, err = tx.ExecContext(ctx, updatePaymentQuery, string(PaymentPaid), payment.ID)
	if err != nil {
		return fmt.Errorf("failed to update payment status: %w", err)
	}

	// 8. Updates credit history (on-time payment rate will be calculated in service layer)
	updateHistoryQuery := `
		UPDATE credit_history
		SET total_amount_repaid = total_amount_repaid + $1, updated_at = NOW()
		WHERE user_id = $2
	`
	_, err = tx.ExecContext(ctx, updateHistoryQuery, payment.TotalAmount.String(), credit.UserID)
	if err != nil {
		return fmt.Errorf("failed to update credit history: %w", err)
	}

	// 9. Checks if this was the last payment
	var remainingPayments int
	countQuery := `
		SELECT COUNT(*) FROM credit_payments
		WHERE credit_id = $1 AND status = $2
	`
	err = tx.QueryRowContext(ctx, countQuery, credit.ID, string(PaymentPending)).Scan(&remainingPayments)
	if err != nil {
		return fmt.Errorf("failed to count remaining payments: %w", err)
	}

	// 10. If last payment: complete credit and release collateral
	if remainingPayments == 0 {
		// 10a. Mark credit as completed
		completeCreditQuery := `
			UPDATE credits
			SET status = $1, completed_at = NOW(), updated_at = NOW()
			WHERE id = $2
		`
		_, err = tx.ExecContext(ctx, completeCreditQuery, string(StatusCompleted), credit.ID)
		if err != nil {
			return fmt.Errorf("failed to complete credit: %w", err)
		}

		// 10b. Mark collateral as released
		releaseCollateralQuery := `
			UPDATE credit_collateral
			SET released_at = NOW(), updated_at = NOW()
			WHERE credit_id = $1 AND released_at IS NULL
		`
		_, err = tx.ExecContext(ctx, releaseCollateralQuery, credit.ID)
		if err != nil {
			return fmt.Errorf("failed to release collateral: %w", err)
		}

		// 10c. Get collateral details
		var collateralCurrency string
		var collateralAmount string
		collateralQuery := `
			SELECT currency, amount FROM credit_collateral WHERE credit_id = $1 LIMIT 1
		`
		err = tx.QueryRowContext(ctx, collateralQuery, credit.ID).Scan(&collateralCurrency, &collateralAmount)
		if err != nil {
			return fmt.Errorf("failed to get collateral details: %w", err)
		}

		// 10d. Return collateral to wallet
		var returnCollateralQuery string
		switch currency.Currency(collateralCurrency) {
		case currency.EUR:
			returnCollateralQuery = "UPDATE wallets SET balance_eur = balance_eur + $1, updated_at = NOW() WHERE user_id = $2"
		case currency.USD:
			returnCollateralQuery = "UPDATE wallets SET balance_usd = balance_usd + $1, updated_at = NOW() WHERE user_id = $2"
		case currency.JPY:
			returnCollateralQuery = "UPDATE wallets SET balance_jpy = balance_jpy + $1, updated_at = NOW() WHERE user_id = $2"
		}
		_, err = tx.ExecContext(ctx, returnCollateralQuery, collateralAmount, credit.UserID)
		if err != nil {
			return fmt.Errorf("failed to return collateral: %w", err)
		}

		// 10e. Update credit history (increment paid, decrement active)
		finalizeHistoryQuery := `
			UPDATE credit_history
			SET credits_paid = credits_paid + 1,
			    active_credits_count = active_credits_count - 1,
			    updated_at = NOW()
			WHERE user_id = $1
		`
		_, err = tx.ExecContext(ctx, finalizeHistoryQuery, credit.UserID)
		if err != nil {
			return fmt.Errorf("failed to finalize credit history: %w", err)
		}
	}

	// Commits the transaction
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

//* Helper functions to scan row data into application structs

// scanCredit scans a single credit from a database row
func (pr *PostgresRepository) scanCredit(row *sql.Row) (*Credit, error) {
	var (
		amountStr         string
		aprStr            string
		collateralPercStr string
		collateralLockStr string
		llmResponse       any
	)

	credit := &Credit{}
	err := row.Scan(
		&credit.ID,
		&credit.UserID,
		&amountStr,
		&credit.Currency,
		&credit.Status,
		&aprStr,
		&credit.DurationMonths,
		&credit.RepaymentType,
		&collateralPercStr,
		&collateralLockStr,
		&credit.RejectionReason,
		&credit.RiskLevel,
		&credit.RequestedAt,
		&credit.ReviewedAt,
		&credit.GivenAt,
		&credit.CompletedAt,
		&llmResponse,
		&credit.CreatedAt,
		&credit.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrCreditNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan credit: %w", err)
	}

	// Convert amounts from strings to decimal.Decimal
	amount, err := decimal.NewFromString(amountStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse amount: %w", err)
	}
	credit.Amount = amount

	apr, err := decimal.NewFromString(aprStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse apr: %w", err)
	}
	credit.APR = apr

	collateralPerc, err := decimal.NewFromString(collateralPercStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse collateral percentage: %w", err)
	}
	credit.CollateralPercent = collateralPerc

	collateralLock, err := decimal.NewFromString(collateralLockStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse collateral locked: %w", err)
	}
	credit.CollateralLocked = collateralLock

	// Deserialize LLM response from JSON if present
	if llmResponse != nil {
		var llmResp llm.CreditEvaluationResponse
		if jsonBytes, ok := llmResponse.([]byte); ok {
			if err := json.Unmarshal(jsonBytes, &llmResp); err != nil {
				return nil, fmt.Errorf("failed to unmarshal LLM response: %w", err)
			}
			credit.LLMResponse = &llmResp
		} else {
			credit.LLMResponse = llmResponse
		}
	}

	return credit, nil
}

// scanCreditRow scans a credit from a rows iterator
func (pr *PostgresRepository) scanCreditRow(rows *sql.Rows) (*Credit, error) {
	var (
		amountStr         string
		aprStr            string
		collateralPercStr string
		collateralLockStr string
		llmResponse       any
	)

	credit := &Credit{}
	err := rows.Scan(
		&credit.ID,
		&credit.UserID,
		&amountStr,
		&credit.Currency,
		&credit.Status,
		&aprStr,
		&credit.DurationMonths,
		&credit.RepaymentType,
		&collateralPercStr,
		&collateralLockStr,
		&credit.RejectionReason,
		&credit.RiskLevel,
		&credit.RequestedAt,
		&credit.ReviewedAt,
		&credit.GivenAt,
		&credit.CompletedAt,
		&llmResponse,
		&credit.CreatedAt,
		&credit.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to scan credit: %w", err)
	}

	// Convert amounts from strings to decimal.Decimal
	amount, err := decimal.NewFromString(amountStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse amount: %w", err)
	}
	credit.Amount = amount

	apr, err := decimal.NewFromString(aprStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse apr: %w", err)
	}
	credit.APR = apr

	collateralPerc, err := decimal.NewFromString(collateralPercStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse collateral percentage: %w", err)
	}
	credit.CollateralPercent = collateralPerc

	collateralLock, err := decimal.NewFromString(collateralLockStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse collateral locked: %w", err)
	}
	credit.CollateralLocked = collateralLock

	// Deserialize LLM response from JSON if present
	if llmResponse != nil {
		var llmResp llm.CreditEvaluationResponse
		if jsonBytes, ok := llmResponse.([]byte); ok {
			if err := json.Unmarshal(jsonBytes, &llmResp); err != nil {
				return nil, fmt.Errorf("failed to unmarshal LLM response: %w", err)
			}
			credit.LLMResponse = &llmResp
		} else {
			credit.LLMResponse = llmResponse
		}
	}

	return credit, nil
}

// scanPayment scans a single payment from a database row
func (pr *PostgresRepository) scanPayment(row *sql.Row) (*Payment, error) {
	var (
		principalStr string
		interestStr  string
		totalStr     string
	)

	payment := &Payment{}
	err := row.Scan(
		&payment.ID,
		&payment.CreditID,
		&payment.PaymentNumber,
		&principalStr,
		&interestStr,
		&totalStr,
		&payment.DueDate,
		&payment.PaidDate,
		&payment.Status,
		&payment.CreatedAt,
		&payment.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("payment not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan payment: %w", err)
	}

	principal, err := decimal.NewFromString(principalStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse principal: %w", err)
	}
	payment.PrincipalAmount = principal

	interest, err := decimal.NewFromString(interestStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse interest: %w", err)
	}
	payment.InterestAmount = interest

	total, err := decimal.NewFromString(totalStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse total: %w", err)
	}
	payment.TotalAmount = total

	return payment, nil
}

// scanPaymentRow scans a payment from a rows iterator
func (pr *PostgresRepository) scanPaymentRow(rows *sql.Rows) (*Payment, error) {
	var (
		principalStr string
		interestStr  string
		totalStr     string
	)

	payment := &Payment{}
	err := rows.Scan(
		&payment.ID,
		&payment.CreditID,
		&payment.PaymentNumber,
		&principalStr,
		&interestStr,
		&totalStr,
		&payment.DueDate,
		&payment.PaidDate,
		&payment.Status,
		&payment.CreatedAt,
		&payment.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to scan payment: %w", err)
	}

	principal, err := decimal.NewFromString(principalStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse principal: %w", err)
	}
	payment.PrincipalAmount = principal

	interest, err := decimal.NewFromString(interestStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse interest: %w", err)
	}
	payment.InterestAmount = interest

	total, err := decimal.NewFromString(totalStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse total: %w", err)
	}
	payment.TotalAmount = total

	return payment, nil
}

// scanCreditHistory scans credit history from a database row
func (pr *PostgresRepository) scanCreditHistory(row *sql.Row) (*CreditHistory, error) {
	var (
		borrowedStr string
		repaidStr   string
		rateStr     string
		scoreStr    *string
	)

	history := &CreditHistory{}
	err := row.Scan(
		&history.ID,
		&history.UserID,
		&history.CreditsAsked,
		&history.CreditsPaid,
		&history.CreditsDefaulted,
		&borrowedStr,
		&repaidStr,
		&rateStr,
		&history.ActiveCreditsCount,
		&history.FirstCreditDate,
		&history.LastDefaultDate,
		&scoreStr,
		&history.CreatedAt,
		&history.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrCreditHistoryNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan credit history: %w", err)
	}

	borrowed, err := decimal.NewFromString(borrowedStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse total borrowed: %w", err)
	}
	history.TotalBorrowed = borrowed

	repaid, err := decimal.NewFromString(repaidStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse total repaid: %w", err)
	}
	history.TotalRepaid = repaid

	rate, err := decimal.NewFromString(rateStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse on-time payment rate: %w", err)
	}
	history.OnTimePaymentRate = rate

	if scoreStr != nil {
		score, err := decimal.NewFromString(*scoreStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse credit score: %w", err)
		}
		history.CreditScore = &score
	}

	return history, nil
}

// scanCollateral scans collateral from a database row
func (pr *PostgresRepository) scanCollateral(row *sql.Row) (*Collateral, error) {
	var amountStr string

	collateral := &Collateral{}
	err := row.Scan(
		&collateral.ID,
		&collateral.CreditID,
		&collateral.UserID,
		&collateral.Currency,
		&amountStr,
		&collateral.LockedAt,
		&collateral.ReleasedAt,
		&collateral.CreatedAt,
		&collateral.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("collateral not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan collateral: %w", err)
	}

	amount, err := decimal.NewFromString(amountStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse collateral amount: %w", err)
	}
	collateral.Amount = amount

	return collateral, nil
}

// scanCollateralRow scans collateral from a rows iterator
func (pr *PostgresRepository) scanCollateralRow(rows *sql.Rows) (*Collateral, error) {
	var amountStr string

	collateral := &Collateral{}
	err := rows.Scan(
		&collateral.ID,
		&collateral.CreditID,
		&collateral.UserID,
		&collateral.Currency,
		&amountStr,
		&collateral.LockedAt,
		&collateral.ReleasedAt,
		&collateral.CreatedAt,
		&collateral.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to scan collateral: %w", err)
	}

	amount, err := decimal.NewFromString(amountStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse collateral amount: %w", err)
	}
	collateral.Amount = amount

	return collateral, nil
}
