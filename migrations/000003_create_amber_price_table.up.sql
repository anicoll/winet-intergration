CREATE TABLE IF NOT EXISTS AmberPrice (
    id 				SERIAL PRIMARY KEY,
    per_kwh        	NUMERIC(10, 5) NOT NULL,
    spot_per_kwh    NUMERIC(10, 5) NOT NULL,
    start_time 		TIMESTAMP WITH TIME ZONE NOT NULL,
    end_time 		TIMESTAMP WITH TIME ZONE NOT NULL,
    duration 		INT NOT NULL,
    forecast 		BOOL NOT NULL DEFAULT FALSE,
    channel_type 	TEXT NOT NULL,
    created_at 		TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at 		TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_amber_price_start_time ON AmberPrice (start_time);
CREATE INDEX IF NOT EXISTS idx_amber_price_end_time ON AmberPrice (end_time);
CREATE UNIQUE INDEX IF NOT EXISTS idx_amber_price_unique_start_time_channel_type ON AmberPrice (start_time, channel_type);
