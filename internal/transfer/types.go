package transfer

import (
	"errors"
	"time"

	"github.com/cristianrisueo/gopay/pkg/currency"
	"github.com/shopspring/decimal"
)

// Domain errors
var (
	ErrTransferNotFound    = errors.New("transfer not found")
	ErrInvalidAmount       = errors.New("invalid amount")
	ErrInvalidCurrency     = errors.New("invalid currency")
	ErrInsufficientBalance = errors.New("insufficient balance")
	ErrSenderNotFound      = errors.New("sender not found")
	ErrReceiverNotFound    = errors.New("receiver not found")
	ErrSelfTransfer        = errors.New("cannot transfer to yourself")
	ErrCurrencyMismatch    = errors.New("currency mismatch: sender and receiver must use same currency")

	ErrIdempotencyKeyMismatch   = errors.New("idempotency key already used with a different request")
	ErrIdempotencyKeyInProgress = errors.New("idempotency key is being used by a transfer in progress")
)

// Amount limits imposed by the DECIMAL(19, 4) columns: 4 decimals and 15 integer digits
const maxAmountDecimals = 4

var maxAmount = decimal.RequireFromString("999999999999999.9999")

// TransferType represents the type of transfer
type TransferType string

const (
	TransferTypeP2P TransferType = "P2P"
)

// TransferStatus represents the status of a transfer
type TransferStatus string

const (
	TransferStatusPending   TransferStatus = "PENDING"
	TransferStatusCompleted TransferStatus = "COMPLETED"
	TransferStatusFailed    TransferStatus = "FAILED"
)

// TransferDirection represents the direction of a transfer
type TransferDirection string

const (
	TransferDirectionSent     TransferDirection = "SENT"
	TransferDirectionReceived TransferDirection = "RECEIVED"
)

// Transfer represents a money transfer between two users
type Transfer struct {
	ID         string            `json:"id"`
	SenderID   string            `json:"sender_id"`
	ReceiverID string            `json:"receiver_id"`
	Amount     decimal.Decimal   `json:"amount"`
	Currency   currency.Currency `json:"currency"`
	Type       TransferType      `json:"type"`
	Status     TransferStatus    `json:"status"`
	CreatedAt  time.Time         `json:"created_at"`
}

// TransferRequest represents a request to transfer money to another user
type TransferRequest struct {
	ReceiverID string            `json:"receiver_id" binding:"required,uuid"`
	Amount     string            `json:"amount" binding:"required"`
	Currency   currency.Currency `json:"currency" binding:"required,oneof=EUR USD JPY"`
}

// TransferResponse represents a successful transfer response
type TransferResponse struct {
	TransferID      string            `json:"transfer_id"`
	SenderID        string            `json:"sender_id"`
	ReceiverID      string            `json:"receiver_id"`
	Amount          decimal.Decimal   `json:"amount"`
	Currency        currency.Currency `json:"currency"`
	Status          TransferStatus    `json:"status"`
	SenderBalance   decimal.Decimal   `json:"sender_balance"`
	ReceiverBalance decimal.Decimal   `json:"receiver_balance"`
	CreatedAt       time.Time         `json:"created_at"`
	Replayed        bool              `json:"-"` // True when the idempotency key was already used: no money was moved
}

// TransferHistoryItem represents a transfer in the user's history
type TransferHistoryItem struct {
	ID          string            `json:"id"`
	Direction   TransferDirection `json:"direction"`
	OtherUserID string            `json:"other_user_id"`
	Amount      decimal.Decimal   `json:"amount"`
	Currency    currency.Currency `json:"currency"`
	Status      TransferStatus    `json:"status"`
	CreatedAt   time.Time         `json:"created_at"`
}
