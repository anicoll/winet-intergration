-- name: InsertProperty :one
INSERT INTO Property (time_stamp, unit_of_measurement, value, identifier, slug)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, time_stamp, unit_of_measurement, value, identifier, slug;

-- name: GetProperties :many
SELECT id, time_stamp, unit_of_measurement, value, identifier, slug
FROM Property
WHERE identifier = $1 AND slug = $2 AND time_stamp BETWEEN $3 AND $4
ORDER BY time_stamp DESC;

-- name: GetLatestProperties :many
SELECT DISTINCT ON (identifier, slug)
    id, time_stamp, unit_of_measurement, value, identifier, slug
FROM Property
WHERE time_stamp > NOW() - INTERVAL '1 day'
ORDER BY identifier, slug, time_stamp DESC;

-- name: CleanupProperties :exec
DELETE FROM Property WHERE time_stamp < $1;
