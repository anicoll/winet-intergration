package migration

import (
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/sqlserver"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

func Migrate(dsn, folderPath string) error {
	m, err := migrate.New("file://"+folderPath, dsn)
	if err != nil {
		return err
	}
	return m.Up()
}

func Force(dsn, folderPath string, version int) error {
	m, err := migrate.New("file://"+folderPath, dsn)
	if err != nil {
		return err
	}
	return m.Force(version)
}
