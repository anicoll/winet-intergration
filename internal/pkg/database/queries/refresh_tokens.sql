-- name: StoreRefreshToken :exec
MERGE refresh_tokens AS target
USING (
    SELECT @p1 AS token_hash, @p2 AS user_id, @p3 AS username, @p4 AS expires_at
) AS source
ON target.token_hash = source.token_hash
WHEN MATCHED THEN
    UPDATE SET expires_at = source.expires_at
WHEN NOT MATCHED THEN
    INSERT (token_hash, user_id, username, expires_at)
    VALUES (source.token_hash, source.user_id, source.username, source.expires_at);

-- name: GetRefreshToken :one
SELECT TOP 1 token_hash, user_id, username, expires_at, created_at
FROM refresh_tokens
WHERE token_hash = @p1;

-- name: DeleteRefreshToken :exec
DELETE FROM refresh_tokens WHERE token_hash = @p1;

-- name: DeleteExpiredRefreshTokens :exec
DELETE FROM refresh_tokens WHERE expires_at < SYSDATETIMEOFFSET();
