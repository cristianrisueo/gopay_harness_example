package pool

import (
	"context"
	"fmt"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// Service defines the interface for pool operations
type Service interface {
	GetAllPoolsStats(ctx context.Context) ([]*PoolStats, error)
	GetPoolStatsByCurrency(ctx context.Context, currency PoolCurrency) (*PoolStats, error)
	RecordInterestDistribution(ctx context.Context, poolID string, interestAmount, apyRate decimal.Decimal) error
	GetInterestHistoryByCurrency(ctx context.Context, currency PoolCurrency, days int) ([]*InterestDistribution, error)
	AddLiquidity(ctx context.Context, currency PoolCurrency, amount decimal.Decimal) error
	RemoveLiquidity(ctx context.Context, currency PoolCurrency, amount decimal.Decimal) error
	GetCurrentAPY(ctx context.Context, currency PoolCurrency) (decimal.Decimal, error)
}

// Type assertion to ensure PoolService implements Service
var _ Service = (*PoolService)(nil)

// PoolService handles business logic for liquidity pools
type PoolService struct {
	poolRepo Repository
	logger   *zap.Logger
}

// NewService creates a new PoolService
func NewService(poolRepo Repository, logger *zap.Logger) *PoolService {
	return &PoolService{
		poolRepo: poolRepo,
		logger:   logger,
	}
}

// GetAllPoolsStats retrieves statistics for all pools
func (ps *PoolService) GetAllPoolsStats(ctx context.Context) ([]*PoolStats, error) {
	// Gets all the liquidity pools
	pools, err := ps.poolRepo.GetAllPools(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get all pool stats: %w", err)
	}

	var stats []*PoolStats

	// Maps all the liquidity pools stats to the slice of PoolStats
	for _, pool := range pools {
		poolStats, err := ps.GetPoolStatsByCurrency(ctx, pool.Currency)
		if err != nil {
			return nil, fmt.Errorf("failed to get all pool stats for: %s, error: %w", pool.ID, err)
		}

		stats = append(stats, poolStats)
	}

	return stats, nil
}

// GetPoolStatsByCurrency retrieves current statistics for a specific pool by currency
func (ps *PoolService) GetPoolStatsByCurrency(ctx context.Context, currency PoolCurrency) (*PoolStats, error) {
	// Validates the currency
	if !currency.IsValid() {
		return nil, ErrInvalidCurrency
	}

	// Gets that currency pool from repository
	pool, err := ps.poolRepo.GetPoolByCurrency(ctx, currency)
	if err != nil {
		return nil, fmt.Errorf("failed to get pool stats: %w", err)
	}

	// Calculates the available liquidity (total - used)
	availableLiquidity := pool.TotalLiquidity.Sub(pool.TotalUsed)

	// Calculates the utilization rate (used / total)
	var utilizationRate decimal.Decimal
	if pool.TotalLiquidity.GreaterThan(decimal.Zero) {
		utilizationRate = pool.TotalUsed.Div(pool.TotalLiquidity)
	} else {
		utilizationRate = decimal.Zero
	}

	// Creates the variables that will contain the aggregate of interests and current APY
	var totalInterestAccumulated decimal.Decimal
	var apyCurrent decimal.Decimal

	// Calculates the interests (amount) from distributions for the last year
	distributions, err := ps.poolRepo.GetInterestDistributions(ctx, pool.ID, 365)

	if err == nil && len(distributions) > 0 {
		// First the interests
		totalInterestAccumulated = decimal.Zero
		for _, dist := range distributions {
			totalInterestAccumulated = totalInterestAccumulated.Add(dist.InterestAmount)
		}

		apyCurrent = distributions[0].APYRate
	} else {
		// If error retrieving or no distributions found uses default values
		totalInterestAccumulated = decimal.Zero
		apyCurrent, _ = decimal.NewFromString("0.0200")
	}

	// Converts the values retrieved to PoolStats type
	return &PoolStats{
		Currency:                 currency.String(),
		TotalLiquidity:           pool.TotalLiquidity.String(),
		TotalUsed:                pool.TotalUsed.String(),
		AvailableLiquidity:       availableLiquidity.String(),
		UtilizationRate:          utilizationRate.String(),
		APYCurrent:               apyCurrent.String(),
		TotalInterestAccumulated: totalInterestAccumulated.String(),
	}, nil
}

// RecordInterestDistribution records an interest distribution event
func (ps *PoolService) RecordInterestDistribution(
	ctx context.Context,
	poolID string,
	interestAmount, apyRate decimal.Decimal,
) error {
	// Validates the interest amount
	if interestAmount.LessThanOrEqual(decimal.Zero) {
		return ErrInvalidLiquidity
	}

	// Validates the APY rate
	if apyRate.LessThan(decimal.Zero) {
		return fmt.Errorf("invalid APY rate: %s", apyRate.String())
	}

	// Records the interest distribution event
	err := ps.poolRepo.RecordInterestDistribution(ctx, poolID, interestAmount, apyRate)
	if err != nil {
		return fmt.Errorf("failed to record interest distribution: %w", err)
	}

	return nil
}

// GetInterestHistoryByCurrency retrieves interest distribution history from a pool by currency
func (ps *PoolService) GetInterestHistoryByCurrency(
	ctx context.Context,
	currency PoolCurrency,
	days int,
) ([]*InterestDistribution, error) {
	// Validates the currency
	if !currency.IsValid() {
		return nil, ErrInvalidCurrency
	}

	// Validates the days
	if days <= 0 {
		days = 30
	}

	// Gets the pool by currency
	pool, err := ps.poolRepo.GetPoolByCurrency(ctx, currency)
	if err != nil {
		return nil, fmt.Errorf("failed to get pool: %w", err)
	}

	// Gets the interest distributions by pool and number of days
	distributions, err := ps.poolRepo.GetInterestDistributions(ctx, pool.ID, days)
	if err != nil {
		return nil, fmt.Errorf("failed to get interest history: %w", err)
	}

	return distributions, nil
}

// AddLiquidity adds liquidity to a pool (e.g., when users stake funds)
func (ps *PoolService) AddLiquidity(ctx context.Context, currency PoolCurrency, amount decimal.Decimal) error {
	// Validates the currency
	if !currency.IsValid() {
		return ErrInvalidCurrency
	}

	// Validates the amount
	if amount.LessThanOrEqual(decimal.Zero) {
		return ErrInvalidLiquidity
	}

	// Gets the pool by currency
	pool, err := ps.poolRepo.GetPoolByCurrency(ctx, currency)
	if err != nil {
		return fmt.Errorf("failed to get pool: %w", err)
	}

	// Updates the pool liquidity (adds to total)
	newTotalLiquidity := pool.TotalLiquidity.Add(amount)

	err = ps.poolRepo.UpdatePoolLiquidity(ctx, pool.ID, newTotalLiquidity)
	if err != nil {
		return fmt.Errorf("failed to add liquidity: %w", err)
	}

	return nil
}

// RemoveLiquidity removes liquidity from a pool (e.g., when users withdraw staked funds)
func (ps *PoolService) RemoveLiquidity(ctx context.Context, currency PoolCurrency, amount decimal.Decimal) error {
	// Validates the currency
	if !currency.IsValid() {
		return ErrInvalidCurrency
	}

	// Validates the amount
	if amount.LessThanOrEqual(decimal.Zero) {
		return ErrInvalidLiquidity
	}

	// Gets the pool by currency
	pool, err := ps.poolRepo.GetPoolByCurrency(ctx, currency)
	if err != nil {
		return fmt.Errorf("failed to get pool: %w", err)
	}

	// Validates sufficient liquidity to remove
	if pool.TotalLiquidity.LessThan(amount) {
		return ErrInvalidLiquidity
	}

	// Updates the pool liquidity (subtracts from total)
	newTotalLiquidity := pool.TotalLiquidity.Sub(amount)

	err = ps.poolRepo.UpdatePoolLiquidity(ctx, pool.ID, newTotalLiquidity)
	if err != nil {
		return fmt.Errorf("failed to remove liquidity: %w", err)
	}

	return nil
}

// GetCurrentAPY retrieves the current APY rate for a pool by currency
func (ps *PoolService) GetCurrentAPY(ctx context.Context, currency PoolCurrency) (decimal.Decimal, error) {
	// Validates the currency
	if !currency.IsValid() {
		return decimal.Zero, ErrInvalidCurrency
	}

	// Gets the pool by currency
	pool, err := ps.poolRepo.GetPoolByCurrency(ctx, currency)
	if err != nil {
		return decimal.Zero, fmt.Errorf("failed to get pool: %w", err)
	}

	// Gets the latest interest distribution to retrieve the current APY rate
	distributions, err := ps.poolRepo.GetInterestDistributions(ctx, pool.ID, 30)
	if err == nil && len(distributions) > 0 {
		return distributions[0].APYRate, nil
	}

	// Returns default APY if no distributions found
	return decimal.NewFromString("0.0200")
}
