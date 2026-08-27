-- ==============================================================================
-- NanoLink ClickHouse Real-Time Analytics Schema
-- Columnar storage with AggregatingMergeTree and Materialized Views
-- ==============================================================================

CREATE DATABASE IF NOT EXISTS nanolink_analytics;

-- Raw Click Stream Log Table
CREATE TABLE IF NOT EXISTS nanolink_analytics.clicks (
    short_code LowCardinality(String),
    clicked_at DateTime64(3, 'UTC'),
    ip_hash FixedString(64),
    country_code LowCardinality(FixedString(2)),
    city String,
    device_type LowCardinality(String),
    browser LowCardinality(String),
    os LowCardinality(String),
    referrer String
) ENGINE = MergeTree()
PARTITION BY toYYYYMM(clicked_at)
ORDER BY (short_code, clicked_at, country_code);

-- Materialized Aggregation Table for Real-Time Rollups
CREATE TABLE IF NOT EXISTS nanolink_analytics.hourly_clicks_mv (
    short_code LowCardinality(String),
    hour DateTime,
    country_code LowCardinality(FixedString(2)),
    device_type LowCardinality(String),
    total_clicks SimpleAggregateFunction(sum, UInt64)
) ENGINE = AggregatingMergeTree()
PARTITION BY toYYYYMM(hour)
ORDER BY (short_code, hour, country_code, device_type);

-- Materialized View Trigger for Auto-Aggregation on Ingestion
CREATE MATERIALIZED VIEW IF NOT EXISTS nanolink_analytics.clicks_to_hourly_mv
TO nanolink_analytics.hourly_clicks_mv AS
SELECT
    short_code,
    toStartOfHour(clicked_at) AS hour,
    country_code,
    device_type,
    count() AS total_clicks
FROM nanolink_analytics.clicks
GROUP BY short_code, hour, country_code, device_type;
