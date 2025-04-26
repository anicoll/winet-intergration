package database

import (
	"context"
	"io"

	"github.com/jackc/pgx/v5"
)

type Database struct {
	conn *pgx.Conn
	io.Closer
}

func NewDatabase(ctx context.Context, conn *pgx.Conn) *Database {
initialise(ctx, conn)
	return &Database{
		conn: conn,
	}
}

func initialise(ctx context.Context, conn *pgx.Conn) {
	const createPropertiesTableSQL = `
CREATE TABLE IF NOT EXISTS Properties (
    Id SERIAL PRIMARY KEY,
    TimeStamp TIMESTAMP WITH TIME ZONE NOT NULL,
    Unit TEXT NOT NULL,
    Value TEXT NOT NULL,
    Identifier TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_properties_identifier ON Properties (Identifier);
CREATE INDEX IF NOT EXISTS idx_properties_timestamp ON Properties (TimeStamp);
`
	if _, err := conn.Exec(ctx, createPropertiesTableSQL); err != nil {
		panic(err)
	}

}
