-- GoPay Database Seeding
-- This file populates the database with initial test data
-- Execute this AFTER running all migrations

-- ============================================================================
-- 1. LIQUIDITY POOLS
-- ============================================================================
INSERT INTO liquidity_pools (currency, total_liquidity, total_used, fee_percentage)
VALUES
    ('EUR', 100000.0000, 0.0000, 0.0030),
    ('USD', 100000.0000, 0.0000, 0.0030),
    ('JPY', 10000000.0000, 0.0000, 0.0030)
ON CONFLICT (currency) DO NOTHING;

-- ============================================================================
-- 2. USERS (with bcrypt hashed passwords)
-- ============================================================================
-- Password for testuser: Test123456!
-- Password for johndoe: John123456!
-- Password for alice: Alice123456!

INSERT INTO users (id, username, email, password_hash, created_at, updated_at)
VALUES
    -- testuser - created 2 years ago for better credit history
    (
        '5901a3fe-d34c-4c51-a76e-4d2cd8d1df08',
        'testuser',
        'test@example.com',
        '$2a$10$Gyk1l5LYnYttyZyvrw7ileeCT3vrnek6Okbrt3ic7dFLZ7u0XVKwi',  -- Test123456!
        NOW() - INTERVAL '2 years',
        NOW() - INTERVAL '2 years'
    ),
    -- johndoe - regular user
    (
        'e9a21472-3b28-47b6-874a-a4378d60462d',
        'johndoe',
        'john@example.com',
        '$2a$10$MhDptz0LIlPv8uW4qUhgaeMtO/t9MB40vXBuH2PIQ1SWhX52R38iG',  -- John123456!
        NOW(),
        NOW()
    ),
    -- alice - new user for testing
    (
        '48b0e8d2-9c5a-4f3e-8d7f-1a2b3c4d5e6f',
        'alice',
        'alice@example.com',
        '$2a$10$Nw56jm/sWjF1mC0rLwJfpOHOv7JH/n0wUa44.mWyZf2Gp9tr8650S',  -- Alice123456!
        NOW(),
        NOW()
    )
ON CONFLICT (id) DO NOTHING;

-- ============================================================================
-- 3. WALLETS (linked to users)
-- ============================================================================
INSERT INTO wallets (id, user_id, balance_eur, balance_usd, balance_jpy, created_at, updated_at)
VALUES
    (
        'bfa7c20f-35d4-46bc-a13d-8b60a054e36b',
        '5901a3fe-d34c-4c51-a76e-4d2cd8d1df08',  -- testuser
        50000.0000,
        20000.0000,
        0.0000,
        NOW() - INTERVAL '2 years',
        NOW()
    ),
    (
        '4363322c-20e4-41dd-9560-771952e54379',
        'e9a21472-3b28-47b6-874a-a4378d60462d',  -- johndoe
        30000.0000,
        15000.0000,
        0.0000,
        NOW(),
        NOW()
    ),
    (
        '7f8e9d0c-1b2a-3c4d-5e6f-7a8b9c0d1e2f',
        '48b0e8d2-9c5a-4f3e-8d7f-1a2b3c4d5e6f',  -- alice
        10000.0000,
        5000.0000,
        100000.0000,
        NOW(),
        NOW()
    )
ON CONFLICT (id) DO NOTHING;

-- ============================================================================
-- 4. CREDIT HISTORY (for testuser to have good credit score)
-- ============================================================================
INSERT INTO credit_history (
    id, user_id, credits_asked, credits_paid, credits_defaulted,
    total_amount_borrowed, total_amount_repaid, on_time_payment_rate,
    active_credits_count, first_credit_date, last_default_date,
    credit_score, created_at, updated_at
)
VALUES
    (
        uuid_generate_v4(),
        '5901a3fe-d34c-4c51-a76e-4d2cd8d1df08',  -- testuser
        5,                                        -- asked 5 credits
        5,                                        -- paid 5 credits
        0,                                        -- no defaults
        15000.0000,                               -- total borrowed
        15500.0000,                               -- total repaid (with interest)
        100.0000,                                 -- 100% on-time payment
        0,                                        -- no active credits
        NOW() - INTERVAL '2 years',              -- first credit 2 years ago
        NULL,                                     -- never defaulted
        750.0000,                                 -- excellent credit score
        NOW() - INTERVAL '2 years',
        NOW()
    ),
    (
        uuid_generate_v4(),
        'e9a21472-3b28-47b6-874a-a4378d60462d',  -- johndoe
        2,                                        -- asked 2 credits
        2,                                        -- paid 2 credits
        0,                                        -- no defaults
        8000.0000,                                -- total borrowed
        8300.0000,                                -- total repaid
        100.0000,                                 -- 100% on-time payment
        0,                                        -- no active credits
        NOW() - INTERVAL '1 year',               -- first credit 1 year ago
        NULL,                                     -- never defaulted
        680.0000,                                 -- good credit score
        NOW() - INTERVAL '1 year',
        NOW()
    ),
    (
        uuid_generate_v4(),
        '48b0e8d2-9c5a-4f3e-8d7f-1a2b3c4d5e6f',  -- alice (new user, no history)
        0,
        0,
        0,
        0.0000,
        0.0000,
        0.0000,
        0,
        NULL,
        NULL,
        NULL,
        NOW(),
        NOW()
    )
ON CONFLICT (user_id) DO UPDATE SET
    credits_asked = EXCLUDED.credits_asked,
    credits_paid = EXCLUDED.credits_paid,
    total_amount_borrowed = EXCLUDED.total_amount_borrowed,
    total_amount_repaid = EXCLUDED.total_amount_repaid,
    on_time_payment_rate = EXCLUDED.on_time_payment_rate,
    credit_score = EXCLUDED.credit_score;

-- ============================================================================
-- 5. INTEREST DISTRIBUTIONS (for APY calculations)
-- ============================================================================
-- Add some historical interest distributions for realistic APY rates
DO $$
DECLARE
    eur_pool_id UUID;
    usd_pool_id UUID;
    jpy_pool_id UUID;
BEGIN
    -- Get pool IDs
    SELECT id INTO eur_pool_id FROM liquidity_pools WHERE currency = 'EUR';
    SELECT id INTO usd_pool_id FROM liquidity_pools WHERE currency = 'USD';
    SELECT id INTO jpy_pool_id FROM liquidity_pools WHERE currency = 'JPY';

    -- EUR pool distributions (last 6 months)
    INSERT INTO interest_distributions (id, pool_id, interest_amount, apy_rate, created_at)
    VALUES
        (uuid_generate_v4(), eur_pool_id, 150.0000, 2.5000, NOW() - INTERVAL '6 months'),
        (uuid_generate_v4(), eur_pool_id, 145.0000, 2.4000, NOW() - INTERVAL '5 months'),
        (uuid_generate_v4(), eur_pool_id, 160.0000, 2.6000, NOW() - INTERVAL '4 months'),
        (uuid_generate_v4(), eur_pool_id, 155.0000, 2.5000, NOW() - INTERVAL '3 months'),
        (uuid_generate_v4(), eur_pool_id, 158.0000, 2.5500, NOW() - INTERVAL '2 months'),
        (uuid_generate_v4(), eur_pool_id, 162.0000, 2.6000, NOW() - INTERVAL '1 month')
    ON CONFLICT DO NOTHING;

    -- USD pool distributions
    INSERT INTO interest_distributions (id, pool_id, interest_amount, apy_rate, created_at)
    VALUES
        (uuid_generate_v4(), usd_pool_id, 180.0000, 3.0000, NOW() - INTERVAL '6 months'),
        (uuid_generate_v4(), usd_pool_id, 175.0000, 2.9000, NOW() - INTERVAL '5 months'),
        (uuid_generate_v4(), usd_pool_id, 185.0000, 3.1000, NOW() - INTERVAL '4 months'),
        (uuid_generate_v4(), usd_pool_id, 190.0000, 3.2000, NOW() - INTERVAL '3 months'),
        (uuid_generate_v4(), usd_pool_id, 188.0000, 3.1500, NOW() - INTERVAL '2 months'),
        (uuid_generate_v4(), usd_pool_id, 192.0000, 3.2000, NOW() - INTERVAL '1 month')
    ON CONFLICT DO NOTHING;

    -- JPY pool distributions
    INSERT INTO interest_distributions (id, pool_id, interest_amount, apy_rate, created_at)
    VALUES
        (uuid_generate_v4(), jpy_pool_id, 1200.0000, 1.8000, NOW() - INTERVAL '6 months'),
        (uuid_generate_v4(), jpy_pool_id, 1150.0000, 1.7000, NOW() - INTERVAL '5 months'),
        (uuid_generate_v4(), jpy_pool_id, 1250.0000, 1.9000, NOW() - INTERVAL '4 months'),
        (uuid_generate_v4(), jpy_pool_id, 1220.0000, 1.8500, NOW() - INTERVAL '3 months'),
        (uuid_generate_v4(), jpy_pool_id, 1230.0000, 1.8500, NOW() - INTERVAL '2 months'),
        (uuid_generate_v4(), jpy_pool_id, 1240.0000, 1.9000, NOW() - INTERVAL '1 month')
    ON CONFLICT DO NOTHING;
END $$;

-- ============================================================================
-- SEEDING COMPLETE
-- ============================================================================
-- Summary:
-- - 3 Liquidity Pools (EUR, USD, JPY) with initial liquidity
-- - 3 Users (testuser, johndoe, alice) with different credit histories
-- - 3 Wallets with funds
-- - Credit history records showing good payment behavior
-- - Historical interest distributions for APY calculations
--
-- Test credentials:
-- - testuser / Test123456!  (2 years old account, excellent credit)
-- - johndoe / John123456!   (1 year old account, good credit)
-- - alice / Alice123456!     (new account, no credit history)
-- ============================================================================
