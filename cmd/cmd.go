// Package cmd provides the main command implementation for the winet integration service.
package cmd

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/gorilla/mux"
	_ "github.com/jackc/pgx/v5/stdlib" // pgx as database/sql driver
	"github.com/robfig/cron/v3"
	"github.com/urfave/cli/v2"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	"github.com/anicoll/winet-integration/internal/pkg/amber"
	"github.com/anicoll/winet-integration/internal/pkg/config"
	"github.com/anicoll/winet-integration/internal/pkg/database"
	"github.com/anicoll/winet-integration/internal/pkg/database/migration"
	"github.com/anicoll/winet-integration/internal/pkg/models"
	"github.com/anicoll/winet-integration/internal/pkg/publisher"
	"github.com/anicoll/winet-integration/internal/pkg/server"
	"github.com/anicoll/winet-integration/internal/pkg/winet"
	api "github.com/anicoll/winet-integration/pkg/server"
)

var errCron = errors.New("cron error")

const (
	// Server configuration
	serverAddr         = "0.0.0.0:8000"
	serverWriteTimeout = 5 * time.Second
	serverReadTimeout  = 5 * time.Second

	// Channel buffer sizes
	errorChannelBuffer = 1000

	// Cron schedules
	dbCleanupSchedule   = "CRON_TZ=Australia/Adelaide 0 3 * * *"
	priceUpdateSchedule = "CRON_TZ=Australia/Adelaide */5 * * * *"

	// Delays
	priceUpdateDelay = 5 * time.Second
)

// WinetCommand is the main entry point for the winet integration CLI command.
// It validates configuration and starts all required services.
func WinetCommand(ctx *cli.Context) error {
	cfg := &config.Config{
		WinetCfg: &config.WinetConfig{
			Password:     ctx.String("winet-password"),
			Username:     ctx.String("winet-username"),
			Host:         ctx.String("winet-host"),
			Ssl:          ctx.Bool("winet-ssl"),
			PollInterval: ctx.Duration("poll-interval"),
		},
		DBDSN:            ctx.String("database-url"),
		MigrationsFolder: ctx.String("migrations-folder"),
		MqttCfg: &config.WinetConfig{
			Host:     ctx.String("mqtt-host"),
			Username: ctx.String("mqtt-user"),
			Password: ctx.String("mqtt-pass"),
		},
		LogLevel: ctx.String("log-level"),
	}

	if err := validateConfig(cfg); err != nil {
		return fmt.Errorf("configuration validation failed: %w", err)
	}

	return run(ctx.Context, cfg)
}

// validateConfig ensures all required configuration values are present.
func validateConfig(cfg *config.Config) error {
	if cfg.WinetCfg.Host == "" {
		return errors.New("winet host is required")
	}
	if cfg.WinetCfg.Username == "" {
		return errors.New("winet username is required")
	}
	if cfg.WinetCfg.Password == "" {
		return errors.New("winet password is required")
	}
	return nil
}

// run orchestrates all services and handles graceful shutdown.
func run(ctx context.Context, cfg *config.Config) error {
	// Setup graceful shutdown
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer cancel()

	logger, err := setupLogger(cfg.LogLevel)
	if err != nil {
		return fmt.Errorf("failed to setup logger: %w", err)
	}
	defer func() {
		_ = logger.Sync() // flushes buffer, if any.
	}()

	// Initialize database connection
	db, cleanup, err := setupDatabase(ctx, cfg.DBDSN, cfg.MigrationsFolder)
	if err != nil {
		return fmt.Errorf("failed to setup database: %w", err)
	}
	_ = db
	defer cleanup()

	// Register publishers
	if err := publisher.RegisterPublisher("postgres", db); err != nil {
		return fmt.Errorf("failed to register postgres publisher: %w", err)
	}

	// // Setup error channel with buffer
	errorChan := make(chan error, errorChannelBuffer)
	winetSvc := winet.New(cfg.WinetCfg, errorChan)

	// // Start all services
	eg, ctx := errgroup.WithContext(ctx)

	// Start database cleanup service
	eg.Go(func() error {
		return startDbCleanupService(ctx, db, errorChan, logger)
	})

	// Start amber price processing service
	eg.Go(func() error {
		return startAmberPriceService(ctx, db, errorChan, logger)
	})

	// Start winet service with retry logic
	eg.Go(func() error {
		return startWinetService(ctx, winetSvc, errorChan, logger)
	})

	// Start HTTP server
	eg.Go(func() error {
		return startHTTPServer(ctx, winetSvc, db, logger)
	})

	// Start error handler
	eg.Go(func() error {
		return handleErrors(ctx, errorChan, logger)
	})

	logger.Info("All services started successfully")
	return eg.Wait()
}

func setupLogger(logLevel string) (*zap.Logger, error) {
	logCfg := zap.NewProductionConfig()

	level, err := zap.ParseAtomicLevel(logLevel)
	if err != nil {
		return nil, err
	}

	logCfg.Level = level
	logCfg.OutputPaths = []string{"stdout"}
	logCfg.ErrorOutputPaths = []string{"stdout"}
	logCfg.Sampling = nil

	logger := zap.Must(logCfg.Build(zap.AddCaller(), zap.AddStacktrace(zap.ErrorLevel)))
	zap.ReplaceGlobals(logger)

	return logger, nil
}

func setupDatabase(ctx context.Context, dsn, migrationsPath string) (*database.Database, func(), error) {
	err := migration.Migrate(dsn, migrationsPath)
	if err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return nil, nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, nil, err
	}
	if err := sqlDB.PingContext(ctx); err != nil {
		return nil, nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	db := database.NewDatabase(ctx, sqlDB)
	cleanup := func() {
		sqlDB.Close()
	}

	return db, cleanup, nil
}

func startDbCleanupService(ctx context.Context, db *database.Database, errChan chan error, logger *zap.Logger) error {
	logger.Info("Starting database cleanup service")

	// Setup cron job
	c := cron.New()
	if _, err := c.AddFunc(dbCleanupSchedule, func() {
		if err := db.Cleanup(context.Background()); err != nil {
			logger.Error("database cleanup failed", zap.Error(err))
			select {
			case errChan <- fmt.Errorf("%w: %v", errCron, err):
			default:
				logger.Warn("error channel full, dropping error")
			}
			return
		}
		logger.Info("database cleanup completed")
	}); err != nil {
		return fmt.Errorf("failed to schedule database cleanup: %w", err)
	}

	c.Start()

	// Wait for context cancellation
	<-ctx.Done()
	c.Stop()
	logger.Info("Database cleanup service stopped")
	return ctx.Err()
}

func startAmberPriceService(ctx context.Context, db *database.Database, errChan chan error, logger *zap.Logger) error {
	logger.Info("Starting amber price service")

	amberHost := os.Getenv("AMBER_HOST")
	amberToken := os.Getenv("AMBER_TOKEN")

	if amberHost == "" || amberToken == "" {
		return errors.New("AMBER_HOST and AMBER_TOKEN environment variables are required")
	}

	svc, err := amber.New(ctx, amberHost, amberToken)
	if err != nil {
		return fmt.Errorf("failed to create amber service: %w", err)
	}

	sites := svc.GetSites()
	if len(sites) == 0 {
		return errors.New("no amber sites available")
	}
	site := sites[0]

	// Initial price fetch
	if err := fetchAndStorePrices(ctx, svc, db, site.Id); err != nil {
		return fmt.Errorf("initial price fetch failed: %w", err)
	}

	// Setup cron job
	c := cron.New()
	if _, err := c.AddFunc(priceUpdateSchedule, func() {
		time.Sleep(priceUpdateDelay) // ensure we get the latest prices
		if err := fetchAndStorePrices(context.Background(), svc, db, site.Id); err != nil {
			logger.Error("amber price update failed", zap.Error(err))
			select {
			case errChan <- fmt.Errorf("%w: %v", errCron, err):
			default:
				logger.Warn("error channel full, dropping error")
			}
			return
		}
		logger.Info("amber prices updated")
	}); err != nil {
		return fmt.Errorf("failed to schedule amber price updates: %w", err)
	}

	c.Start()

	// Wait for context cancellation
	<-ctx.Done()
	c.Stop()
	logger.Info("Amber price service stopped")
	return ctx.Err()
}

func fetchAndStorePrices(ctx context.Context, svc interface {
	GetPrices(ctx context.Context, siteID string) ([]models.Amberprice, error)
}, db *database.Database, siteId string,
) error {
	prices, err := svc.GetPrices(ctx, siteId)
	if err != nil {
		return fmt.Errorf("failed to get prices: %w", err)
	}

	if err := db.WriteAmberPrices(ctx, prices); err != nil {
		return fmt.Errorf("failed to write prices to database: %w", err)
	}

	return nil
}

type winetSvc interface {
	Connect(ctx context.Context) error
	SubscribeToTimeout() <-chan error
}

func startWinetService(ctx context.Context, winetSvc winetSvc, errChan chan error, logger *zap.Logger) error {
	logger.Info("Starting winet service")

	for {
		select {
		case <-ctx.Done():
			logger.Info("Winet service stopped")
			return ctx.Err()
		default:
		}

		if err := winetSvc.Connect(ctx); err != nil {
			logger.Error("winet connection failed", zap.Error(err))
			// Add exponential backoff here if needed
			time.Sleep(5 * time.Second)
			continue
		}

		logger.Info("Winet service connected successfully")

		// Wait for timeout or context cancellation
		select {
		case err := <-winetSvc.SubscribeToTimeout():
			if errors.Is(err, winet.ErrTimeout) {
				logger.Warn("winet timeout occurred, reconnecting", zap.Error(err))
				continue
			}
			logger.Error("winet service error", zap.Error(err))
			return err
		case <-ctx.Done():
			logger.Info("Winet service stopped")
			return ctx.Err()
		}
	}
}

func startHTTPServer(ctx context.Context, winetSvc server.WinetService, db *database.Database, logger *zap.Logger) error {
	logger.Info("Starting HTTP server", zap.String("addr", serverAddr))

	r := mux.NewRouter()
	r.Use(mux.CORSMethodMiddleware(r))
	apiHandler := api.HandlerWithOptions(server.New(winetSvc, db), api.GorillaServerOptions{
		BaseRouter:  r,
		Middlewares: []api.MiddlewareFunc{server.TimeoutMiddleware, server.LoggingMiddleware},
		ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			logger.Error("HTTP handler error", zap.Error(err))
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		},
	})

	srv := &http.Server{
		Handler:      apiHandler,
		Addr:         serverAddr,
		WriteTimeout: serverWriteTimeout,
		ReadTimeout:  serverReadTimeout,
	}

	// Start server in a goroutine
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("HTTP server error", zap.Error(err))
		}
	}()

	// Wait for context cancellation
	<-ctx.Done()

	// Graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	logger.Info("Shutting down HTTP server gracefully")
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("HTTP server shutdown error", zap.Error(err))
		return err
	}

	logger.Info("HTTP server stopped")
	return ctx.Err()
}

func handleErrors(ctx context.Context, errorChan chan error, logger *zap.Logger) error {
	logger.Info("Starting error handler")

	for {
		select {
		case err := <-errorChan:
			if errors.Is(err, errCron) {
				logger.Error("cron job error", zap.Error(err))
				// For cron errors, we might want to continue instead of failing
				continue
			}
			logger.Error("service error received", zap.Error(err))
			return err
		case <-ctx.Done():
			logger.Info("Error handler stopped")
			return ctx.Err()
		}
	}
}
