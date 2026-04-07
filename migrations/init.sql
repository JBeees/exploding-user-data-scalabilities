-- ============================================================
-- Peak Load Management - Database Schema & Seed Data
-- Dengan Table Partitioning per Bulan pada tabel transactions
-- ============================================================

-- Enable extensions
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

CREATE INDEX IF NOT EXISTS idx_users_username ON users(username);
CREATE INDEX IF NOT EXISTS idx_users_email    ON users(email);
CREATE INDEX IF NOT EXISTS idx_users_status   ON users(status);

-- ─── TRANSACTIONS TABLE (PARTITIONED) ───────────────────────
-- Tabel induk dengan PARTITION BY RANGE berdasarkan created_at
-- PostgreSQL akan otomatis routing INSERT ke partisi yang tepat
-- Query dengan filter created_at hanya scan partisi yang relevan
CREATE TABLE IF NOT EXISTS transactions (
    id              UUID NOT NULL DEFAULT uuid_generate_v4(),
    user_id         UUID NOT NULL REFERENCES users(id),
    type            VARCHAR(20) NOT NULL
                        CHECK (type IN ('credit', 'debit', 'transfer')),
    amount          NUMERIC(18, 2) NOT NULL CHECK (amount > 0),
    status          VARCHAR(20) NOT NULL DEFAULT 'pending'
                        CHECK (status IN ('pending', 'processing', 'completed', 'failed')),
    reference_id    VARCHAR(100),
    description     TEXT,
    metadata        JSONB,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
) PARTITION BY RANGE (created_at);

-- ─── PARTISI 2025 ───────────────────────────────────────────
CREATE TABLE IF NOT EXISTS transactions_2025_01
    PARTITION OF transactions
    FOR VALUES FROM ('2025-01-01') TO ('2025-02-01');

CREATE TABLE IF NOT EXISTS transactions_2025_02
    PARTITION OF transactions
    FOR VALUES FROM ('2025-02-01') TO ('2025-03-01');

CREATE TABLE IF NOT EXISTS transactions_2025_03
    PARTITION OF transactions
    FOR VALUES FROM ('2025-03-01') TO ('2025-04-01');

CREATE TABLE IF NOT EXISTS transactions_2025_04
    PARTITION OF transactions
    FOR VALUES FROM ('2025-04-01') TO ('2025-05-01');

CREATE TABLE IF NOT EXISTS transactions_2025_05
    PARTITION OF transactions
    FOR VALUES FROM ('2025-05-01') TO ('2025-06-01');

CREATE TABLE IF NOT EXISTS transactions_2025_06
    PARTITION OF transactions
    FOR VALUES FROM ('2025-06-01') TO ('2025-07-01');

CREATE TABLE IF NOT EXISTS transactions_2025_07
    PARTITION OF transactions
    FOR VALUES FROM ('2025-07-01') TO ('2025-08-01');

CREATE TABLE IF NOT EXISTS transactions_2025_08
    PARTITION OF transactions
    FOR VALUES FROM ('2025-08-01') TO ('2025-09-01');

CREATE TABLE IF NOT EXISTS transactions_2025_09
    PARTITION OF transactions
    FOR VALUES FROM ('2025-09-01') TO ('2025-10-01');

CREATE TABLE IF NOT EXISTS transactions_2025_10
    PARTITION OF transactions
    FOR VALUES FROM ('2025-10-01') TO ('2025-11-01');

CREATE TABLE IF NOT EXISTS transactions_2025_11
    PARTITION OF transactions
    FOR VALUES FROM ('2025-11-01') TO ('2025-12-01');

CREATE TABLE IF NOT EXISTS transactions_2025_12
    PARTITION OF transactions
    FOR VALUES FROM ('2025-12-01') TO ('2026-01-01');

-- ─── PARTISI 2026 ───────────────────────────────────────────
CREATE TABLE IF NOT EXISTS transactions_2026_01
    PARTITION OF transactions
    FOR VALUES FROM ('2026-01-01') TO ('2026-02-01');

CREATE TABLE IF NOT EXISTS transactions_2026_02
    PARTITION OF transactions
    FOR VALUES FROM ('2026-02-01') TO ('2026-03-01');

CREATE TABLE IF NOT EXISTS transactions_2026_03
    PARTITION OF transactions
    FOR VALUES FROM ('2026-03-01') TO ('2026-04-01');

CREATE TABLE IF NOT EXISTS transactions_2026_04
    PARTITION OF transactions
    FOR VALUES FROM ('2026-04-01') TO ('2026-05-01');

CREATE TABLE IF NOT EXISTS transactions_2026_05
    PARTITION OF transactions
    FOR VALUES FROM ('2026-05-01') TO ('2026-06-01');

CREATE TABLE IF NOT EXISTS transactions_2026_06
    PARTITION OF transactions
    FOR VALUES FROM ('2026-06-01') TO ('2026-07-01');

CREATE TABLE IF NOT EXISTS transactions_2026_07
    PARTITION OF transactions
    FOR VALUES FROM ('2026-07-01') TO ('2026-08-01');

CREATE TABLE IF NOT EXISTS transactions_2026_08
    PARTITION OF transactions
    FOR VALUES FROM ('2026-08-01') TO ('2026-09-01');

CREATE TABLE IF NOT EXISTS transactions_2026_09
    PARTITION OF transactions
    FOR VALUES FROM ('2026-09-01') TO ('2026-10-01');

CREATE TABLE IF NOT EXISTS transactions_2026_10
    PARTITION OF transactions
    FOR VALUES FROM ('2026-10-01') TO ('2026-11-01');

CREATE TABLE IF NOT EXISTS transactions_2026_11
    PARTITION OF transactions
    FOR VALUES FROM ('2026-11-01') TO ('2026-12-01');

CREATE TABLE IF NOT EXISTS transactions_2026_12
    PARTITION OF transactions
    FOR VALUES FROM ('2026-12-01') TO ('2027-01-01');

-- ─── PARTISI DEFAULT (fallback) ─────────────────────────────
-- Menampung data yang tidak masuk partisi manapun
-- Mencegah INSERT error kalau ada data di luar range
CREATE TABLE IF NOT EXISTS transactions_default
    PARTITION OF transactions DEFAULT;

-- ─── INDEX PER PARTISI ──────────────────────────────────────
-- Index dibuat di tabel induk, otomatis berlaku di semua partisi
-- Ini yang membuat query tetap cepat meski data sudah terbagi

-- Index untuk lookup by user_id + created_at (query paling umum)
CREATE INDEX IF NOT EXISTS idx_txn_user_created
    ON transactions(user_id, created_at DESC);

-- Index untuk lookup by status
CREATE INDEX IF NOT EXISTS idx_txn_status
    ON transactions(status);

-- Index untuk lookup by reference_id (unique per transaksi)
CREATE INDEX IF NOT EXISTS idx_txn_reference_id
    ON transactions(reference_id);

-- Partial index — hanya untuk transaksi yang belum selesai
-- Ukuran index jauh lebih kecil karena tidak include 'completed'
CREATE INDEX IF NOT EXISTS idx_txn_pending_processing
    ON transactions(user_id, created_at DESC)
    WHERE status IN ('pending', 'processing');

-- ─── UNIQUE CONSTRAINT pada reference_id ────────────────────
-- Catatan: UNIQUE constraint pada tabel terpartisi di PostgreSQL
-- harus menyertakan partition key (created_at)
-- Maka kita enforce uniqueness via index partial
CREATE UNIQUE INDEX IF NOT EXISTS idx_txn_reference_unique
    ON transactions(reference_id, created_at)
    WHERE reference_id IS NOT NULL;

-- ─── AUTO-UPDATE updated_at TRIGGER ─────────────────────────
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE TRIGGER trg_users_updated_at
    BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE OR REPLACE TRIGGER trg_transactions_updated_at
    BEFORE UPDATE ON transactions
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- ─── SEED DATA: 1000 USERS ──────────────────────────────────
INSERT INTO users (id, username, email, balance, status)
SELECT
    uuid_generate_v4(),
    'user_' || LPAD(i::TEXT, 6, '0'),
    'user_' || LPAD(i::TEXT, 6, '0') || '@example.com',
    ROUND((RANDOM() * 10000000)::NUMERIC, 2),
    CASE
        WHEN RANDOM() < 0.90 THEN 'active'
        WHEN RANDOM() < 0.07 THEN 'inactive'
        ELSE 'suspended'
    END
FROM generate_series(1, 1000) AS i
ON CONFLICT DO NOTHING;

-- ─── SEED DATA: 50.000 TRANSACTIONS ─────────────────────────
-- Data tersebar dalam 90 hari terakhir
-- Otomatis masuk ke partisi bulan yang tepat
INSERT INTO transactions (id, user_id, type, amount, status, reference_id, description, created_at)
SELECT
    uuid_generate_v4(),
    u.id,
    CASE FLOOR(RANDOM() * 3)::INT
        WHEN 0 THEN 'credit'
        WHEN 1 THEN 'debit'
        ELSE 'transfer'
    END,
    ROUND((RANDOM() * 5000000 + 1000)::NUMERIC, 2),
    CASE FLOOR(RANDOM() * 4)::INT
        WHEN 0 THEN 'completed'
        WHEN 1 THEN 'completed'
        WHEN 2 THEN 'completed'
        ELSE 'failed'
    END,
    'REF-' || TO_CHAR(NOW() - (RANDOM() * INTERVAL '90 days'), 'YYYYMMDD')
        || '-' || LPAD((ROW_NUMBER() OVER ())::TEXT, 10, '0'),
    'Dummy transaction seed data #' || ROW_NUMBER() OVER (),
    NOW() - (RANDOM() * INTERVAL '90 days')
FROM
    (SELECT id FROM users ORDER BY RANDOM()) u
    CROSS JOIN generate_series(1, 50) AS s
LIMIT 50000
ON CONFLICT DO NOTHING;

-- ─── VERIFY PARTISI ─────────────────────────────────────────
-- Cek distribusi data per partisi
DO $$
DECLARE
    user_count  INT;
    txn_count   INT;
BEGIN
    SELECT COUNT(*) INTO user_count FROM users;
    SELECT COUNT(*) INTO txn_count FROM transactions;
    RAISE NOTICE '✅ Seed complete: % users, % transactions', user_count, txn_count;
END $$;

-- Query ini bisa dijalankan manual untuk melihat distribusi per partisi:
-- SELECT
--     inhrelid::regclass AS partition_name,
--     pg_relation_size(inhrelid) AS size_bytes,
--     pg_size_pretty(pg_relation_size(inhrelid)) AS size_pretty
-- FROM pg_inherits
-- WHERE inhparent = 'transactions'::regclass
-- ORDER BY partition_name;
