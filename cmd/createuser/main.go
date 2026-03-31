// createuser is a local CLI tool for adding users to the database.
// It is never exposed as a network endpoint — run it directly on the host.
//
// Usage (postgres):
//
//	DB_DRIVER=postgres DATABASE_URL=postgres://... ./createuser --username alice --password supersecret123
//
// Usage (oracle):
//
//	DB_DRIVER=oracle ORACLE_HOST=... ORACLE_SERVICE=... ORACLE_USER=... ORACLE_PASSWORD=... ./createuser --username alice --password supersecret123
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	go_ora "github.com/sijms/go-ora/v2"

	"github.com/anicoll/winet-integration/internal/pkg/config"
	orastore "github.com/anicoll/winet-integration/internal/pkg/store/oracle"
	pgstore "github.com/anicoll/winet-integration/internal/pkg/store/postgres"
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

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	hash, err := hasher.HashPassword([]byte(*password))
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	ctx := context.Background()

	switch cfg.DBDriver {
	case "oracle":
		if cfg.OracleCfg.Host == "" || cfg.OracleCfg.Service == "" || cfg.OracleCfg.User == "" || cfg.OracleCfg.Password == "" {
			return fmt.Errorf("ORACLE_HOST, ORACLE_SERVICE, ORACLE_USER and ORACLE_PASSWORD are required when DB_DRIVER=oracle")
		}
		urlOptions := map[string]string{"ssl": "true"}
		if !cfg.OracleCfg.SSLVerify {
			urlOptions["ssl verify"] = "false"
		}
		if cfg.OracleCfg.WalletPath != "" {
			urlOptions["wallet"] = cfg.OracleCfg.WalletPath
		}
		connStr := go_ora.BuildUrl(cfg.OracleCfg.Host, cfg.OracleCfg.Port, cfg.OracleCfg.Service, cfg.OracleCfg.User, cfg.OracleCfg.Password, urlOptions)
		db, err := sql.Open("oracle", connStr)
		if err != nil {
			return fmt.Errorf("connect to oracle: %w", err)
		}
		defer func() { _ = db.Close() }()
		if err := db.PingContext(ctx); err != nil {
			return fmt.Errorf("ping oracle: %w", err)
		}
		st := orastore.New(db)
		user, err := st.CreateUser(ctx, *username, hash)
		if err != nil {
			return fmt.Errorf("create user: %w", err)
		}
		fmt.Printf("created user %q (id=%d)\n", user.Username, user.ID)

	default: // postgres
		if cfg.DBDSN == "" {
			return fmt.Errorf("DATABASE_URL is required when DB_DRIVER=postgres")
		}
		pool, err := pgxpool.New(ctx, cfg.DBDSN)
		if err != nil {
			return fmt.Errorf("connect to database: %w", err)
		}
		defer pool.Close()
		if err := pool.Ping(ctx); err != nil {
			return fmt.Errorf("ping database: %w", err)
		}
		st := pgstore.New(pool)
		user, err := st.CreateUser(ctx, *username, hash)
		if err != nil {
			return fmt.Errorf("create user: %w", err)
		}
		fmt.Printf("created user %q (id=%d)\n", user.Username, user.ID)
	}

	return nil
}
