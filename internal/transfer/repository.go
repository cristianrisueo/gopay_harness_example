package transfer

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/cristianrisueo/gopay/pkg/currency"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Repository defines the interface for transfer data operations
type Repository interface {
	Transfer(
		ctx context.Context,
		senderID, receiverID string,
		amount decimal.Decimal,
		curr currency.Currency,
		idempotencyKey string,
	) (*Transfer, bool, error)
	GetByID(ctx context.Context, transferID string) (*Transfer, error)
	GetHistoryByUserID(ctx context.Context, userID string, limit int) ([]TransferHistoryItem, error)
}

// TransferRepository implements Repository using PostgreSQL with ACID transactions
type TransferRepository struct {
	db *sql.DB
}

// Type assertion to ensure TransferRepository implements Repository
var _ Repository = (*TransferRepository)(nil)

// NewPostgresRepository creates a new PostgreSQL repository instance
func NewPostgresRepository(db *sql.DB) *TransferRepository {
	return &TransferRepository{db: db}
}

// Transfer performs a P2P transfer with ACID guarantees using SELECT FOR UPDATE.
// The idempotency key is recorded in the same transaction that moves the money. If the sender
// already used the key, nothing is moved and the existing transfer is returned with replayed = true.
func (tr *TransferRepository) Transfer(
	ctx context.Context,
	senderID, receiverID string,
	amount decimal.Decimal,
	curr currency.Currency,
	idempotencyKey string,
) (*Transfer, bool, error) {
	// Begins a database transaction
	tx, err := tr.db.BeginTx(ctx, &sql.TxOptions{
		Isolation: sql.LevelReadCommitted,
	})
	if err != nil {
		return nil, false, fmt.Errorf("failed to begin transaction: %w", err)
	}

	// Ensures rollback if anything fails
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	// Locks sender's wallet and retrieves sender's balance (SELECT FOR UPDATE prevents race conditions)
	senderQuery := `
		SELECT balance_eur, balance_usd, balance_jpy
		FROM wallets
		WHERE user_id = $1
		FOR UPDATE
	`

	var usedBalance, senderBalanceEUR, senderBalanceUSD, senderBalanceJPY string

	// Executes the query and scans every currency balance in its respective variable as string
	err = tx.QueryRowContext(ctx, senderQuery, senderID).Scan(
		&senderBalanceEUR,
		&senderBalanceUSD,
		&senderBalanceJPY,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, false, ErrSenderNotFound
		}

		return nil, false, fmt.Errorf("failed to lock sender wallet: %w", err)
	}

	// Selects the correct balance based on the currency received
	switch curr {
	case currency.EUR:
		usedBalance = senderBalanceEUR
	case currency.USD:
		usedBalance = senderBalanceUSD
	case currency.JPY:
		usedBalance = senderBalanceJPY
	default:
		// Assigns err so the deferred rollback runs
		err = ErrInvalidCurrency
		return nil, false, err
	}

	// Locks receiver's wallet (SELECT FOR UPDATE) before recording the transfer,
	// so an unknown receiver is reported as not found instead of a foreign key error
	receiverQuery := `
		SELECT user_id
		FROM wallets
		WHERE user_id = $1
		FOR UPDATE
	`

	var receiverUserID string

	// Executes the query and scans the receiver's ID retrieved
	err = tx.QueryRowContext(ctx, receiverQuery, receiverID).Scan(&receiverUserID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, false, ErrReceiverNotFound
		}

		return nil, false, fmt.Errorf("failed to lock receiver wallet: %w", err)
	}

	// Records the transfer with its idempotency key before moving any money. The unique index on
	// (sender_id, idempotency_key) turns a second insert with the same key into a no-op, also when
	// requests arrive at the same time: PostgreSQL makes the second insert wait for the first one
	transfer := &Transfer{}
	transferID := uuid.New().String()

	insertTransferQuery := `
		INSERT INTO transfers (id, sender_id, receiver_id, amount, currency, type, status, idempotency_key, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
		ON CONFLICT (sender_id, idempotency_key) DO NOTHING
		RETURNING id, sender_id, receiver_id, amount, currency, type, status, created_at
	`

	var amountStr string

	err = tx.QueryRowContext(
		ctx,
		insertTransferQuery,
		transferID,
		senderID,
		receiverID,
		amount.String(),
		string(curr),
		string(TransferTypeP2P),
		string(TransferStatusCompleted),
		idempotencyKey,
	).Scan(
		&transfer.ID,
		&transfer.SenderID,
		&transfer.ReceiverID,
		&amountStr,
		&transfer.Currency,
		&transfer.Type,
		&transfer.Status,
		&transfer.CreatedAt,
	)

	if err != nil {
		// No row returned: the sender already used this key. Returns the existing transfer without
		// moving money. err is still sql.ErrNoRows, so the deferred rollback releases the locks
		if err == sql.ErrNoRows {
			existing, lookupErr := getBySenderAndKey(ctx, tx, senderID, idempotencyKey)
			if lookupErr != nil {
				return nil, false, lookupErr
			}

			return existing, true, nil
		}

		return nil, false, fmt.Errorf("failed to record transfer: %w", err)
	}

	// Converts the used balance to decimal and validates if has sufficient funds
	currentBalance, err := decimal.NewFromString(usedBalance)
	if err != nil {
		return nil, false, fmt.Errorf("failed to parse sender balance: %w", err)
	}

	if currentBalance.LessThan(amount) {
		// Assigns err so the deferred rollback runs and the key is not registered
		err = ErrInsufficientBalance
		return nil, false, err
	}

	// Deducts amount from sender's wallet
	var updateSenderQuery string

	switch curr {
	case currency.EUR:
		updateSenderQuery = `
			UPDATE wallets
			SET balance_eur = balance_eur - $1, updated_at = NOW()
			WHERE user_id = $2
		`
	case currency.USD:
		updateSenderQuery = `
			UPDATE wallets
			SET balance_usd = balance_usd - $1, updated_at = NOW()
			WHERE user_id = $2
		`
	case currency.JPY:
		updateSenderQuery = `
			UPDATE wallets
			SET balance_jpy = balance_jpy - $1, updated_at = NOW()
			WHERE user_id = $2
		`
	}

	_, err = tx.ExecContext(ctx, updateSenderQuery, amount.String(), senderID)
	if err != nil {
		return nil, false, fmt.Errorf("failed to deduct from sender: %w", err)
	}

	// Adds amount to receiver's wallet
	var updateReceiverQuery string

	switch curr {
	case currency.EUR:
		updateReceiverQuery = `
			UPDATE wallets
			SET balance_eur = balance_eur + $1, updated_at = NOW()
			WHERE user_id = $2
		`
	case currency.USD:
		updateReceiverQuery = `
			UPDATE wallets
			SET balance_usd = balance_usd + $1, updated_at = NOW()
			WHERE user_id = $2
		`
	case currency.JPY:
		updateReceiverQuery = `
			UPDATE wallets
			SET balance_jpy = balance_jpy + $1, updated_at = NOW()
			WHERE user_id = $2
		`
	}

	_, err = tx.ExecContext(ctx, updateReceiverQuery, amount.String(), receiverID)
	if err != nil {
		return nil, false, fmt.Errorf("failed to add to receiver: %w", err)
	}

	// Converts amount string to decimal
	transfer.Amount, err = decimal.NewFromString(amountStr)
	if err != nil {
		return nil, false, fmt.Errorf("failed to parse transfer amount: %w", err)
	}

	// Commits the transaction (all or nothing) and returns the transfer
	if err = tx.Commit(); err != nil {
		return nil, false, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return transfer, false, nil
}

// getBySenderAndKey retrieves, inside the given transaction, the transfer a sender recorded with an idempotency key
func getBySenderAndKey(
	ctx context.Context,
	tx *sql.Tx,
	senderID, idempotencyKey string,
) (*Transfer, error) {
	// Creates an empty transfer and the query to execute
	transfer := &Transfer{}

	query := `
		SELECT id, sender_id, receiver_id, amount, currency, type, status, created_at
		FROM transfers
		WHERE sender_id = $1 AND idempotency_key = $2
	`

	// Executes the query and maps the result to the transfer
	var amountStr string
	err := tx.QueryRowContext(ctx, query, senderID, idempotencyKey).Scan(
		&transfer.ID,
		&transfer.SenderID,
		&transfer.ReceiverID,
		&amountStr,
		&transfer.Currency,
		&transfer.Type,
		&transfer.Status,
		&transfer.CreatedAt,
	)

	if err != nil {
		// The insert saw a conflicting key that is not visible: should not happen, the caller may retry
		if err == sql.ErrNoRows {
			return nil, ErrIdempotencyKeyInProgress
		}

		return nil, fmt.Errorf("failed to get transfer by idempotency key: %w", err)
	}

	// Converts amount string to decimal
	transfer.Amount, err = decimal.NewFromString(amountStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse amount: %w", err)
	}

	return transfer, nil
}

// GetByID retrieves a transfer by its ID
func (tr *TransferRepository) GetByID(
	ctx context.Context,
	transferID string,
) (*Transfer, error) {
	// Creates an empty transfer and the query to execute
	transfer := &Transfer{}

	query := `
		SELECT id, sender_id, receiver_id, amount, currency, type, status, created_at
		FROM transfers
		WHERE id = $1
	`

	// Executes the query and maps the result to the transfer
	var amountStr string
	err := tr.db.QueryRowContext(ctx, query, transferID).Scan(
		&transfer.ID,
		&transfer.SenderID,
		&transfer.ReceiverID,
		&amountStr,
		&transfer.Currency,
		&transfer.Type,
		&transfer.Status,
		&transfer.CreatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrTransferNotFound
		}

		return nil, fmt.Errorf("failed to get transfer: %w", err)
	}

	// Converts amount string to decimal
	transfer.Amount, err = decimal.NewFromString(amountStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse amount: %w", err)
	}

	return transfer, nil
}

// GetHistoryByUserID retrieves transfer history for a user (sent and received)
func (tr *TransferRepository) GetHistoryByUserID(
	ctx context.Context,
	userID string,
	limit int,
) ([]TransferHistoryItem, error) {
	// Query to get both sent and received transfers
	query := `
		SELECT
			id,
			CASE
				WHEN sender_id = $1 THEN 'SENT'
				ELSE 'RECEIVED'
			END as type,
			CASE
				WHEN sender_id = $1 THEN receiver_id
				ELSE sender_id
			END as other_user_id,
			amount,
			currency,
			status,
			created_at
		FROM transfers
		WHERE sender_id = $1 OR receiver_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`

	// Executes the query
	rows, err := tr.db.QueryContext(ctx, query, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get transfer history: %w", err)
	}

	defer rows.Close()

	// Scans each record into the history array
	var history []TransferHistoryItem

	for rows.Next() {
		var item TransferHistoryItem
		var amountStr string

		err := rows.Scan(
			&item.ID,
			&item.Direction,
			&item.OtherUserID,
			&amountStr,
			&item.Currency,
			&item.Status,
			&item.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan transfer history: %w", err)
		}

		// Converts amount string to decimal
		item.Amount, err = decimal.NewFromString(amountStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse amount: %w", err)
		}

		history = append(history, item)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating transfer history: %w", err)
	}

	return history, nil
}
