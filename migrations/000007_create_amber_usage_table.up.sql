CREATE TABLE IF NOT EXISTS AmberUsage (
    id                 SERIAL PRIMARY KEY,
    per_kwh            NUMERIC(10, 5) NOT NULL,
    spot_per_kwh       NUMERIC(10, 5) NOT NULL,
    start_time         TIMESTAMP WITH TIME ZONE NOT NULL,
    end_time           TIMESTAMP WITH TIME ZONE NOT NULL,
    duration           INT NOT NULL,
    channel_type       TEXT NOT NULL,
    channel_identifier TEXT NOT NULL,
    kwh                NUMERIC(10, 5) NOT NULL,
    quality            TEXT NOT NULL,
    cost               NUMERIC(10, 5) NOT NULL,
    created_at         TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at         TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_amber_usage_start_time ON AmberUsage (start_time);
CREATE UNIQUE INDEX IF NOT EXISTS idx_amber_usage_unique_start_time_channel ON AmberUsage (start_time, channel_identifier);
