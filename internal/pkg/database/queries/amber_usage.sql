-- name: UpsertAmberUsage :exec
MERGE AmberUsage AS target
USING (
    SELECT @p1  AS per_kwh,   @p2 AS spot_per_kwh, @p3 AS start_time,
           @p4  AS end_time,  @p5 AS duration,      @p6 AS channel_type,
           @p7  AS channel_identifier,               @p8 AS kwh,
           @p9  AS quality,   @p10 AS cost
) AS source
ON target.start_time = source.start_time AND target.channel_identifier = source.channel_identifier
WHEN MATCHED THEN
    UPDATE SET
        per_kwh      = source.per_kwh,
        spot_per_kwh = source.spot_per_kwh,
        duration     = source.duration,
        kwh          = source.kwh,
        quality      = source.quality,
        cost         = source.cost,
        updated_at   = SYSDATETIMEOFFSET()
WHEN NOT MATCHED THEN
    INSERT (per_kwh, spot_per_kwh, start_time, end_time, duration,
            channel_type, channel_identifier, kwh, quality, cost)
    VALUES (source.per_kwh, source.spot_per_kwh, source.start_time, source.end_time,
            source.duration, source.channel_type, source.channel_identifier,
            source.kwh, source.quality, source.cost);

-- name: GetAmberUsage :many
SELECT id, per_kwh, spot_per_kwh, start_time, end_time, duration,
       channel_type, channel_identifier, kwh, quality, cost, created_at, updated_at
FROM AmberUsage
WHERE start_time BETWEEN @p1 AND @p2
ORDER BY start_time DESC;
