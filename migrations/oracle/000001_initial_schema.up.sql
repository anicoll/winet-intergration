CREATE TABLE Property (
    id                  NUMBER GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    time_stamp          TIMESTAMP WITH TIME ZONE NOT NULL,
    unit_of_measurement VARCHAR2(64) NOT NULL,
    value               VARCHAR2(256) NOT NULL,
    identifier          VARCHAR2(256) NOT NULL,
    slug                VARCHAR2(256) NOT NULL
);

CREATE INDEX idx_properties_identifier ON Property (identifier);
CREATE INDEX idx_properties_timestamp  ON Property (time_stamp);

CREATE TABLE Device (
    id            VARCHAR2(256) PRIMARY KEY,
    model         VARCHAR2(256),
    serial_number VARCHAR2(256),
    created_at    TIMESTAMP WITH TIME ZONE DEFAULT SYSTIMESTAMP NOT NULL
);

CREATE TABLE AmberPrice (
    id           NUMBER GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    per_kwh      NUMBER(10, 5) NOT NULL,
    spot_per_kwh NUMBER(10, 5) NOT NULL,
    start_time   TIMESTAMP WITH TIME ZONE NOT NULL,
    end_time     TIMESTAMP WITH TIME ZONE NOT NULL,
    duration     NUMBER NOT NULL,
    forecast     NUMBER(1) DEFAULT 0 NOT NULL,
    channel_type VARCHAR2(64) NOT NULL,
    created_at   TIMESTAMP WITH TIME ZONE DEFAULT SYSTIMESTAMP NOT NULL,
    updated_at   TIMESTAMP WITH TIME ZONE DEFAULT SYSTIMESTAMP NOT NULL
);

CREATE UNIQUE INDEX uq_amberprice ON AmberPrice (SYS_EXTRACT_UTC(start_time), channel_type);
CREATE INDEX idx_amber_price_start_time ON AmberPrice (start_time);
CREATE INDEX idx_amber_price_end_time   ON AmberPrice (end_time);

CREATE TABLE Inverter (
    id             VARCHAR2(256) PRIMARY KEY,
    state          VARCHAR2(64) NOT NULL,
    battery_state  VARCHAR2(64) NOT NULL,
    charge_rate    NUMBER(3, 1) NOT NULL,
    feedin_enabled NUMBER(1) DEFAULT 0 NOT NULL,
    created_at     TIMESTAMP WITH TIME ZONE DEFAULT SYSTIMESTAMP NOT NULL,
    updated_at     TIMESTAMP WITH TIME ZONE DEFAULT SYSTIMESTAMP NOT NULL
);

CREATE TABLE users (
    id            NUMBER GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    username      VARCHAR2(256) NOT NULL,
    password_hash VARCHAR2(256) NOT NULL,
    created_at    TIMESTAMP WITH TIME ZONE DEFAULT SYSTIMESTAMP NOT NULL,
    updated_at    TIMESTAMP WITH TIME ZONE DEFAULT SYSTIMESTAMP NOT NULL,
    CONSTRAINT uq_users_username UNIQUE (username)
);

CREATE TABLE refresh_tokens (
    token_hash VARCHAR2(512) PRIMARY KEY,
    user_id    NUMBER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    username   VARCHAR2(256) NOT NULL,
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT SYSTIMESTAMP NOT NULL
);

CREATE TABLE AmberUsage (
    id                 NUMBER GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    per_kwh            NUMBER(10, 5) NOT NULL,
    spot_per_kwh       NUMBER(10, 5) NOT NULL,
    start_time         TIMESTAMP WITH TIME ZONE NOT NULL,
    end_time           TIMESTAMP WITH TIME ZONE NOT NULL,
    duration           NUMBER NOT NULL,
    channel_type       VARCHAR2(64) NOT NULL,
    channel_identifier VARCHAR2(64) NOT NULL,
    kwh                NUMBER(10, 5) NOT NULL,
    quality            VARCHAR2(64) NOT NULL,
    cost               NUMBER(10, 5) NOT NULL,
    created_at         TIMESTAMP WITH TIME ZONE DEFAULT SYSTIMESTAMP NOT NULL,
    updated_at         TIMESTAMP WITH TIME ZONE DEFAULT SYSTIMESTAMP NOT NULL
);

CREATE UNIQUE INDEX uq_amberusage ON AmberUsage (SYS_EXTRACT_UTC(start_time), channel_identifier);
CREATE INDEX idx_amber_usage_start_time ON AmberUsage (start_time);
