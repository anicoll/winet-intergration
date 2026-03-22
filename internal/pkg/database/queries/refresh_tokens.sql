-- name: StoreRefreshToken :exec
INSERT INTO refresh_tokens (token_hash, user_id, username, expires_at)
VALUES ($1, $2, $3, $4)
ON CONFLICT (token_hash) DO UPDATE SET expires_at = EXCLUDED.expires_at;

-- name: GetRefreshToken :one
SELECT * FROM refresh_tokens WHERE token_hash = $1 LIMIT 1;

-- name: DeleteRefreshToken :exec
DELETE FROM refresh_tokens WHERE token_hash = $1;

-- name: DeleteExpiredRefreshTokens :exec
DELETE FROM refresh_tokens WHERE expires_at < NOW();
