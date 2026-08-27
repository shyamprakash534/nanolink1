-- ==============================================================================
-- NanoLink PostgreSQL Database Schema
-- High-throughput, partitioned schema for distributed URL shortening
-- ==============================================================================

CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- Users and API Keys Table
CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email VARCHAR(255) UNIQUE NOT NULL,
    api_key_hash VARCHAR(255) UNIQUE NOT NULL,
    tier VARCHAR(20) DEFAULT 'free' CHECK (tier IN ('free', 'pro', 'enterprise')),
    rate_limit_per_hr INT DEFAULT 100,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_users_api_key_hash ON users (api_key_hash);

-- URLs Table (Optimized for Consistent Hashing / Sharding)
CREATE TABLE IF NOT EXISTS urls (
    id BIGSERIAL,
    short_code VARCHAR(10) NOT NULL,
    long_url TEXT NOT NULL,
    custom_alias BOOLEAN DEFAULT FALSE,
    user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    expires_at TIMESTAMPTZ,
    click_count BIGINT DEFAULT 0,
    is_active BOOLEAN DEFAULT TRUE,
    PRIMARY KEY (id, short_code)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_urls_short_code ON urls (short_code);
CREATE INDEX IF NOT EXISTS idx_urls_user_id ON urls (user_id);
CREATE INDEX IF NOT EXISTS idx_urls_expires_at ON urls (expires_at) WHERE expires_at IS NOT NULL;

-- Click Events Table (Partitioned by Range on Date)
CREATE TABLE IF NOT EXISTS click_events (
    id BIGSERIAL,
    short_code VARCHAR(10) NOT NULL,
    clicked_at TIMESTAMPTZ DEFAULT NOW(),
    ip_hash VARCHAR(64) NOT NULL,
    country_code CHAR(2) DEFAULT 'XX',
    user_agent TEXT,
    device_type VARCHAR(20),
    referrer TEXT,
    PRIMARY KEY (id, clicked_at)
) PARTITION BY RANGE (clicked_at);

-- Monthly Partitions
CREATE TABLE IF NOT EXISTS click_events_2026_08 PARTITION OF click_events
    FOR VALUES FROM ('2026-08-01 00:00:00+00') TO ('2026-09-01 00:00:00+00');

CREATE TABLE IF NOT EXISTS click_events_2026_09 PARTITION OF click_events
    FOR VALUES FROM ('2026-09-01 00:00:00+00') TO ('2026-10-01 00:00:00+00');

CREATE TABLE IF NOT EXISTS click_events_2026_10 PARTITION OF click_events
    FOR VALUES FROM ('2026-10-01 00:00:00+00') TO ('2026-11-01 00:00:00+00');

CREATE INDEX IF NOT EXISTS idx_click_events_code_time ON click_events (short_code, clicked_at DESC);
