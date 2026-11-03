-- Credit History Table
-- Tracks user's credit history and score
CREATE TABLE credit_history (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    credits_asked INTEGER NOT NULL DEFAULT 0,
    credits_paid INTEGER NOT NULL DEFAULT 0,
    credits_defaulted INTEGER NOT NULL DEFAULT 0,
    total_amount_borrowed DECIMAL(19, 4) NOT NULL DEFAULT 0,
    total_amount_repaid DECIMAL(19, 4) NOT NULL DEFAULT 0,
    on_time_payment_rate DECIMAL(5, 2) NOT NULL DEFAULT 100.00,
    active_credits_count INTEGER NOT NULL DEFAULT 0,
    first_credit_date TIMESTAMP WITH TIME ZONE,
    last_default_date TIMESTAMP WITH TIME ZONE,
    credit_score DECIMAL(6, 2),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Credits Table
-- Main table storing loan information
CREATE TABLE credits (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    amount DECIMAL(19, 4) NOT NULL,
    currency VARCHAR(3) NOT NULL,
    status VARCHAR(20) NOT NULL,
    apr DECIMAL(5, 2) NOT NULL,
    duration_months INTEGER NOT NULL,
    repayment_type VARCHAR(20) NOT NULL DEFAULT 'AMORTIZED',
    collateral_percentage DECIMAL(5, 2) NOT NULL,
    collateral_locked DECIMAL(19, 4) NOT NULL,
    rejection_reason TEXT,
    risk_level VARCHAR(20),
    requested_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    reviewed_at TIMESTAMP WITH TIME ZONE,
    given_at TIMESTAMP WITH TIME ZONE,
    completed_at TIMESTAMP WITH TIME ZONE,
    llm_response JSONB,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Credit Payments Table
-- Payment schedule and tracking
CREATE TABLE credit_payments (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    credit_id UUID NOT NULL REFERENCES credits(id) ON DELETE CASCADE,
    payment_number INTEGER NOT NULL,
    principal_amount DECIMAL(19, 4) NOT NULL,
    interest_amount DECIMAL(19, 4) NOT NULL,
    total_amount DECIMAL(19, 4) NOT NULL,
    due_date TIMESTAMP WITH TIME ZONE NOT NULL,
    paid_date TIMESTAMP WITH TIME ZONE,
    status VARCHAR(20) NOT NULL DEFAULT 'PENDING',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Credit Collateral Table
-- Multi-currency collateral tracking
CREATE TABLE credit_collateral (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    credit_id UUID NOT NULL REFERENCES credits(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    currency VARCHAR(3) NOT NULL,
    amount DECIMAL(19, 4) NOT NULL,
    locked_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    released_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Indexes for credit_history
CREATE INDEX idx_credit_history_user_id ON credit_history(user_id);

-- Indexes for credits
CREATE INDEX idx_credits_user_id ON credits(user_id);
CREATE INDEX idx_credits_status ON credits(status);
CREATE INDEX idx_credits_currency ON credits(currency);

-- Compound index for filtering user's credits by status
CREATE INDEX idx_credits_user_status ON credits(user_id, status);

-- Indexes for credit_payments
CREATE INDEX idx_payments_credit_id ON credit_payments(credit_id);
CREATE INDEX idx_payments_due_date ON credit_payments(due_date);
CREATE INDEX idx_payments_status ON credit_payments(status);

-- Compound index for finding pending payments by credit
CREATE INDEX idx_payments_credit_status ON credit_payments(credit_id, status);

-- Indexes for credit_collateral
CREATE INDEX idx_collateral_credit_id ON credit_collateral(credit_id);
CREATE INDEX idx_collateral_user_id ON credit_collateral(user_id);
CREATE INDEX idx_collateral_currency ON credit_collateral(currency);

-- Compound index for user's locked collateral by currency
CREATE INDEX idx_collateral_user_currency ON credit_collateral(user_id, currency);
