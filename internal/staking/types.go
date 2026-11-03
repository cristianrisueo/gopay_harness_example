package staking

import (
	"errors"
	"time"

	"github.com/cristianrisueo/gopay/pkg/currency"
	"github.com/shopspring/decimal"
)

// Domain errors
var (
	ErrInsufficientBalance = errors.New("insufficient available balance for staking")
	ErrInvalidAmount       = errors.New("invalid amount: must be at least 1000")
	ErrPositionNotFound    = errors.New("staking position not found")
	ErrPositionNotActive   = errors.New("staking position is not active")
	ErrStakingLocked       = errors.New("staking position is still locked")
	ErrUnauthorized        = errors.New("you are not authorized to access this position")
	ErrInvalidCurrency     = errors.New("invalid currency")
)

// PositionStatus represents the state of a staking position
type PositionStatus string

const (
	StatusActive    PositionStatus = "ACTIVE"
	StatusCompleted PositionStatus = "COMPLETED"
	StatusWithdrawn PositionStatus = "WITHDRAWN"
)

// StakingPosition represents a user's staking position in a liquidity pool
type StakingPosition struct {
	ID                  string            `json:"id"`
	UserID              string            `json:"user_id"`
	Currency            currency.Currency `json:"currency"`
	Amount              decimal.Decimal   `json:"amount"`
	AccumulatedInterest decimal.Decimal   `json:"accumulated_interest"`
	APYRate             decimal.Decimal   `json:"apy_rate"`
	LockupEnd           time.Time         `json:"lockup_end"`
	Status              PositionStatus    `json:"status"`
	CreatedAt           time.Time         `json:"created_at"`
	UpdatedAt           time.Time         `json:"updated_at"`
	WithdrawnAt         *time.Time        `json:"withdrawn_at,omitempty"`
}

// CreateStakeRequest represents the request to create a new stake
type CreateStakeRequest struct {
	Amount   string            `json:"amount" binding:"required"`
	Currency currency.Currency `json:"currency" binding:"required,oneof=EUR USD JPY"`
}

// IncreaseStakeRequest represents the request to increase an existing stake
type IncreaseStakeRequest struct {
	Amount string `json:"amount" binding:"required"`
}

// StakeResponse represents the response after creating a stake
type StakeResponse struct {
	PositionID string            `json:"position_id"`
	Amount     string            `json:"amount"`
	Currency   currency.Currency `json:"currency"`
	APYRate    string            `json:"apy_rate"`
	LockupEnd  time.Time         `json:"lockup_end"`
	Status     PositionStatus    `json:"status"`
	CreatedAt  time.Time         `json:"created_at"`
}

// PositionDetail represents detailed information about a staking position
type PositionDetail struct {
	ID                  string            `json:"id"`
	Currency            currency.Currency `json:"currency"`
	Amount              string            `json:"amount"`
	AccumulatedInterest string            `json:"accumulated_interest"`
	APYRate             string            `json:"apy_rate"`
	LockupEnd           time.Time         `json:"lockup_end"`
	Status              PositionStatus    `json:"status"`
	CanWithdraw         bool              `json:"can_withdraw"`
	DaysRemaining       int               `json:"days_remaining"`
	CreatedAt           time.Time         `json:"created_at"`
}

// TotalStaked represents total staked amounts by currency
type TotalStaked struct {
	EUR string `json:"eur"`
	USD string `json:"usd"`
	JPY string `json:"jpy"`
}

// UserPositionsResponse represents the response for user positions query
type UserPositionsResponse struct {
	Positions   []PositionDetail `json:"positions"`
	TotalStaked TotalStaked      `json:"total_staked"`
}

// WithdrawResponse represents the response after withdrawing a stake
type WithdrawResponse struct {
	WithdrawnAmount  string `json:"withdrawn_amount"`
	Principal        string `json:"principal"`
	Interest         string `json:"interest"`
	NewWalletBalance string `json:"new_wallet_balance"`
}
