-- name: UpsertAmberPrice :exec
INSERT INTO AmberPrice (per_kwh, spot_per_kwh, start_time, end_time, duration, forecast, channel_type)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (start_time, channel_type) DO UPDATE
SET per_kwh      = EXCLUDED.per_kwh,
    spot_per_kwh = EXCLUDED.spot_per_kwh,
    duration     = EXCLUDED.duration,
    forecast     = EXCLUDED.forecast,
    updated_at   = NOW();

-- name: GetAmberPrices :many
SELECT id, per_kwh, spot_per_kwh, start_time, end_time, duration, forecast, channel_type, created_at, updated_at
FROM AmberPrice
WHERE start_time BETWEEN $1 AND $2
ORDER BY start_time DESC;
