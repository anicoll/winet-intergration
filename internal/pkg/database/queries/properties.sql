-- name: InsertProperty :one
INSERT INTO Property (time_stamp, unit_of_measurement, value, identifier, slug)
OUTPUT INSERTED.id, INSERTED.time_stamp, INSERTED.unit_of_measurement, INSERTED.value, INSERTED.identifier, INSERTED.slug
VALUES (@p1, @p2, @p3, @p4, @p5);

-- name: GetProperties :many
SELECT id, time_stamp, unit_of_measurement, value, identifier, slug
FROM Property
WHERE identifier = @p1
  AND slug       = @p2
  AND time_stamp BETWEEN @p3 AND @p4
ORDER BY time_stamp DESC;

-- name: GetLatestProperties :many
SELECT id, time_stamp, unit_of_measurement, value, identifier, slug
FROM (
    SELECT id, time_stamp, unit_of_measurement, value, identifier, slug,
           ROW_NUMBER() OVER (PARTITION BY identifier, slug ORDER BY time_stamp DESC) AS rn
    FROM Property
    WHERE time_stamp > DATEADD(day, -1, SYSDATETIMEOFFSET())
) t
WHERE rn = 1;

-- name: CleanupProperties :exec
DELETE FROM Property WHERE time_stamp < @p1;
