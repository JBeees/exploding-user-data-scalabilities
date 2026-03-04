-- ============================================================
-- Peak Load Management - Database Schema & Seed Data
-- ============================================================

-- Enable UUID extension
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pg_stat_statements";

-- ─── USERS TABLE ────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS users (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    username    VARCHAR(50)  NOT NULL UNIQUE,
    email       VARCHAR(100) NOT NULL UNIQUE,
    balance     NUMERIC(18, 2) NOT NULL DEFAULT 0.00,
    status      VARCHAR(20)  NOT NULL DEFAULT 'active'
                    CHECK (status IN ('active', 'inactive', 'suspended')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Index for fast lookup by username/email
CREATE INDEX IF NOT EXISTS idx_users_username ON users(username);
CREATE INDEX IF NOT EXISTS idx_users_email    ON users(email);
CREATE INDEX IF NOT EXISTS idx_users_status   ON users(status);

-- ─── TRANSACTIONS TABLE ─────────────────────────────────────
CREATE TABLE IF NOT EXISTS transactions (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id         UUID NOT NULL REFERENCES users(id),
    type            VARCHAR(20) NOT NULL
                        CHECK (type IN ('credit', 'debit', 'transfer')),
    amount          NUMERIC(18, 2) NOT NULL CHECK (amount > 0),
    status          VARCHAR(20) NOT NULL DEFAULT 'pending'
                        CHECK (status IN ('pending', 'processing', 'completed', 'failed')),
    reference_id    VARCHAR(100) UNIQUE,
    description     TEXT,
    metadata        JSONB,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes optimized for read-heavy query patterns
CREATE INDEX IF NOT EXISTS idx_txn_user_id      ON transactions(user_id);
CREATE INDEX IF NOT EXISTS idx_txn_status        ON transactions(status);
CREATE INDEX IF NOT EXISTS idx_txn_created_at    ON transactions(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_txn_reference_id  ON transactions(reference_id);
-- Composite index for common query: user's transactions by date
CREATE INDEX IF NOT EXISTS idx_txn_user_created  ON transactions(user_id, created_at DESC);

-- ─── AUTO-UPDATE updated_at TRIGGER ─────────────────────────
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_users_updated_at
    BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER trg_transactions_updated_at
    BEFORE UPDATE ON transactions
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- ─── SEED DATA: 1000 USERS ──────────────────────────────────
-- Generate dummy users with realistic data
INSERT INTO users (id, username, email, balance, status)
SELECT
    uuid_generate_v4(),
    'user_' || LPAD(i::TEXT, 6, '0'),
    'user_' || LPAD(i::TEXT, 6, '0') || '@example.com',
    ROUND((RANDOM() * 10000000)::NUMERIC, 2),  -- balance 0 - 10jt
    CASE
        WHEN RANDOM() < 0.90 THEN 'active'
        WHEN RANDOM() < 0.07 THEN 'inactive'
        ELSE 'suspended'
    END
FROM generate_series(1, 1000) AS i
ON CONFLICT DO NOTHING;

-- ─── SEED DATA: 50.000 TRANSACTIONS ────────────────────────
-- Generate dummy transactions untuk simulasi beban query
INSERT INTO transactions (id, user_id, type, amount, status, reference_id, description, created_at)
SELECT
    uuid_generate_v4(),
    u.id,
    CASE FLOOR(RANDOM() * 3)::INT
        WHEN 0 THEN 'credit'
        WHEN 1 THEN 'debit'
        ELSE 'transfer'
    END,
    ROUND((RANDOM() * 5000000 + 1000)::NUMERIC, 2),  -- amount 1rb - 5jt
    CASE FLOOR(RANDOM() * 4)::INT
        WHEN 0 THEN 'completed'
        WHEN 1 THEN 'completed'
        WHEN 2 THEN 'completed'
        ELSE 'failed'
    END,  -- 75% completed, 25% failed
    'REF-' || TO_CHAR(NOW(), 'YYYYMMDD') || '-' || LPAD((ROW_NUMBER() OVER ())::TEXT, 10, '0'),
    'Dummy transaction seed data #' || ROW_NUMBER() OVER (),
    NOW() - (RANDOM() * INTERVAL '90 days')   -- created within last 90 days
FROM
    (SELECT id FROM users ORDER BY RANDOM()) u
    CROSS JOIN generate_series(1, 50) AS s  -- 50 txn per user = 50.000 total
LIMIT 50000
ON CONFLICT DO NOTHING;

-- ─── VERIFY SEED ────────────────────────────────────────────
DO $$
DECLARE
    user_count  INT;
    txn_count   INT;
BEGIN
    SELECT COUNT(*) INTO user_count FROM users;
    SELECT COUNT(*) INTO txn_count FROM transactions;
    RAISE NOTICE '✅ Seed complete: % users, % transactions', user_count, txn_count;
END $$;
