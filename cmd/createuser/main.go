// createuser is a local CLI tool for adding users to the database.
// It is never exposed as a network endpoint — run it directly on the host.
//
// Usage:
//
//	DATABASE_URL=postgres://... ./createuser --username alice --password supersecret123
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	dbpkg "github.com/anicoll/winet-integration/internal/pkg/database/db"
	"github.com/anicoll/winet-integration/pkg/hasher"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	username := flag.String("username", "", "username (required)")
	password := flag.String("password", "", "password — min 12 characters (required)")
	flag.Parse()

	if *username == "" {
		return fmt.Errorf("--username is required")
	}
	if len(*password) < 12 {
		return fmt.Errorf("--password must be at least 12 characters")
	}

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return fmt.Errorf("DATABASE_URL environment variable is required")
	}

	hash, err := hasher.HashPassword([]byte(*password))
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}

	q := dbpkg.New(pool)
	user, err := q.CreateUser(ctx, dbpkg.CreateUserParams{
		Username:     *username,
		PasswordHash: hash,
	})
	if err != nil {
		return fmt.Errorf("create user: %w", err)
	}

	fmt.Printf("created user %q (id=%d)\n", user.Username, user.ID)
	return nil
}
