-- name: UpsertAmberPrice :exec
MERGE AmberPrice AS target
USING (
    SELECT @p1 AS per_kwh, @p2 AS spot_per_kwh, @p3 AS start_time,
           @p4 AS end_time, @p5 AS duration, @p6 AS forecast, @p7 AS channel_type
) AS source
ON target.start_time = source.start_time AND target.channel_type = source.channel_type
WHEN MATCHED THEN
    UPDATE SET
        per_kwh      = source.per_kwh,
        spot_per_kwh = source.spot_per_kwh,
        duration     = source.duration,
        forecast     = source.forecast,
        updated_at   = SYSDATETIMEOFFSET()
WHEN NOT MATCHED THEN
    INSERT (per_kwh, spot_per_kwh, start_time, end_time, duration, forecast, channel_type)
    VALUES (source.per_kwh, source.spot_per_kwh, source.start_time, source.end_time,
            source.duration, source.forecast, source.channel_type);

-- name: GetAmberPrices :many
SELECT id, per_kwh, spot_per_kwh, start_time, end_time, duration, forecast,
       channel_type, created_at, updated_at
FROM AmberPrice
WHERE start_time BETWEEN @p1 AND @p2
ORDER BY start_time DESC;
