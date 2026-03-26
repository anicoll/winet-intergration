CREATE TABLE AmberUsage (
    id                 INT IDENTITY(1,1)  PRIMARY KEY,
    per_kwh            DECIMAL(10, 5)     NOT NULL,
    spot_per_kwh       DECIMAL(10, 5)     NOT NULL,
    start_time         DATETIMEOFFSET     NOT NULL,
    end_time           DATETIMEOFFSET     NOT NULL,
    duration           INT                NOT NULL,
    channel_type       NVARCHAR(255)      NOT NULL,
    channel_identifier NVARCHAR(255)      NOT NULL,
    kwh                DECIMAL(10, 5)     NOT NULL,
    quality            NVARCHAR(255)      NOT NULL,
    cost               DECIMAL(10, 5)     NOT NULL,
    created_at         DATETIMEOFFSET     NOT NULL DEFAULT SYSDATETIMEOFFSET(),
    updated_at         DATETIMEOFFSET     NOT NULL DEFAULT SYSDATETIMEOFFSET()
);

CREATE INDEX idx_amber_usage_start_time ON AmberUsage (start_time);
CREATE UNIQUE INDEX idx_amber_usage_unique_start_time_channel ON AmberUsage (start_time, channel_identifier);
