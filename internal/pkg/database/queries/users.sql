-- name: GetUserByUsername :one
SELECT TOP 1 id, username, password_hash, created_at, updated_at
FROM users
WHERE username = @p1;

-- name: CreateUser :one
INSERT INTO users (username, password_hash)
OUTPUT INSERTED.id, INSERTED.username, INSERTED.password_hash, INSERTED.created_at, INSERTED.updated_at
VALUES (@p1, @p2);
