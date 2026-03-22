// Package cmd provides the main command implementation for the winet integration service.
package cmd

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	paho_mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/golang-migrate/migrate/v4"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	"github.com/anicoll/winet-integration/internal/pkg/amber"
	"github.com/anicoll/winet-integration/internal/pkg/auth"
	"github.com/anicoll/winet-integration/internal/pkg/config"
	"github.com/anicoll/winet-integration/internal/pkg/database"
	dbpkg "github.com/anicoll/winet-integration/internal/pkg/database/db"
	"github.com/anicoll/winet-integration/internal/pkg/database/migration"
	"github.com/anicoll/winet-integration/internal/pkg/logic"
	"github.com/anicoll/winet-integration/internal/pkg/mqtt"
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

	// Reconnect backoff
	backoffBase     = 5 * time.Second
	backoffMax      = 5 * time.Minute
	maxConnAttempts = 10
)

// healthState holds the current winet connection status, safe for concurrent access.
type healthState struct {
	status atomic.Value // stores string
}

func (h *healthState) set(s string) { h.status.Store(s) }
func (h *healthState) get() string {
	if v := h.status.Load(); v != nil {
		return v.(string)
	}
	return "starting"
}

// reconnectBackoff returns the wait duration for the given attempt (0-indexed)
// using exponential backoff (base×2^attempt, capped at backoffMax) plus ±20% jitter.
func reconnectBackoff(attempt int) time.Duration {
	d := min(backoffBase*(1<<min(attempt, 6)), backoffMax)
	jitter := time.Duration(rand.Int64N(int64(d/5)*2)) - d/5
	return d + jitter
}

// Run orchestrates all services and handles graceful shutdown.
func Run(ctx context.Context, cfg *config.Config) error {
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

	defer cleanup()

	mqttOpts := paho_mqtt.NewClientOptions()
	mqttOpts.SetPassword(cfg.MqttCfg.Password)
	mqttOpts.SetUsername(cfg.MqttCfg.Username)
	mqttOpts.AddBroker(cfg.MqttCfg.Host)

	mqttPublisher := mqtt.New(paho_mqtt.NewClient(mqttOpts))
	if err := mqttPublisher.Connect(); err != nil {
		return fmt.Errorf("failed to connect to MQTT broker: %w", err)
	}

	pub := publisher.NewMultiPublisher(db, mqttPublisher)

	authSvc := auth.NewService(cfg.AuthCfg.JWTSecret, cfg.AuthCfg.AccessTokenTTL, cfg.AuthCfg.RefreshTokenTTL, db, db)
	authSvc.StartCleanup(ctx, time.Hour)

	errorChan := make(chan error, errorChannelBuffer)
	winetSvc := winet.New(&cfg.WinetCfg, pub, errorChan)

	health := &healthState{}
	health.set("starting")

	eg, ctx := errgroup.WithContext(ctx)

	// // Start database cleanup service
	// eg.Go(func() error {
	// 	return startDbCleanupService(ctx, db, errorChan, logger)
	// })

	// Start amber price processing service
	eg.Go(func() error {
		return startAmberPriceService(ctx, &cfg.AmberCfg, db, errorChan, logger)
	})

	// Start winet service with retry logic
	eg.Go(func() error {
		return startWinetService(ctx, winetSvc, health, logger)
	})
	// Start decision logic service.
	// eg.Go(func() error {
	// 	return startDecisionService(ctx, winetSvc, db, errorChan, logger)
	// })

	// enable feedin at 5PM daily
	eg.Go(func() error {
		return dailyFeedinEnabler(ctx, cfg.Timezone, winetSvc, logger)
	})

	// Start HTTP server
	eg.Go(func() error {
		return startHTTPServer(ctx, winetSvc, db, authSvc, health, cfg.AllowedOrigin, logger)
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

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	return database.NewDatabase(pool), pool.Close, nil
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

func startAmberPriceService(ctx context.Context, amberCfg *config.AmberConfig, db *database.Database, errChan chan error, logger *zap.Logger) error {
	logger.Info("Starting amber price service")

	if amberCfg == nil || amberCfg.Host == "" || amberCfg.Token == "" {
		return errors.New("amber host and token are required")
	}

	svc, err := amber.New(ctx, amberCfg.Host, amberCfg.Token)
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
	GetPrices(ctx context.Context, siteID string) ([]dbpkg.Amberprice, error)
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
	Events() <-chan winet.SessionEvent
}

func startWinetService(ctx context.Context, winetSvc winetSvc, health *healthState, logger *zap.Logger) error {
	logger.Info("Starting winet service")
	consecutiveFails := 0

	for {
		select {
		case <-ctx.Done():
			health.set("disconnected")
			logger.Info("Winet service stopped")
			return ctx.Err()
		default:
		}

		health.set("reconnecting")
		if err := winetSvc.Connect(ctx); err != nil {
			consecutiveFails++
			backoff := reconnectBackoff(consecutiveFails - 1)
			logger.Error("winet connection failed",
				zap.Error(err),
				zap.Int("attempt", consecutiveFails),
				zap.Duration("backoff", backoff),
			)
			if consecutiveFails >= maxConnAttempts {
				health.set("disconnected")
				return fmt.Errorf("winet: exceeded %d consecutive connection failures: %w", maxConnAttempts, err)
			}
			select {
			case <-ctx.Done():
				health.set("disconnected")
				return ctx.Err()
			case <-time.After(backoff):
			}
			continue
		}

		consecutiveFails = 0
		health.set("connected")
		logger.Info("Winet service connected successfully")

		// Wait for a session event or context cancellation
		select {
		case event := <-winetSvc.Events():
			if errors.Is(event.Err, winet.ErrTimeout) {
				logger.Warn("winet timeout occurred, reconnecting", zap.Error(event.Err))
				continue
			}
			logger.Error("winet service error", zap.Error(event.Err))
			return event.Err
		case <-ctx.Done():
			health.set("disconnected")
			logger.Info("Winet service stopped")
			return ctx.Err()
		}
	}
}

func dailyFeedinEnabler(ctx context.Context, timezone string, winetSvc server.WinetService, logger *zap.Logger) error {
	if timezone == "" {
		timezone = "Australia/Adelaide"
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return err
	}
	c := cron.New(cron.WithLocation(location))
	if _, err := c.AddFunc("0 17 * * *", func() {
		logger.Info("enabling feedin")
		success, errr := winetSvc.SetFeedInLimitation(false)
		if !success {
			err = errr
		}
	}); err != nil {
		return err
	}
	c.Start()
	<-ctx.Done()
	c.Stop()
	return ctx.Err()
}

func startDecisionService(ctx context.Context, winetSvc server.WinetService, db *database.Database, errChan chan error, logger *zap.Logger) error {
	logger.Info("Starting logic service")

	svc := logic.NewLogicSvc(winetSvc, db)

	for {
		select {
		case <-ctx.Done():
			logger.Info("logic service stopped")
			return nil
		default:
			time.Sleep(5 * time.Second) // Delay to avoid tight loop

			if err := svc.NextBestAction(ctx); err != nil {
				logger.Error("logic service error", zap.Error(err))
				select {
				case errChan <- fmt.Errorf("%w: %v", errCron, err):
				default:
					logger.Warn("error channel full, dropping error")
				}
				return err
			}
			logger.Info("Next best action executed successfully")
		}
	}
}

func startHTTPServer(ctx context.Context, winetSvc server.WinetService, db *database.Database, authSvc *auth.Service, health *healthState, allowedOrigin string, logger *zap.Logger) error {
	logger.Info("Starting HTTP server", zap.String("addr", serverAddr))

	apiHandler := api.HandlerWithOptions(server.New(winetSvc, db, authSvc), api.StdHTTPServerOptions{
		Middlewares: []api.MiddlewareFunc{server.TimeoutMiddleware, server.LoggingMiddleware(allowedOrigin), server.AuthMiddleware(authSvc)},
		ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			logger.Error("HTTP handler error", zap.Error(err))
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		},
	})

	mux := http.NewServeMux()
	mux.Handle("/", apiHandler)
	mux.HandleFunc("OPTIONS /", func(w http.ResponseWriter, r *http.Request) {
		if allowedOrigin != "" {
			w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":%q}`, health.get())
	})

	srv := &http.Server{
		Handler:      mux,
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
