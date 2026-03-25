-- name: InsertPendingCommand :exec
INSERT INTO pending_commands (id, device_id, command_type, payload)
VALUES (@p1, @p2, @p3, @p4);

-- name: GetPendingCommands :many
-- Returns unacknowledged commands for a device, oldest first so they are
-- executed in the order they were queued.
SELECT id, device_id, command_type, payload, created_at
FROM pending_commands
WHERE device_id = @p1
  AND acked_at IS NULL
ORDER BY created_at ASC;

-- name: AckCommand :exec
UPDATE pending_commands
SET acked_at = SYSDATETIMEOFFSET(),
    success  = @p2
WHERE id = @p1;
