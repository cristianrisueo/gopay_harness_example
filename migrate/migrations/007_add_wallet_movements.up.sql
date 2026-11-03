-- Wallet movements: one row per deposit or withdrawal, written in the same transaction as the balance change
CREATE TABLE wallet_movements (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id),
    type VARCHAR(10) NOT NULL CHECK (type IN ('DEPOSIT', 'WITHDRAWAL')),
    currency VARCHAR(3) NOT NULL CHECK (currency IN ('EUR', 'USD', 'JPY')),
    amount DECIMAL(19, 4) NOT NULL CHECK (amount > 0),
    idempotency_key UUID NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- A key is used at most once per user, across deposits and withdrawals
CREATE UNIQUE INDEX uq_wallet_movements_user_idempotency_key ON wallet_movements (user_id, idempotency_key);

-- Support reads the history of a user, newest first
CREATE INDEX idx_wallet_movements_user_created_at ON wallet_movements (user_id, created_at DESC);
