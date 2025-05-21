package cmd

import (
	"context"
	"errors"
	"net/http"
	"os"
	"time"

	"github.com/anicoll/winet-integration/internal/pkg/amber"
	"github.com/anicoll/winet-integration/internal/pkg/config"
	"github.com/anicoll/winet-integration/internal/pkg/database"
	"github.com/anicoll/winet-integration/internal/pkg/publisher"
	"github.com/anicoll/winet-integration/internal/pkg/server"
	"github.com/anicoll/winet-integration/internal/pkg/winet"
	api "github.com/anicoll/winet-integration/pkg/server"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/robfig/cron/v3"
	"github.com/urfave/cli/v2"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

func WinetCommand(ctx *cli.Context) error {
	cfg := &config.Config{
		WinetCfg: &config.WinetConfig{
			Password:     ctx.String("winet-password"),
			Username:     ctx.String("winet-username"),
			Host:         ctx.String("winet-host"),
			Ssl:          ctx.Bool("winet-ssl"),
			PollInterval: ctx.Duration("poll-interval"),
		},
		MqttCfg: &config.WinetConfig{
			Host:     ctx.String("mqtt-host"),
			Username: ctx.String("mqtt-user"),
			Password: ctx.String("mqtt-pass"),
		},
		LogLevel: ctx.String("log-level"),
	}

	// Call the main application runner which sets up dependencies.
	return runWinetApp(ctx.Context, cfg)
}

// runWinetApp sets up the application's dependencies and starts the core logic.
func runWinetApp(appCtx context.Context, cfg *config.Config) error {
	errorChan := make(chan error) // Unbuffered channel for non-critical errors
	logCfg := zap.NewProductionConfig()
	var err error

	// Setup Logger
	logLevel, err := zap.ParseAtomicLevel(cfg.LogLevel)
	if err != nil {
		// Fallback to InfoLevel if parsing fails, and log this occurrence.
		// Using fmt.Println here as logger might not be initialized.
		fmt.Printf("Failed to parse log level '%s': %v. Defaulting to INFO.\n", cfg.LogLevel, err)
		logLevel = zap.InfoLevel
	}
	logCfg.Level = logLevel
	logCfg.OutputPaths = []string{"stdout"}
	logCfg.ErrorOutputPaths = []string{"stdout"}
	logCfg.Sampling = nil // Disable sampling for more consistent logs
	logger := zap.Must(logCfg.Build(zap.AddCaller(), zap.AddStacktrace(zap.ErrorLevel)))
	defer func() {
		_ = logger.Sync() // Flushes buffer, if any
	}()
	originalGlobalLogger := zap.L()
	zap.ReplaceGlobals(logger)
	defer zap.ReplaceGlobals(originalGlobalLogger) // Restore original global logger

	// Setup Database
	pool, err := pgxpool.New(appCtx, os.Getenv("DATABASE_URL"))
	if err != nil {
		return fmt.Errorf("failed to create pgxpool connection: %w", err)
	}
	defer pool.Close()
	db := database.NewDatabase(appCtx, pool)

	// Register Publisher
	if err := publisher.RegisterPublisher("postgres", db); err != nil {
		return fmt.Errorf("failed to register publisher 'postgres': %w", err)
	}

	// Instantiate Real Winet Service
	realWinetSvc := winet.New(cfg.WinetCfg)

	// Call the testable core run logic
	return run(appCtx, cfg, realWinetSvc, errorChan, logger, db)
}

// run contains the core application logic and is designed to be testable
// by accepting its dependencies as arguments, including the WinetService interface.
func run(ctx context.Context, cfg *config.Config, winetSvc WinetService, errorChan chan error, logger *zap.Logger, db *database.Database) error {
	eg, egCtx := errgroup.WithContext(ctx) // Use egCtx for goroutines managed by this errgroup

	// Cron job for DB cleanup
	eg.Go(func() error {
		// cronDbCleanup now returns error only on setup failure. Actual cron errors go to errorChan.
		if err := cronDbCleanup(db, errorChan, logger); err != nil {
			logger.Error("Failed to setup cronDbCleanup", zap.Error(err))
			return err // This error will stop the errgroup
		}
		return nil
	})

	// Cron job for processing Amber prices
	eg.Go(func() error {
		// processAmberPrices now returns error only on setup failure. Actual cron errors go to errorChan.
		if err := processAmberPrices(egCtx, db, errorChan, logger); err != nil {
			logger.Error("Failed to setup processAmberPrices", zap.Error(err))
			return err // This error will stop the errgroup
		}
		return nil
	})

	eg.Go(func() error {
		var err error
		for {
			// winetSvc is already initialized, no need to re-initialize
			if err = winetSvc.Connect(ctx); err != nil {
				// Log the error
				logger.Error("winetSvc.Connect failed", zap.Error(err))
				// Check if it's ErrConnect or any other error from Connect
				// If Connect fails, it's critical, return the error for errgroup
				return err // This goroutine should exit on any connection error
			}
			// This channel is specifically for timeout errors from winetSvc
			timeoutErr := <-winetSvc.SubscribeToTimeout()

			// Check if the received error is winet.ErrTimeout
			if errors.Is(timeoutErr, winet.ErrTimeout) {
				logger.Error("winet.ErrTimeout received, treating as critical, shutting down.", zap.Error(timeoutErr))
				return timeoutErr // Return the error to errgroup for shutdown
			}
			
			// Handle other potential non-timeout errors from SubscribeToTimeout if they can occur
			if timeoutErr != nil {
				logger.Error("Non-timeout error from winetSvc.SubscribeToTimeout, treating as critical, shutting down.", zap.Error(timeoutErr))
				// Depending on policy, other errors from this channel might also be critical
				return timeoutErr // Return the error to errgroup for shutdown
			}
			// If timeoutErr is nil (channel closed cleanly without error), the loop might continue or exit based on for loop behavior.
			// Assuming SubscribeToTimeout() would only send errors or block.
			// If it can close cleanly, this loop might need a way to terminate.
			// For now, assuming it primarily signals errors.
		}
	})

	})

	// HTTP Server Goroutine
	eg.Go(func() error {
		srv := &http.Server{
			Handler: api.HandlerWithOptions(server.New(winetSvc, db), api.GorillaServerOptions{
				Middlewares: []api.MiddlewareFunc{server.TimeoutMiddleware, server.LoggingMiddleware},
				ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
					logger.Error("HTTP handler error", zap.Error(err), zap.String("path", r.URL.Path))
					// Avoid writing header if it's already written (e.g. by middleware)
					if !isHeaderWritten(w) {
						http.Error(w, "Internal Server Error", http.StatusInternalServerError)
					}
				},
			}),
			Addr:         "0.0.0.0:8000",
			WriteTimeout: 15 * time.Second,
			ReadTimeout:  15 * time.Second,
		}
		logger.Info("Starting HTTP server on 0.0.0.0:8000")
		// ListenAndServe always returns a non-nil error.
		//nolint:contextcheck // egCtx is used by other goroutines; http server has its own shutdown via srv.Shutdown
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("HTTP server ListenAndServe error", zap.Error(err))
			return err // Critical error, stop the app
		}
		logger.Info("HTTP server stopped.")
		return nil
	})

	// Goroutine for handling non-critical errors from errorChan and context cancellation
	eg.Go(func() error {
		for {
			select {
			case err := <-errorChan: // Should only be non-critical errors (e.g., from cron jobs)
				if errors.Is(err, errCron) {
					logger.Error("Non-critical cron job error received", zap.Error(err))
				} else if err != nil {
					logger.Error("Other non-critical error received", zap.Error(err))
				}
				// Do not return err here, as these are non-critical. Loop continues.
			case <-egCtx.Done(): // Use errgroup's context here
				logger.Info("Context done signal received in error/cancellation processing goroutine.", zap.Error(egCtx.Err()))
				return egCtx.Err() // Propagate context cancellation
			}
		}
	})

	logger.Info("Application goroutines starting...")
	if err := eg.Wait(); err != nil {
		// Log the primary error that caused the shutdown, unless it's context.Canceled
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			logger.Info("Application shutdown initiated by context cancellation.", zap.Error(err))
		} else {
			logger.Error("Application shutdown due to critical error.", zap.Error(err))
		}
		return err // Return the error that caused the shutdown
	}

	logger.Info("Application shutdown gracefully.")
	return nil
}

// Helper function to check if HTTP headers have been written
func isHeaderWritten(w http.ResponseWriter) bool {
	// http.ResponseWriter doesn't explicitly expose this,
	// but common implementations have a way, or we assume if WriteHeader/Write is called.
	// For a robust check, a custom ResponseWriter wrapper would be needed.
	// Here, we'll rely on the fact that ErrorHandlerFunc is called before an automatic write.
	// This is a simplification.
	return false // Assume not written, to be safe for ErrorHandlerFunc to write.
}

var errCron = errors.New("cron error")

// cronDbCleanup now returns an error only if the cron job setup fails.
// Errors within the cron function itself are sent to errChan.
func cronDbCleanup(db *database.Database, errChan chan error, logger *zap.Logger) error {
	if err := db.Cleanup(context.Background()); err != nil {
		logger.Error("Critical error during initial db cleanup", zap.Error(err))
		// This initial cleanup failure is critical for startup.
		return fmt.Errorf("initial db cleanup failed: %w", err)
	}

	c := cron.New(cron.WithLogger(cron.PrintfLogger(logger.Named("cronDbCleanup").Sugar()))) // Use zap logger for cron
	_, err := c.AddFunc("CRON_TZ=Australia/Adelaide 0 3 * * *", func() {
		logger.Info("Running scheduled database cleanup...")
		if err := db.Cleanup(context.Background()); err != nil {
			logger.Error("Error during scheduled database cleanup", zap.Error(err))
			select {
			case errChan <- errors.Join(errCron, err):
			default:
				logger.Warn("errorChan full, dropping cron db cleanup error", zap.Error(err))
			}
		} else {
			logger.Info("Scheduled database cleanup completed successfully.")
		}
	})
	if err != nil {
		return fmt.Errorf("failed to add cron job for db cleanup: %w", err)
	}

	go c.Run() // Run the cron scheduler in a new goroutine
	logger.Info("Database cleanup cron job scheduled successfully.")
	return nil // Setup successful
}

// processAmberPrices now returns an error only if the cron job setup fails.
// Errors within the cron function itself are sent to errChan.
func processAmberPrices(ctx context.Context, db *database.Database, errChan chan error, logger *zap.Logger) error {
	svc, err := amber.New(ctx, os.Getenv("AMBER_HOST"), os.Getenv("AMBER_TOKEN"))
	if err != nil {
		logger.Error("error creating amber service", zap.Error(err))
		return err
	}
	// Ensure sites are available
	sites := svc.GetSites()
	if len(sites) == 0 {
		err := errors.New("no amber sites found")
		logger.Error(err.Error())
		return err
	}
	site := sites[0]

	prices, err := svc.GetPrices(ctx, site.Id)
	if err != nil {
		logger.Error("error getting initial amber prices", zap.Error(err))
		return err
	}

	if err := db.WriteAmberPrices(ctx, prices); err != nil {
		logger.Error("error writing initial amber prices", zap.Error(err))
		return err
	}
	logger.Info("initial amber prices written successfully")

	c := cron.New()
	if _, err := c.AddFunc("CRON_TZ=Australia/Adelaide */5 * * * *", func() {
		time.Sleep(5 * time.Second) // just to ensure we get the latest prices.
		prices, err = svc.GetPrices(ctx, site.Id)
		if err != nil {
			logger.Error("error getting amber prices via cron", zap.Error(err))
			select {
			case errChan <- errors.Join(errCron, err): // Join errors
			default:
				logger.Warn("errorChan full, dropping cron amber prices error", zap.Error(err))
			}
			return
		}
		if err = db.WriteAmberPrices(ctx, prices); err != nil {
			logger.Error("error writing amber prices via cron", zap.Error(err))
			select {
			case errChan <- errors.Join(errCron, err): // Join errors
			default:
				logger.Warn("errorChan full, dropping cron amber prices db write error", zap.Error(err))
			}
			return
		}
		logger.Info("amber prices updated via cron")
	}); err != nil {
		logger.Error("error adding cron job for amber prices", zap.Error(err))
		return err
	}

	c := cron.New(cron.WithLogger(cron.PrintfLogger(logger.Named("cronAmberPrices").Sugar()))) // Use zap logger
	_, err = c.AddFunc("CRON_TZ=Australia/Adelaide */5 * * * *", func() {
		// Use the context passed to processAmberPrices for operations within the cron job, if appropriate,
		// or context.Background() if the job's lifecycle is independent of the initial context.
		// For long-running services, context.Background() or a dedicated cron job context is often better.
		logger.Info("Running scheduled amber prices update...")
		time.Sleep(5 * time.Second) // just to ensure we get the latest prices.
		currentPrices, GetPricesErr := svc.GetPrices(context.Background(), site.Id) // Use Background for cron task
		if GetPricesErr != nil {
			logger.Error("Error getting amber prices via cron", zap.Error(GetPricesErr))
			select {
			case errChan <- errors.Join(errCron, GetPricesErr):
			default:
				logger.Warn("errorChan full, dropping cron amber prices error", zap.Error(GetPricesErr))
			}
			return
		}
		if writeErr := db.WriteAmberPrices(context.Background(), currentPrices); writeErr != nil { // Use Background for cron task
			logger.Error("Error writing amber prices via cron", zap.Error(writeErr))
			select {
			case errChan <- errors.Join(errCron, writeErr):
			default:
				logger.Warn("errorChan full, dropping cron amber prices db write error", zap.Error(writeErr))
			}
			return
		}
		logger.Info("Amber prices updated via cron successfully.")
	})
	if err != nil {
		return fmt.Errorf("failed to add cron job for amber prices: %w", err)
	}

	go c.Run() // Run the cron scheduler in a new goroutine
	logger.Info("Amber prices cron job scheduled successfully.")
	return nil // Setup successful
}
