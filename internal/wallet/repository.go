package wallet

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/cristianrisueo/gopay/pkg/currency"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Repository defines the interface for wallet data operations
type Repository interface {
	Create(ctx context.Context, userID string) (*Wallet, error)
	GetByUserID(ctx context.Context, userID string) (*Wallet, error)
	UpdateBalance(ctx context.Context, userID string, curr currency.Currency, amount decimal.Decimal) error
	Withdraw(ctx context.Context, userID string, curr currency.Currency, amount decimal.Decimal) error
	DepositWithMovement(ctx context.Context, userID string, curr currency.Currency, amount decimal.Decimal, key string) (*Movement, error)
	WithdrawWithMovement(ctx context.Context, userID string, curr currency.Currency, amount decimal.Decimal, key string) (*Movement, error)
}

// WalletRepository implements Repository using PostgreSQL
type WalletRepository struct {
	db *sql.DB
}

// Type assertion to ensure WalletRepository implements Repository
var _ Repository = (*WalletRepository)(nil)

// NewPostgresRepository creates a new PostgreSQL repository instance
func NewPostgresRepository(db *sql.DB) *WalletRepository {
	return &WalletRepository{db: db}
}

// Create creates a new wallet with zero balances for all currencies
func (wr *WalletRepository) Create(ctx context.Context, userID string) (*Wallet, error) {
	// Creates an empty wallet and the query to execute
	wallet := &Wallet{}

	query := `
		INSERT INTO wallets (id, user_id, balance_eur, balance_usd, balance_jpy, created_at, updated_at)
		VALUES ($1, $2, 0, 0, 0, NOW(), NOW())
		RETURNING id, user_id, balance_eur, balance_usd, balance_jpy, created_at, updated_at
	`

	// Scans balances as strings first (database returns DECIMAL as string)
	var balanceEUR, balanceUSD, balanceJPY string

	// Executes the query and maps the result to the wallet. If everything's ok returns it
	err := wr.db.QueryRowContext(
		ctx,
		query,
		uuid.New().String(),
		userID,
	).Scan(
		&wallet.ID,
		&wallet.UserID,
		&balanceEUR,
		&balanceUSD,
		&balanceJPY,
		&wallet.CreatedAt,
		&wallet.UpdatedAt,
	)

	if err != nil {
		return nil, err
	}

	// Converts string balances to decimal (errors ignored since we know they're zero)
	wallet.BalanceEUR, _ = decimal.NewFromString(balanceEUR)
	wallet.BalanceUSD, _ = decimal.NewFromString(balanceUSD)
	wallet.BalanceJPY, _ = decimal.NewFromString(balanceJPY)

	return wallet, nil
}

// GetByUserID retrieves a wallet by user ID
func (wr *WalletRepository) GetByUserID(ctx context.Context, userID string) (*Wallet, error) {
	// Creates an empty wallet and the query to execute
	wallet := &Wallet{}

	query := `
		SELECT id, user_id, balance_eur, balance_usd, balance_jpy, created_at, updated_at
		FROM wallets
		WHERE user_id = $1
	`

	// Scans balances as strings first (database returns DECIMAL as string)
	var balanceEUR, balanceUSD, balanceJPY string

	// Executes the query and maps the result to the wallet
	err := wr.db.QueryRowContext(ctx, query, userID).Scan(
		&wallet.ID,
		&wallet.UserID,
		&balanceEUR,
		&balanceUSD,
		&balanceJPY,
		&wallet.CreatedAt,
		&wallet.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrWalletNotFound
		}

		return nil, err
	}

	// Converts string balances to decimal.Decimal for precise monetary calculations
	wallet.BalanceEUR, err = decimal.NewFromString(balanceEUR)
	if err != nil {
		return nil, err
	}

	wallet.BalanceUSD, err = decimal.NewFromString(balanceUSD)
	if err != nil {
		return nil, err
	}

	wallet.BalanceJPY, err = decimal.NewFromString(balanceJPY)
	if err != nil {
		return nil, err
	}

	return wallet, nil
}

// UpdateBalance updates the balance for a specific currency. It records no wallet movement:
// the service deposits through DepositWithMovement and never calls it.
func (wr *WalletRepository) UpdateBalance(
	ctx context.Context,
	userID string,
	curr currency.Currency,
	amount decimal.Decimal,
) error {
	// Selects the appropriate column based on currency (type-safe thanks to Currency type)
	var query string

	switch curr {
	case currency.EUR:
		query = `
			UPDATE wallets
			SET balance_eur = balance_eur + $1, updated_at = NOW()
			WHERE user_id = $2
		`
	case currency.USD:
		query = `
			UPDATE wallets
			SET balance_usd = balance_usd + $1, updated_at = NOW()
			WHERE user_id = $2
		`
	case currency.JPY:
		query = `
			UPDATE wallets
			SET balance_jpy = balance_jpy + $1, updated_at = NOW()
			WHERE user_id = $2
		`
	default:
		return ErrInvalidCurrency
	}

	// Executes the query (amount converted to string for PostgreSQL DECIMAL)
	result, err := wr.db.ExecContext(ctx, query, amount.String(), userID)
	if err != nil {
		return err
	}

	// Checks if any wallet was updated
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return ErrWalletNotFound
	}

	return nil
}

// Withdraw subtracts a positive amount from a specific currency only if the balance covers it.
// The check and the update are one statement, so concurrent withdrawals cannot overdraw the wallet:
// PostgreSQL locks the row and re-evaluates the WHERE clause against the latest committed balance.
// It records no wallet movement: the service withdraws through WithdrawWithMovement and never calls it.
func (wr *WalletRepository) Withdraw(
	ctx context.Context,
	userID string,
	curr currency.Currency,
	amount decimal.Decimal,
) error {
	// Rejects non-positive amounts, since subtracting a negative one would add money
	if amount.LessThanOrEqual(decimal.Zero) {
		return ErrInvalidAmount
	}

	// Selects the appropriate column based on currency (type-safe thanks to Currency type)
	var query string

	switch curr {
	case currency.EUR:
		query = `
			UPDATE wallets
			SET balance_eur = balance_eur - $1, updated_at = NOW()
			WHERE user_id = $2 AND balance_eur >= $1
		`
	case currency.USD:
		query = `
			UPDATE wallets
			SET balance_usd = balance_usd - $1, updated_at = NOW()
			WHERE user_id = $2 AND balance_usd >= $1
		`
	case currency.JPY:
		query = `
			UPDATE wallets
			SET balance_jpy = balance_jpy - $1, updated_at = NOW()
			WHERE user_id = $2 AND balance_jpy >= $1
		`
	default:
		return ErrInvalidCurrency
	}

	// Executes the query (amount converted to string for PostgreSQL DECIMAL)
	result, err := wr.db.ExecContext(ctx, query, amount.String(), userID)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected > 0 {
		return nil
	}

	// No row updated: the wallet does not exist or its balance is too low. Database errors
	// are returned as they are, so they never reach the user as an insufficient balance
	if _, err := wr.GetByUserID(ctx, userID); err != nil {
		return err
	}

	return ErrInsufficientBalance
}

// DepositWithMovement credits a positive amount and records one DEPOSIT movement with the given key,
// both in one transaction (see applyMovement). A non-nil movement means the key was already used.
func (wr *WalletRepository) DepositWithMovement(
	ctx context.Context,
	userID string,
	curr currency.Currency,
	amount decimal.Decimal,
	key string,
) (*Movement, error) {
	return wr.applyMovement(ctx, userID, MovementDeposit, curr, amount, key)
}

// WithdrawWithMovement debits a positive amount only if the balance covers it and records one
// WITHDRAWAL movement with the given key, both in one transaction (see applyMovement).
// A non-nil movement means the key was already used.
func (wr *WalletRepository) WithdrawWithMovement(
	ctx context.Context,
	userID string,
	curr currency.Currency,
	amount decimal.Decimal,
	key string,
) (*Movement, error) {
	return wr.applyMovement(ctx, userID, MovementWithdrawal, curr, amount, key)
}

// balanceColumn returns the wallets column that holds the balance of the currency
func balanceColumn(curr currency.Currency) (string, error) {
	switch curr {
	case currency.EUR:
		return "balance_eur", nil
	case currency.USD:
		return "balance_usd", nil
	case currency.JPY:
		return "balance_jpy", nil
	default:
		return "", ErrInvalidCurrency
	}
}

// applyMovement runs a deposit or a withdrawal in one READ COMMITTED transaction, in this lock order:
// wallets[user] FOR UPDATE → unique-index entry on the movement insert → UPDATE wallets[user].
// Any error rolls everything back, so a failed operation leaves no movement and no balance change,
// and its key stays unused.
//
// If the user already used the key, nothing is applied: the transaction is rolled back and the
// movement recorded with the key is returned, so the caller can tell a replay from a reused key.
// This happens before the balance UPDATE, so a replay is never answered as an insufficient balance.
// It returns a nil movement when the operation was applied.
func (wr *WalletRepository) applyMovement(
	ctx context.Context,
	userID string,
	movementType MovementType,
	curr currency.Currency,
	amount decimal.Decimal,
	key string,
) (existing *Movement, err error) {
	// Rejects non-positive amounts, since subtracting a negative one would add money
	if amount.LessThanOrEqual(decimal.Zero) {
		return nil, ErrInvalidAmount
	}

	// The column name comes from a closed switch, never from user input
	column, err := balanceColumn(curr)
	if err != nil {
		return nil, err
	}

	tx, err := wr.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, err
	}

	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	// Locks the wallet row; a missing wallet is reported before anything is written
	var locked string
	err = tx.QueryRowContext(ctx, `SELECT `+column+` FROM wallets WHERE user_id = $1 FOR UPDATE`, userID).Scan(&locked)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrWalletNotFound
		}

		return nil, err
	}

	// Records the movement (amount converted to string for PostgreSQL DECIMAL). The unique index on
	// (user_id, idempotency_key) turns a second insert with the same key into a no-op
	inserted, err := tx.ExecContext(ctx, `
		INSERT INTO wallet_movements (user_id, type, currency, amount, idempotency_key)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (user_id, idempotency_key) DO NOTHING
	`, userID, string(movementType), string(curr), amount.String(), key)
	if err != nil {
		return nil, err
	}

	insertedRows, err := inserted.RowsAffected()
	if err != nil {
		return nil, err
	}

	if insertedRows == 0 {
		// The user already used this key: reads what it recorded and applies nothing
		existing, err = movementByKey(ctx, tx, userID, key)
		if err != nil {
			return nil, err
		}

		if err = tx.Rollback(); err != nil {
			return nil, err
		}

		return existing, nil
	}

	// Changes the balance. The debit stays one atomic conditional UPDATE, so it never overdraws
	var query string

	switch movementType {
	case MovementDeposit:
		query = `UPDATE wallets SET ` + column + ` = ` + column + ` + $1, updated_at = NOW() WHERE user_id = $2`
	case MovementWithdrawal:
		query = `UPDATE wallets SET ` + column + ` = ` + column + ` - $1, updated_at = NOW() WHERE user_id = $2 AND ` + column + ` >= $1`
	default:
		return nil, fmt.Errorf("unknown movement type %q", movementType)
	}

	result, err := tx.ExecContext(ctx, query, amount.String(), userID)
	if err != nil {
		return nil, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}

	if rowsAffected == 0 {
		// No row updated: looks the wallet up again. Database errors are returned as they are,
		// so they never reach the user as an insufficient balance
		var exists bool

		err = tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM wallets WHERE user_id = $1)`, userID).Scan(&exists)
		if err != nil {
			return nil, err
		}

		if !exists {
			return nil, ErrWalletNotFound
		}

		if movementType == MovementWithdrawal {
			return nil, ErrInsufficientBalance
		}

		return nil, fmt.Errorf("deposit updated no row for user %s", userID)
	}

	if err = tx.Commit(); err != nil {
		return nil, err
	}

	return nil, nil
}

// movementByKey reads, inside the given transaction, the movement a user recorded with an idempotency key
func movementByKey(ctx context.Context, tx *sql.Tx, userID, key string) (*Movement, error) {
	var movementType, curr, amount string

	err := tx.QueryRowContext(ctx, `
		SELECT type, currency, amount
		FROM wallet_movements
		WHERE user_id = $1 AND idempotency_key = $2
	`, userID, key).Scan(&movementType, &curr, &amount)
	if err != nil {
		// The insert saw a conflicting key that is not visible: should not happen under the wallet lock
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrIdempotencyKeyInProgress
		}

		return nil, err
	}

	parsed, err := decimal.NewFromString(amount)
	if err != nil {
		return nil, fmt.Errorf("failed to parse movement amount: %w", err)
	}

	return &Movement{Type: MovementType(movementType), Currency: currency.Currency(curr), Amount: parsed}, nil
}
