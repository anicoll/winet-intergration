package migration

import (
	"database/sql"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/lib/pq"
)

const pgDriverName = "postgres"

func Migrate(dsn, folderPath string) error {
	db, err := sql.Open(pgDriverName, dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return err
	}
	m, err := migrate.NewWithDatabaseInstance(
		"file://"+folderPath,
		"postgres", driver)
	if err != nil {
		return err
	}
	return m.Up() // or m.Steps(2) if you want to explicitly set the number of migrations to ru
}
