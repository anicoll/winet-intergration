CREATE TABLE refresh_tokens (
    token_hash NVARCHAR(255)  NOT NULL PRIMARY KEY,
    user_id    INT            NOT NULL,
    username   NVARCHAR(255)  NOT NULL,
    expires_at DATETIMEOFFSET NOT NULL,
    created_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
    CONSTRAINT fk_refresh_tokens_users FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
