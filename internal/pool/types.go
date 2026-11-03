package pool

import (
	"errors"
	"time"

	"github.com/shopspring/decimal"
)

// Domain errors
var (
	ErrPoolNotFound     = errors.New("pool not found")
	ErrInvalidCurrency  = errors.New("invalid currency")
	ErrInvalidLiquidity = errors.New("invalid liquidity amount")
)

// PoolCurrency represents a supported currency type
type PoolCurrency string

const (
	EUR PoolCurrency = "EUR"
	USD PoolCurrency = "USD"
	JPY PoolCurrency = "JPY"
)

// IsValid checks if the currency is supported
func (pc PoolCurrency) IsValid() bool {
	return pc == EUR || pc == USD || pc == JPY
}

// String returns the string representation of the currency
func (pc PoolCurrency) String() string {
	return string(pc)
}

// LiquidityPool represents a liquidity pool for a specific currency
type LiquidityPool struct {
	ID             string          `json:"id"`
	Currency       PoolCurrency    `json:"currency"`
	TotalLiquidity decimal.Decimal `json:"total_liquidity"`
	TotalUsed      decimal.Decimal `json:"total_used"`
	FeePercentage  decimal.Decimal `json:"fee_percentage"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

// PoolStats represents current statistics for a liquidity pool (response DTO)
type PoolStats struct {
	Currency                 string `json:"currency"`
	TotalLiquidity           string `json:"total_liquidity"`
	TotalUsed                string `json:"total_used"`
	AvailableLiquidity       string `json:"available_liquidity"`
	UtilizationRate          string `json:"utilization_rate"`
	APYCurrent               string `json:"apy_current"`
	TotalInterestAccumulated string `json:"total_interest_accumulated"`
}

// InterestDistribution represents a recorded interest distribution event
type InterestDistribution struct {
	ID             string          `json:"id"`
	PoolID         string          `json:"pool_id"`
	InterestAmount decimal.Decimal `json:"interest_amount"`
	APYRate        decimal.Decimal `json:"apy_rate"`
	CreatedAt      time.Time       `json:"created_at"`
}

// InterestDistributionResponse represents historical interest distributions
type InterestDistributionResponse struct {
	CreatedAt      string `json:"created_at"`
	InterestAmount string `json:"interest_amount"`
	APYRate        string `json:"apy_rate"`
}
