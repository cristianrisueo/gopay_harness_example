package wallet

import (
	"errors"
	"time"

	"github.com/cristianrisueo/gopay/pkg/currency"
	"github.com/shopspring/decimal"
)

// Domain errors
var (
	ErrWalletNotFound      = errors.New("wallet not found")
	ErrInsufficientBalance = errors.New("insufficient balance")
	ErrInvalidCurrency     = errors.New("invalid currency")
	ErrInvalidAmount       = errors.New("invalid amount")
	ErrAmountOutOfRange    = errors.New("amount out of range")

	ErrIdempotencyKeyMismatch   = errors.New("idempotency key already used with a different request")
	ErrIdempotencyKeyInProgress = errors.New("idempotency key is being used by a wallet operation in progress")
)

// Amount limits imposed by the DECIMAL(19, 4) columns: 4 decimals and 15 integer digits
const maxAmountDecimals = 4

var maxAmount = decimal.RequireFromString("999999999999999.9999")

// MovementType is the kind of balance change recorded in wallet_movements
type MovementType string

const (
	MovementDeposit    MovementType = "DEPOSIT"
	MovementWithdrawal MovementType = "WITHDRAWAL"
)

// Movement is the request a wallet_movements row recorded, read back to compare it with a retry
type Movement struct {
	Type     MovementType
	Currency currency.Currency
	Amount   decimal.Decimal
}

// Wallet represents a user's wallet with multiple currency balances
type Wallet struct {
	ID         string          `json:"id"`
	UserID     string          `json:"user_id"`
	BalanceEUR decimal.Decimal `json:"balance_eur"`
	BalanceUSD decimal.Decimal `json:"balance_usd"`
	BalanceJPY decimal.Decimal `json:"balance_jpy"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
}

// TransactionRequest represents a request to deposit or withdraw funds from a wallet
type TransactionRequest struct {
	Amount   string            `json:"amount" binding:"required"`
	Currency currency.Currency `json:"currency" binding:"required,oneof=EUR USD JPY"`
}

// BalanceResponse represents the balance of all currencies in a wallet
type BalanceResponse struct {
	EUR decimal.Decimal `json:"eur"`
	USD decimal.Decimal `json:"usd"`
	JPY decimal.Decimal `json:"jpy"`
}
