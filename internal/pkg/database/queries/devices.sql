-- name: UpsertDevice :exec
INSERT INTO Device (id, model, serial_number)
VALUES ($1, $2, $3)
ON CONFLICT (id) DO NOTHING;
