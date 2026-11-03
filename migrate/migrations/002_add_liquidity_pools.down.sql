-- Drop indexes
DROP INDEX IF EXISTS idx_interest_distributions_created_at;
DROP INDEX IF EXISTS idx_interest_distributions_pool_id;
DROP INDEX IF EXISTS idx_liquidity_pools_created_at;
DROP INDEX IF EXISTS idx_liquidity_pools_currency;

-- Drop tables (in reverse order due to foreign keys)
DROP TABLE IF EXISTS interest_distributions;
DROP TABLE IF EXISTS liquidity_pools;
