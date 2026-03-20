package database

import (
	"github.com/jackc/pgx/v5/pgxpool"

	dbq "github.com/anicoll/winet-integration/internal/pkg/database/db"
)

type Database struct {
	pool    *pgxpool.Pool
	queries *dbq.Queries
}

func NewDatabase(pool *pgxpool.Pool) *Database {
	return &Database{
		pool:    pool,
		queries: dbq.New(pool),
	}
}
