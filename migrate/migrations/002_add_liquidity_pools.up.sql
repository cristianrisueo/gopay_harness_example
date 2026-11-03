-- Liquidity Pools table
CREATE TABLE liquidity_pools (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    currency VARCHAR(3) UNIQUE NOT NULL,
    total_liquidity DECIMAL(19, 4) NOT NULL DEFAULT 0,
    total_used DECIMAL(19, 4) NOT NULL DEFAULT 0,
    fee_percentage DECIMAL(5, 4) NOT NULL DEFAULT 0.0030,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Interest Distributions table (track interest generated and distributed)
CREATE TABLE interest_distributions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    pool_id UUID NOT NULL REFERENCES liquidity_pools(id) ON DELETE CASCADE,
    interest_amount DECIMAL(19, 4) NOT NULL,
    apy_rate DECIMAL(7, 4) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Indexes for performance
CREATE INDEX idx_liquidity_pools_currency ON liquidity_pools(currency);
CREATE INDEX idx_liquidity_pools_created_at ON liquidity_pools(created_at);
CREATE INDEX idx_interest_distributions_pool_id ON interest_distributions(pool_id);
CREATE INDEX idx_interest_distributions_created_at ON interest_distributions(created_at);
