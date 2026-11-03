package staking

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/cristianrisueo/gopay/pkg/currency"
	"github.com/shopspring/decimal"
)

// Repository defines the interface for staking data access
type Repository interface {
	CreatePosition(
		ctx context.Context, userID string, curr currency.Currency,
		amount, apyRate decimal.Decimal, lockupEnd time.Time,
	) (*StakingPosition, error)

	GetPositionByID(ctx context.Context, positionID string) (*StakingPosition, error)
	GetPositionsByUser(ctx context.Context, userID string, status *PositionStatus) ([]StakingPosition, error)
	GetTotalStakedByUser(ctx context.Context, userID string, curr currency.Currency) (decimal.Decimal, error)
	UpdatePositionAmount(ctx context.Context, positionID string, additionalAmount decimal.Decimal, curr currency.Currency) error
	UpdateAccumulatedInterest(ctx context.Context, positionID string, interest decimal.Decimal) error
	WithdrawPosition(ctx context.Context, positionID string, principalAmount decimal.Decimal, totalToReturn decimal.Decimal, curr currency.Currency, userID string) error
}

// PostgresRepository implements the Repository interface for PostgreSQL
type PostgresRepository struct {
	db *sql.DB
}

// Type assertion to ensure UsersRepository implements Repository
var _ Repository = (*PostgresRepository)(nil)

// NewPostgresRepository creates a new PostgreSQL repository
func NewPostgresRepository(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

// CreatePosition creates a staking position and updates pool liquidity atomically
func (pr *PostgresRepository) CreatePosition(
	ctx context.Context,
	userID string,
	curr currency.Currency,
	amount, apyRate decimal.Decimal,
	lockupEnd time.Time,
) (*StakingPosition, error) {
	// Begins the DB transaction
	tx, err := pr.db.BeginTx(ctx, &sql.TxOptions{
		Isolation: sql.LevelReadCommitted,
	})

	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}

	// Ensures rollback if anything fails
	defer func() {
		if err != nil {
			if rollbackErr := tx.Rollback(); rollbackErr != nil {
				err = fmt.Errorf("%w (rollback failed: %v)", err, rollbackErr)
			}
		}
	}()

	// Gets the pool ID and current liquidity for that currency
	var poolID string
	var currentLiquidity string

	poolQuery := `
		SELECT id, total_liquidity
		FROM liquidity_pools
		WHERE currency = $1
		FOR UPDATE
	`

	err = tx.QueryRowContext(ctx, poolQuery, string(curr)).Scan(&poolID, &currentLiquidity)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("pool not found for currency %s", curr)
		}

		return nil, fmt.Errorf("failed to get pool: %w", err)
	}

	// Parses the current liquidity to decimal from string
	currentLiq, err := decimal.NewFromString(currentLiquidity)
	if err != nil {
		return nil, fmt.Errorf("failed to parse current liquidity: %w", err)
	}

	// Updates the liquidity pool with the new amount
	newLiquidity := currentLiq.Add(amount)

	updatePoolQuery := `
		UPDATE liquidity_pools
		SET total_liquidity = $1, updated_at = NOW()
		WHERE id = $2
	`

	_, err = tx.ExecContext(ctx, updatePoolQuery, newLiquidity.String(), poolID)
	if err != nil {
		return nil, fmt.Errorf("failed to update pool liquidity: %w", err)
	}

	// Create staking position
	positionQuery := `
		INSERT INTO staking_positions (user_id, currency, amount, apy_rate, lockup_end, status)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING
			id, user_id, currency, amount, accumulated_interest, apy_rate, lockup_end, status,
			created_at, updated_at, withdrawn_at
	`

	position := &StakingPosition{}

	err = tx.QueryRowContext(
		ctx,
		positionQuery,
		userID,
		string(curr),
		amount,
		apyRate,
		lockupEnd,
		StatusActive,
	).Scan(
		&position.ID,
		&position.UserID,
		&position.Currency,
		&position.Amount,
		&position.AccumulatedInterest,
		&position.APYRate,
		&position.LockupEnd,
		&position.Status,
		&position.CreatedAt,
		&position.UpdatedAt,
		&position.WithdrawnAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create staking position: %w", err)
	}

	// Deducts the staked amount from user's wallet balance
	var walletUpdateQuery string
	switch curr {
	case currency.EUR:
		walletUpdateQuery = `
			UPDATE wallets
			SET balance_eur = balance_eur - $1, updated_at = NOW()
			WHERE user_id = $2
		`
	case currency.USD:
		walletUpdateQuery = `
			UPDATE wallets
			SET balance_usd = balance_usd - $1, updated_at = NOW()
			WHERE user_id = $2
		`
	case currency.JPY:
		walletUpdateQuery = `
			UPDATE wallets
			SET balance_jpy = balance_jpy - $1, updated_at = NOW()
			WHERE user_id = $2
		`
	}

	_, err = tx.ExecContext(ctx, walletUpdateQuery, amount.String(), userID)
	if err != nil {
		return nil, fmt.Errorf("failed to deduct from wallet: %w", err)
	}

	// Commits the transaction and returns the staking position
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return position, nil
}

// GetPositionByID retrieves a position by its ID
func (pr *PostgresRepository) GetPositionByID(
	ctx context.Context,
	positionID string,
) (*StakingPosition, error) {
	// Creates the query to execute
	query := `
		SELECT 
			id, user_id, currency, amount, accumulated_interest, apy_rate, 
			lockup_end, status, created_at, updated_at, withdrawn_at
		FROM 
			staking_positions
		WHERE 
			id = $1
	`

	// Executes the query and scans the result in the StakingPosition type created
	position := &StakingPosition{}

	err := pr.db.QueryRowContext(ctx, query, positionID).Scan(
		&position.ID,
		&position.UserID,
		&position.Currency,
		&position.Amount,
		&position.AccumulatedInterest,
		&position.APYRate,
		&position.LockupEnd,
		&position.Status,
		&position.CreatedAt,
		&position.UpdatedAt,
		&position.WithdrawnAt,
	)

	if err == sql.ErrNoRows {
		return nil, ErrPositionNotFound
	}

	if err != nil {
		return nil, fmt.Errorf("failed to get staking position: %w", err)
	}

	return position, nil
}

// GetPositionsByUser retrieves positions for a user with optional status filter
// If status is nil, retrieves all positions regardless of status
func (pr *PostgresRepository) GetPositionsByUser(
	ctx context.Context,
	userID string,
	status *PositionStatus,
) ([]StakingPosition, error) {
	// Creates the query to execute
	// If $2 is NULL, the condition is always TRUE (returns all statuses)
	query := `
		SELECT
			id, user_id, currency, amount, accumulated_interest, apy_rate,
			lockup_end, status, created_at, updated_at, withdrawn_at
		FROM
			staking_positions
		WHERE
			user_id = $1
			AND ($2::VARCHAR IS NULL OR status = $2)
		ORDER BY
			created_at DESC
	`

	// Converts *PositionStatus to *string
	var statusStr *string
	if status != nil {
		s := string(*status)
		statusStr = &s
	}

	// Execute the query
	rows, err := pr.db.QueryContext(ctx, query, userID, statusStr)
	if err != nil {
		return nil, fmt.Errorf("failed to get positions: %w", err)
	}

	defer rows.Close()

	// Initializes an empty slice to store results
	positions := []StakingPosition{}

	// Maps each retrieved row to a StakingPosition struct and adds it to the slice
	for rows.Next() {
		var position StakingPosition

		err := rows.Scan(
			&position.ID,
			&position.UserID,
			&position.Currency,
			&position.Amount,
			&position.AccumulatedInterest,
			&position.APYRate,
			&position.LockupEnd,
			&position.Status,
			&position.CreatedAt,
			&position.UpdatedAt,
			&position.WithdrawnAt,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan position: %w", err)
		}

		positions = append(positions, position)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating positions: %w", err)
	}

	return positions, nil
}

// GetTotalStakedByUser returns the total staked amount for a user by currency
func (pr *PostgresRepository) GetTotalStakedByUser(
	ctx context.Context,
	userID string,
	curr currency.Currency,
) (decimal.Decimal, error) {
	// Creates the query. We use COALESCE to avoid returning null if now records
	query := `
		SELECT 
			COALESCE(SUM(amount), 0)
		FROM 
			staking_positions
		WHERE 
			user_id = $1 AND currency = $2 AND status = $3
	`

	// Executes the query and scans the data returned in the total variable
	var total decimal.Decimal

	err := pr.db.QueryRowContext(
		ctx,
		query,
		userID,
		string(curr),
		StatusActive,
	).Scan(&total)

	if err != nil {
		return decimal.Zero, fmt.Errorf("failed to get total staked: %w", err)
	}

	return total, nil
}

// UpdateAccumulatedInterest updates the accumulated interest for a position
func (pr *PostgresRepository) UpdateAccumulatedInterest(
	ctx context.Context,
	positionID string,
	interest decimal.Decimal,
) error {
	// Creates the query
	query := `
		UPDATE staking_positions
		SET accumulated_interest = $1, updated_at = NOW()
		WHERE id = $2
	`

	// Executes the query
	result, err := pr.db.ExecContext(ctx, query, interest, positionID)
	if err != nil {
		return fmt.Errorf("failed to update accumulated interest: %w", err)
	}

	// Checks if any staking position has been updated
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return ErrPositionNotFound
	}

	return nil
}

// UpdatePositionAmount updates position amount and pool liquidity atomically
func (pr *PostgresRepository) UpdatePositionAmount(
	ctx context.Context,
	positionID string,
	additionalAmount decimal.Decimal,
	curr currency.Currency,
) error {
	// Beginss the DB transaction
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

	// Gets the current position amount and user_id
	var currentAmount string
	var userID string

	positionQuery := ` SELECT amount, user_id FROM staking_positions WHERE id = $1 FOR UPDATE`

	err = tx.QueryRowContext(ctx, positionQuery, positionID).Scan(&currentAmount, &userID)
	if err != nil {
		if err == sql.ErrNoRows {
			return ErrPositionNotFound
		}
		return fmt.Errorf("failed to get position: %w", err)
	}

	// Parses and calculates the new amount from string to decimal
	current, err := decimal.NewFromString(currentAmount)
	if err != nil {
		return fmt.Errorf("failed to parse current amount: %w", err)
	}

	// Creates the new ammount adding the amount received
	newAmount := current.Add(additionalAmount)

	// Updates the staking positions amount
	updatePositionQuery := `
		UPDATE staking_positions
		SET amount = $1, updated_at = NOW()
		WHERE id = $2
	`

	_, err = tx.ExecContext(ctx, updatePositionQuery, newAmount, positionID)
	if err != nil {
		return fmt.Errorf("failed to update position amount: %w", err)
	}

	// Gets the pool ID and current liquidity
	var poolID string
	var currentLiquidity string

	poolQuery := `
		SELECT id, total_liquidity
		FROM liquidity_pools
		WHERE currency = $1
		FOR UPDATE
	`

	err = tx.QueryRowContext(ctx, poolQuery, string(curr)).Scan(&poolID, &currentLiquidity)
	if err != nil {
		return fmt.Errorf("failed to get pool: %w", err)
	}

	// Parses the current liquidity from string to decimal
	currentLiq, err := decimal.NewFromString(currentLiquidity)
	if err != nil {
		return fmt.Errorf("failed to parse current liquidity: %w", err)
	}

	// Updates the liquidity pool
	newLiquidity := currentLiq.Add(additionalAmount)

	updatePoolQuery := `
		UPDATE liquidity_pools
		SET total_liquidity = $1, updated_at = NOW()
		WHERE id = $2
	`

	_, err = tx.ExecContext(ctx, updatePoolQuery, newLiquidity.String(), poolID)
	if err != nil {
		return fmt.Errorf("failed to update pool liquidity: %w", err)
	}

	// Deducts the additional staked amount from user's wallet balance
	var walletUpdateQuery string
	switch curr {
	case currency.EUR:
		walletUpdateQuery = `
			UPDATE wallets
			SET balance_eur = balance_eur - $1, updated_at = NOW()
			WHERE user_id = $2
		`
	case currency.USD:
		walletUpdateQuery = `
			UPDATE wallets
			SET balance_usd = balance_usd - $1, updated_at = NOW()
			WHERE user_id = $2
		`
	case currency.JPY:
		walletUpdateQuery = `
			UPDATE wallets
			SET balance_jpy = balance_jpy - $1, updated_at = NOW()
			WHERE user_id = $2
		`
	}

	_, err = tx.ExecContext(ctx, walletUpdateQuery, additionalAmount.String(), userID)
	if err != nil {
		return fmt.Errorf("failed to deduct from wallet: %w", err)
	}

	// Commits the transaction
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// WithdrawPosition marks position as withdrawn, updates pool liquidity, and deposits to wallet atomically
func (pr *PostgresRepository) WithdrawPosition(
	ctx context.Context,
	positionID string,
	principalAmount decimal.Decimal,
	totalToReturn decimal.Decimal,
	curr currency.Currency,
	userID string,
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

	// Sets the position as withdrawn
	withdrawQuery := `
		UPDATE staking_positions
		SET status = $1, withdrawn_at = NOW(), updated_at = NOW()
		WHERE id = $2
	`

	result, err := tx.ExecContext(ctx, withdrawQuery, StatusWithdrawn, positionID)
	if err != nil {
		return fmt.Errorf("failed to withdraw position: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return ErrPositionNotFound
	}

	// Gets the pool ID and current liquidity
	var poolID string
	var currentLiquidity string

	poolQuery := `
		SELECT id, total_liquidity
		FROM liquidity_pools
		WHERE currency = $1
		FOR UPDATE
	`

	err = tx.QueryRowContext(ctx, poolQuery, string(curr)).Scan(&poolID, &currentLiquidity)
	if err != nil {
		return fmt.Errorf("failed to get pool: %w", err)
	}

	// Parses the current liquidity from string to decimal
	currentLiq, err := decimal.NewFromString(currentLiquidity)
	if err != nil {
		return fmt.Errorf("failed to parse current liquidity: %w", err)
	}

	// Updates the liquidity  pool (remove only principal, interest stays in pool)
	newLiquidity := currentLiq.Sub(principalAmount)

	updatePoolQuery := `
		UPDATE liquidity_pools
		SET total_liquidity = $1, updated_at = NOW()
		WHERE id = $2
	`

	_, err = tx.ExecContext(ctx, updatePoolQuery, newLiquidity.String(), poolID)
	if err != nil {
		return fmt.Errorf("failed to update pool liquidity: %w", err)
	}

	// Updates the user's wallet balance (deposit principal + interest)
	var currentBalance string
	var balanceColumn string

	switch curr {
	case currency.EUR:
		balanceColumn = "balance_eur"
	case currency.USD:
		balanceColumn = "balance_usd"
	case currency.JPY:
		balanceColumn = "balance_jpy"
	default:
		return fmt.Errorf("invalid currency: %s", curr)
	}

	// Gets the user's wallet balance for that currency
	walletQuery := fmt.Sprintf(`
		SELECT %s
		FROM wallets
		WHERE user_id = $1
		FOR UPDATE
	`, balanceColumn)

	err = tx.QueryRowContext(ctx, walletQuery, userID).Scan(&currentBalance)
	if err != nil {
		return fmt.Errorf("failed to get wallet balance: %w", err)
	}

	// Parses and calculates the new balance
	current, err := decimal.NewFromString(currentBalance)
	if err != nil {
		return fmt.Errorf("failed to parse current balance: %w", err)
	}

	newBalance := current.Add(totalToReturn)

	// Updates the wallet balance
	updateWalletQuery := fmt.Sprintf(`
		UPDATE wallets
		SET %s = $1, updated_at = NOW()
		WHERE user_id = $2
	`, balanceColumn)

	_, err = tx.ExecContext(ctx, updateWalletQuery, newBalance.String(), userID)
	if err != nil {
		return fmt.Errorf("failed to update wallet balance: %w", err)
	}

	// Commits the transaction
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}
