-- name: UpsertAmberUsage :exec
INSERT INTO AmberUsage (per_kwh, spot_per_kwh, start_time, end_time, duration, channel_type, channel_identifier, kwh, quality, cost)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
ON CONFLICT (start_time, channel_identifier) DO UPDATE
SET per_kwh      = EXCLUDED.per_kwh,
    spot_per_kwh = EXCLUDED.spot_per_kwh,
    duration     = EXCLUDED.duration,
    kwh          = EXCLUDED.kwh,
    quality      = EXCLUDED.quality,
    cost         = EXCLUDED.cost,
    updated_at   = NOW();

-- name: GetAmberUsage :many
SELECT id, per_kwh, spot_per_kwh, start_time, end_time, duration, channel_type, channel_identifier, kwh, quality, cost, created_at, updated_at
FROM AmberUsage
WHERE start_time BETWEEN $1 AND $2
ORDER BY start_time DESC;
