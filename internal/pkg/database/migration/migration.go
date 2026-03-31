package migration

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	_ "github.com/sijms/go-ora/v2"
)

// MigratePostgres runs up migrations against an existing pgxpool.Pool.
func MigratePostgres(pool *pgxpool.Pool, folderPath string) error {
	db := stdlib.OpenDBFromPool(pool)
	defer func() { _ = db.Close() }()

	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("postgres migration: create driver: %w", err)
	}

	m, err := migrate.NewWithDatabaseInstance("file://"+folderPath, "postgres", driver)
	if err != nil {
		return fmt.Errorf("postgres migration: init: %w", err)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("postgres migration: up: %w", err)
	}
	return nil
}

// MigrateOracle runs up migrations against an existing *sql.DB opened with the oracle driver.
func MigrateOracle(db *sql.DB, folderPath string) error {
	ctx := context.Background()

	if _, err := db.ExecContext(ctx, `
		BEGIN
			EXECUTE IMMEDIATE 'CREATE TABLE schema_migrations (
				version   VARCHAR2(256) PRIMARY KEY,
				applied_at TIMESTAMP WITH TIME ZONE DEFAULT SYSTIMESTAMP NOT NULL
			)';
		EXCEPTION
			WHEN OTHERS THEN
				IF SQLCODE != -955 THEN RAISE; END IF;
		END;`); err != nil {
		return fmt.Errorf("oracle migration: ensure schema_migrations: %w", err)
	}

	entries, err := os.ReadDir(folderPath)
	if err != nil {
		return fmt.Errorf("oracle migration: read dir: %w", err)
	}

	var upFiles []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".up.sql") {
			upFiles = append(upFiles, e.Name())
		}
	}
	sort.Strings(upFiles)

	for _, name := range upFiles {
		version := strings.TrimSuffix(name, ".up.sql")

		var count int
		if err := db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM schema_migrations WHERE version = :1`, version,
		).Scan(&count); err != nil {
			return fmt.Errorf("oracle migration: check %s: %w", version, err)
		}
		if count > 0 {
			continue
		}

		content, err := os.ReadFile(filepath.Join(folderPath, name))
		if err != nil {
			return fmt.Errorf("oracle migration: read %s: %w", name, err)
		}

		for _, stmt := range splitStatements(string(content)) {
			if _, err := db.ExecContext(ctx, stmt); err != nil {
				return fmt.Errorf("oracle migration: apply %s: %w\nSQL: %s", name, err, stmt)
			}
		}

		if _, err := db.ExecContext(ctx,
			`INSERT INTO schema_migrations (version) VALUES (:1)`, version,
		); err != nil {
			return fmt.Errorf("oracle migration: record %s: %w", version, err)
		}
	}
	return nil
}

// splitStatements splits a SQL file containing multiple statements separated by
// semicolons, returning only non-empty trimmed statements.
func splitStatements(sql string) []string {
	var out []string
	for s := range strings.SplitSeq(sql, ";") {
		if trimmed := strings.TrimSpace(s); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
