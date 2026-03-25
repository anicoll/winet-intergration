CREATE TABLE AmberPrice (
    id           INT IDENTITY(1,1)  PRIMARY KEY,
    per_kwh      DECIMAL(10, 5)     NOT NULL,
    spot_per_kwh DECIMAL(10, 5)     NOT NULL,
    start_time   DATETIMEOFFSET     NOT NULL,
    end_time     DATETIMEOFFSET     NOT NULL,
    duration     INT                NOT NULL,
    forecast     BIT                NOT NULL DEFAULT 0,
    channel_type NVARCHAR(255)      NOT NULL,
    created_at   DATETIMEOFFSET     NOT NULL DEFAULT SYSDATETIMEOFFSET(),
    updated_at   DATETIMEOFFSET     NOT NULL DEFAULT SYSDATETIMEOFFSET()
);

CREATE INDEX idx_amber_price_start_time ON AmberPrice (start_time);
CREATE INDEX idx_amber_price_end_time   ON AmberPrice (end_time);
CREATE UNIQUE INDEX idx_amber_price_unique_start_time_channel_type ON AmberPrice (start_time, channel_type);
