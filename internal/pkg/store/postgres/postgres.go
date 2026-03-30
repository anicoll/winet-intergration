// Package postgres is the PostgreSQL implementation of store.Store.
// It wraps a pgxpool connection and the sqlc-generated query layer.
// Wire it with New(pool) and pass the result as a store.Store anywhere.
package postgres

import (
	"github.com/jackc/pgx/v5/pgxpool"

	dbq "github.com/anicoll/winet-integration/internal/pkg/database/db"
	"github.com/anicoll/winet-integration/internal/pkg/store"
)

// compile-time check that *Store satisfies store.Store.
var _ store.Store = (*Store)(nil)

// Store is the PostgreSQL-backed implementation of store.Store.
type Store struct {
	pool    *pgxpool.Pool
	queries *dbq.Queries
}

// New creates a Store from an existing connection pool.
func New(pool *pgxpool.Pool) *Store {
	return &Store{
		pool:    pool,
		queries: dbq.New(pool),
	}
}
