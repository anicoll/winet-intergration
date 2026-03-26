CREATE TABLE Inverter (
    id             NVARCHAR(255)  NOT NULL PRIMARY KEY,
    state          NVARCHAR(MAX)  NOT NULL, -- e.g. "ON", "OFF", "FAULT"
    battery_state  NVARCHAR(MAX)  NOT NULL, -- e.g. "CHARGING", "DISCHARGING", "STOPPED", "SELF_CONSUMPTION"
    charge_rate    DECIMAL(3, 1)  NOT NULL,
    feedin_enabled BIT            NOT NULL DEFAULT 0,
    created_at     DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
    updated_at     DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET()
);
