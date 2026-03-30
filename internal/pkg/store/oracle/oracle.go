// Package oracle is the Oracle Database implementation of store.Store.
// It uses the standard database/sql interface with the go-ora/v2 pure-Go driver.
//
// Connection DSN format: oracle://user:password@host:port/service_name
//
// The driver is registered automatically by importing this package.
// Migrations are not run automatically — apply the Oracle DDL separately before
// starting the application.
package oracle

import (
	"database/sql"

	// Register the "oracle" driver with database/sql.
	_ "github.com/sijms/go-ora/v2"

	"github.com/anicoll/winet-integration/internal/pkg/store"
)

// compile-time check that *Store satisfies store.Store.
var _ store.Store = (*Store)(nil)

// Store is the Oracle-backed implementation of store.Store.
type Store struct {
	db *sql.DB
}

// New creates a Store from an existing *sql.DB opened with the "oracle" driver.
func New(db *sql.DB) *Store {
	return &Store{db: db}
}
