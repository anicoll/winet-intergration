package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/golang-migrate/migrate/v4"

	"github.com/anicoll/winet-integration/internal/pkg/database/migration"
)

func main() {
	forceVersion := flag.Int("force", -1, "force database to this version (clears dirty state)")
	flag.Parse()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "DATABASE_URL is not set")
		os.Exit(1)
	}

	if *forceVersion >= 0 {
		if err := migration.Force(dsn, "./migrations", *forceVersion); err != nil {
			fmt.Fprintf(os.Stderr, "force failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("database forced to version %d\n", *forceVersion)
		return
	}

	if err := migration.Migrate(dsn, "./migrations"); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		fmt.Fprintf(os.Stderr, "migration failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("migrations applied successfully")
}
