CREATE TABLE users (
    id            INT IDENTITY(1,1)  PRIMARY KEY,
    username      NVARCHAR(255)      NOT NULL CONSTRAINT uq_users_username UNIQUE,
    password_hash NVARCHAR(MAX)      NOT NULL,
    created_at    DATETIMEOFFSET     NOT NULL DEFAULT SYSDATETIMEOFFSET(),
    updated_at    DATETIMEOFFSET     NOT NULL DEFAULT SYSDATETIMEOFFSET()
);
