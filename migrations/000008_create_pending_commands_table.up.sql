-- pending_commands queues inverter commands submitted by the cloud REST API.
-- The local service polls GetPendingCommands, executes each command against the
-- WiNet inverter, then calls AckCommand to record the outcome.
CREATE TABLE pending_commands (
    id           NVARCHAR(36)   NOT NULL PRIMARY KEY DEFAULT NEWID(),
    device_id    NVARCHAR(255)  NOT NULL,
    command_type NVARCHAR(100)  NOT NULL, -- matches the InverterCommand oneof field name
    payload      NVARCHAR(MAX)  NOT NULL, -- JSON-encoded command-specific parameters
    created_at   DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
    acked_at     DATETIMEOFFSET NULL,
    success      BIT            NULL      -- NULL until acked
);

-- Index used by GetPendingCommands to efficiently find unacknowledged commands per device.
CREATE INDEX idx_pending_commands_device_acked ON pending_commands (device_id, acked_at);
