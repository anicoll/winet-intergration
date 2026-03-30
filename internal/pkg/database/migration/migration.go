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
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/sijms/go-ora/v2"
)

// Migrate runs up migrations for the given driver.
//
// For "postgres" it uses golang-migrate with the pgx/v5 driver.
// For "oracle" it applies SQL files directly via database/sql, tracking applied
// migrations in a schema_migrations table (golang-migrate has no Oracle driver).
func Migrate(driver, dsn, folderPath string) error {
	switch driver {
	case "oracle":
		return migrateOracle(dsn, folderPath)
	default:
		return migratePostgres(dsn, folderPath)
	}
}

func migratePostgres(dsn, folderPath string) error {
	m, err := migrate.New("file://"+folderPath, dsn)
	if err != nil {
		return err
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return err
	}
	return nil
}

// splitStatements splits a SQL file containing multiple statements separated by
// semicolons, returning only non-empty trimmed statements. Oracle requires each
// statement to be executed individually.
func splitStatements(sql string) []string {
	var out []string
	for s := range strings.SplitSeq(sql, ";") {
		if trimmed := strings.TrimSpace(s); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// migrateOracle applies *.up.sql files in lexicographic order, skipping any
// that have already been recorded in the schema_migrations table.
func migrateOracle(dsn, folderPath string) error {
	ctx := context.Background()

	db, err := sql.Open("oracle", dsn)
	if err != nil {
		return fmt.Errorf("oracle migration: open: %w", err)
	}
	defer func() { _ = db.Close() }()

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
