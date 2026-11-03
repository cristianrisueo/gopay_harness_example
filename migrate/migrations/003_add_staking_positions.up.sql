-- Staking Positions Table
CREATE TABLE staking_positions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    currency VARCHAR(3) NOT NULL,
    amount DECIMAL(19, 4) NOT NULL,
    accumulated_interest DECIMAL(19, 4) NOT NULL DEFAULT 0,
    apy_rate DECIMAL(5, 2) NOT NULL,
    lockup_end TIMESTAMP WITH TIME ZONE NOT NULL,
    status VARCHAR(20) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    withdrawn_at TIMESTAMP WITH TIME ZONE
);

-- Indexes for performance
CREATE INDEX idx_staking_user_id ON staking_positions(user_id);
CREATE INDEX idx_staking_status ON staking_positions(status);
CREATE INDEX idx_staking_currency ON staking_positions(currency);
CREATE INDEX idx_staking_lockup_end ON staking_positions(lockup_end);

-- Compound index to consult active positions by user
CREATE INDEX idx_staking_user_status ON staking_positions(user_id, status);
