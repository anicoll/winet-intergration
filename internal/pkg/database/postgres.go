package database

import (
	"context"
	"database/sql"
)

type Database struct {
	db *sql.DB
}

func NewDatabase(ctx context.Context, db *sql.DB) *Database {
	return &Database{
		db: db,
	}
}
