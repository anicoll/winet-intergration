// Package cmd provides the main command implementation for the winet integration service.
package cmd

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"os/signal"
	"syscall"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	"github.com/anicoll/winet-integration/internal/pkg/amber"
	"github.com/anicoll/winet-integration/internal/pkg/config"
	dbpkg "github.com/anicoll/winet-integration/internal/pkg/database/db"
	"github.com/anicoll/winet-integration/internal/pkg/feedin"
	"github.com/anicoll/winet-integration/internal/pkg/publisher"
	"github.com/anicoll/winet-integration/internal/pkg/sqldb"
	"github.com/anicoll/winet-integration/internal/pkg/winet"
)

var errCron = errors.New("cron error")

type AmberUsageFetcher interface {
	GetUsage(ctx context.Context, siteID string, startDate, endDate openapi_types.Date) ([]dbpkg.Amberusage, error)
}

type AmberUsageWriter interface {
	WriteAmberUsage(ctx context.Context, usage []dbpkg.Amberusage) error
}

type AmberPricesWriter interface {
	WriteAmberPrices(ctx context.Context, prices []dbpkg.Amberprice) error
}

const (
	// Channel buffer sizes
	errorChannelBuffer = 1000

	// Cron schedules
	priceUpdateSchedule = "CRON_TZ=Australia/Adelaide */5 * * * *"
	usageUpdateSchedule = "CRON_TZ=Australia/Adelaide 0 8 * * *"

	// Delays
	priceUpdateDelay = 5 * time.Second

	// Reconnect backoff
	backoffBase     = 5 * time.Second
	backoffMax      = 5 * time.Minute
	maxConnAttempts = 10
)

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
		_ = logger.Sync()
	}()

	if cfg.DatabaseCfg.URL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	sqlWriter, err := sqldb.New(cfg.DatabaseCfg.URL)
	if err != nil {
		return fmt.Errorf("failed to open SQL database: %w", err)
	}
	defer func() { _ = sqlWriter.Close() }()

	pub := publisher.NewMultiPublisher(sqlWriter)

	errorChan := make(chan error, errorChannelBuffer)
	winetSvc := winet.New(&cfg.WinetCfg, pub, errorChan)

	loc, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		return fmt.Errorf("failed to load timezone: %w", err)
	}

	feedinCtrl := feedin.New(winetSvc, loc)
	winetSvc.SetDeviceStatusHook(feedinCtrl.UpdateFromStatuses)

	eg, ctx := errgroup.WithContext(ctx)

	// Start amber price processing service
	eg.Go(func() error {
		return startAmberPriceService(ctx, &cfg.AmberCfg, sqlWriter, errorChan, logger, feedinCtrl.Evaluate)
	})

	// Start amber usage processing service
	eg.Go(func() error {
		return startAmberUsageService(ctx, &cfg.AmberCfg, sqlWriter, errorChan, logger)
	})

	// Start winet service with retry logic
	eg.Go(func() error {
		return startWinetService(ctx, winetSvc, logger)
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

func startAmberPriceService(ctx context.Context, amberCfg *config.AmberConfig, db AmberPricesWriter, errChan chan error, logger *zap.Logger, onPriceUpdate func([]dbpkg.Amberprice)) error {
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
	if _, err := fetchAndStorePrices(ctx, svc, db, site.Id); err != nil {
		return fmt.Errorf("initial price fetch failed: %w", err)
	}

	// Setup cron job
	c := cron.New()
	if _, err := c.AddFunc(priceUpdateSchedule, func() {
		time.Sleep(priceUpdateDelay) // ensure we get the latest prices
		prices, err := fetchAndStorePrices(context.Background(), svc, db, site.Id)
		if err != nil {
			logger.Error("amber price update failed", zap.Error(err))
			select {
			case errChan <- fmt.Errorf("%w: %v", errCron, err):
			default:
				logger.Warn("error channel full, dropping error")
			}
			return
		}
		logger.Info("amber prices updated")
		if onPriceUpdate != nil {
			onPriceUpdate(prices)
		}
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

func startAmberUsageService(ctx context.Context, amberCfg *config.AmberConfig, db AmberUsageWriter, errChan chan error, logger *zap.Logger) error {
	logger.Info("Starting amber usage service")

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

	// Initial usage fetch
	if err := fetchAndStoreUsage(ctx, svc, db, site.Id); err != nil {
		logger.Warn("initial usage fetch failed", zap.Error(err))
	}

	c := cron.New()
	if _, err := c.AddFunc(usageUpdateSchedule, func() {
		if err := fetchAndStoreUsage(context.Background(), svc, db, site.Id); err != nil {
			logger.Error("amber usage update failed", zap.Error(err))
			select {
			case errChan <- fmt.Errorf("%w: %v", errCron, err):
			default:
				logger.Warn("error channel full, dropping error")
			}
			return
		}
		logger.Info("amber usage updated")
	}); err != nil {
		return fmt.Errorf("failed to schedule amber usage updates: %w", err)
	}

	c.Start()

	<-ctx.Done()
	c.Stop()
	logger.Info("Amber usage service stopped")
	return ctx.Err()
}

func fetchAndStoreUsage(ctx context.Context, svc AmberUsageFetcher, db AmberUsageWriter, siteId string) error {
	now := time.Now()
	startDate := openapi_types.Date{Time: now.AddDate(0, 0, -7)}
	endDate := openapi_types.Date{Time: now.AddDate(0, 0, -1)}

	usage, err := svc.GetUsage(ctx, siteId, startDate, endDate)
	if err != nil {
		return fmt.Errorf("failed to get usage: %w", err)
	}

	if err := db.WriteAmberUsage(ctx, usage); err != nil {
		return fmt.Errorf("failed to write usage to database: %w", err)
	}

	return nil
}

func fetchAndStorePrices(ctx context.Context, svc interface {
	GetPrices(ctx context.Context, siteID string) ([]dbpkg.Amberprice, error)
}, db AmberPricesWriter, siteId string,
) ([]dbpkg.Amberprice, error) {
	prices, err := svc.GetPrices(ctx, siteId)
	if err != nil {
		return nil, fmt.Errorf("failed to get prices: %w", err)
	}

	if err := db.WriteAmberPrices(ctx, prices); err != nil {
		return nil, fmt.Errorf("failed to write prices to database: %w", err)
	}

	return prices, nil
}

type winetSvc interface {
	Connect(ctx context.Context) error
	Events() <-chan winet.SessionEvent
}

func startWinetService(ctx context.Context, winetSvc winetSvc, logger *zap.Logger) error {
	logger.Info("Starting winet service")
	consecutiveFails := 0

	for {
		select {
		case <-ctx.Done():
			logger.Info("Winet service stopped")
			return ctx.Err()
		default:
		}

		if err := winetSvc.Connect(ctx); err != nil {
			consecutiveFails++
			backoff := reconnectBackoff(consecutiveFails - 1)
			logger.Error("winet connection failed",
				zap.Error(err),
				zap.Int("attempt", consecutiveFails),
				zap.Duration("backoff", backoff),
			)
			if consecutiveFails >= maxConnAttempts {
				return fmt.Errorf("winet: exceeded %d consecutive connection failures: %w", maxConnAttempts, err)
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
			continue
		}

		consecutiveFails = 0
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
			logger.Info("Winet service stopped")
			return ctx.Err()
		}
	}
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
