package pool

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/shopspring/decimal"
)

// Repository defines the interface for pool data access operations
type Repository interface {
	GetAllPools(ctx context.Context) ([]*LiquidityPool, error)
	GetPoolByCurrency(ctx context.Context, currency PoolCurrency) (*LiquidityPool, error)
	RecordInterestDistribution(ctx context.Context, poolID string, interestAmount, apyRate decimal.Decimal) error
	GetInterestDistributions(ctx context.Context, poolID string, days int) ([]*InterestDistribution, error)
	UpdatePoolLiquidity(ctx context.Context, poolID string, newTotalLiquidity decimal.Decimal) error
}

// PostgresRepository implements Repository interface using PostgreSQL
type PostgresRepository struct {
	db *sql.DB
}

// Type assertion to ensure UsersRepository implements Repository
var _ Repository = (*PostgresRepository)(nil)

// NewPostgresRepository creates a new PostgreSQL repository
func NewPostgresRepository(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

// GetAllPools retrieves all liquidity pools
func (pr *PostgresRepository) GetAllPools(ctx context.Context) ([]*LiquidityPool, error) {
	// Creates the query to execute
	const query = `
		SELECT id, currency, total_liquidity, total_used, fee_percentage, created_at, updated_at
		FROM liquidity_pools
		ORDER BY currency
	`

	// Executes the query
	rows, err := pr.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get all pools: %w", err)
	}

	defer rows.Close()

	// Creates an array of LiquidityPool and adds each pool retrieved by the query
	var pools []*LiquidityPool

	for rows.Next() {
		var pool LiquidityPool
		var liquidity, used, fee string

		if err := rows.Scan(
			&pool.ID,
			&pool.Currency,
			&liquidity,
			&used,
			&fee,
			&pool.CreatedAt,
			&pool.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan pool row: %w", err)
		}

		pool.TotalLiquidity, err = decimal.NewFromString(liquidity)
		if err != nil {
			return nil, fmt.Errorf("failed to parse liquidity: %w", err)
		}

		pool.TotalUsed, err = decimal.NewFromString(used)
		if err != nil {
			return nil, fmt.Errorf("failed to parse total used: %w", err)
		}

		pool.FeePercentage, err = decimal.NewFromString(fee)
		if err != nil {
			return nil, fmt.Errorf("failed to parse fee percentage: %w", err)
		}

		pools = append(pools, &pool)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating pool rows: %w", err)
	}

	return pools, nil
}

// GetPoolByCurrency retrieves a liquidity pool by currency
func (pr *PostgresRepository) GetPoolByCurrency(
	ctx context.Context,
	currency PoolCurrency,
) (*LiquidityPool, error) {
	// Creates the query to execute
	const query = `
		SELECT id, currency, total_liquidity, total_used, fee_percentage, created_at, updated_at
		FROM liquidity_pools
		WHERE currency = $1
	`

	// Creates the pool object and the params that will contain fee, liquidity and used as string
	var pool LiquidityPool
	var liquidity, used, fee string

	// Executes the query and maps the data
	err := pr.db.QueryRowContext(ctx, query, string(currency)).Scan(
		&pool.ID,
		&pool.Currency,
		&liquidity,
		&used,
		&fee,
		&pool.CreatedAt,
		&pool.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrPoolNotFound
		}

		return nil, fmt.Errorf("failed to get pool by currency: %w", err)
	}

	// Converts strings to decimal.Decimal and sets it to pool object
	pool.TotalLiquidity, err = decimal.NewFromString(liquidity)
	if err != nil {
		return nil, fmt.Errorf("failed to parse liquidity: %w", err)
	}

	pool.TotalUsed, err = decimal.NewFromString(used)
	if err != nil {
		return nil, fmt.Errorf("failed to parse total used: %w", err)
	}

	pool.FeePercentage, err = decimal.NewFromString(fee)
	if err != nil {
		return nil, fmt.Errorf("failed to parse fee percentage: %w", err)
	}

	return &pool, nil
}

// RecordInterestDistribution records an interest distribution event
func (pr *PostgresRepository) RecordInterestDistribution(
	ctx context.Context,
	poolID string,
	interestAmount, apyRate decimal.Decimal,
) error {
	// Creates the query to execute
	const query = `
		INSERT INTO interest_distributions (pool_id, interest_amount, apy_rate)
		VALUES ($1, $2, $3)
	`

	// Executes the query
	_, err := pr.db.ExecContext(ctx, query, poolID, interestAmount.String(), apyRate.String())
	if err != nil {
		return fmt.Errorf("failed to record interest distribution: %w", err)
	}

	return nil
}

// GetInterestDistributions retrieves interest distribution records for a pool and set of days
func (pr *PostgresRepository) GetInterestDistributions(
	ctx context.Context,
	poolID string,
	days int,
) ([]*InterestDistribution, error) {
	const query = `
		SELECT id, pool_id, interest_amount, apy_rate, created_at
		FROM interest_distributions
		WHERE pool_id = $1 AND created_at >= NOW() - INTERVAL '1 day' * $2
		ORDER BY created_at DESC
	`

	// Executes the query
	rows, err := pr.db.QueryContext(ctx, query, poolID, days)
	if err != nil {
		return nil, fmt.Errorf("failed to get interest distributions: %w", err)
	}

	defer rows.Close()

	// Creates an array of InterestDistribution and adds each interest retrieved by the query
	var distributions []*InterestDistribution

	for rows.Next() {
		var dist InterestDistribution
		var interestAmount, apyRate string

		if err := rows.Scan(
			&dist.ID,
			&dist.PoolID,
			&interestAmount,
			&apyRate,
			&dist.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan distribution row: %w", err)
		}

		dist.InterestAmount, err = decimal.NewFromString(interestAmount)
		if err != nil {
			return nil, fmt.Errorf("failed to parse interest amount: %w", err)
		}

		dist.APYRate, err = decimal.NewFromString(apyRate)
		if err != nil {
			return nil, fmt.Errorf("failed to parse APY rate: %w", err)
		}

		distributions = append(distributions, &dist)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating distribution rows: %w", err)
	}

	return distributions, nil
}

// UpdatePoolLiquidity updates the total liquidity for a pool
func (pr *PostgresRepository) UpdatePoolLiquidity(
	ctx context.Context,
	poolID string,
	newTotalLiquidity decimal.Decimal,
) error {
	// Creates the query to execute
	const query = `
		UPDATE liquidity_pools
		SET total_liquidity = $1, updated_at = NOW()
		WHERE id = $2
	`

	// Executes the query
	result, err := pr.db.ExecContext(ctx, query, newTotalLiquidity.String(), poolID)
	if err != nil {
		return fmt.Errorf("failed to update pool liquidity: %w", err)
	}

	// Checks if any pool has been updated
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return ErrPoolNotFound
	}

	return nil
}
