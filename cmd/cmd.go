package cmd

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/anicoll/winet-integration/internal/pkg/amber"
	"github.com/anicoll/winet-integration/internal/pkg/config"
	"github.com/anicoll/winet-integration/internal/pkg/database"
	"github.com/anicoll/winet-integration/internal/pkg/publisher"
	"github.com/anicoll/winet-integration/internal/pkg/server"
	"github.com/anicoll/winet-integration/internal/pkg/winet"
	api "github.com/anicoll/winet-integration/pkg/server"
	"github.com/jackc/pgx/v5"
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

	return run(ctx.Context, cfg)
}

func run(ctx context.Context, cfg *config.Config) error {
	errorChan := make(chan error, 1000)
	var err error

	eg, ctx := errgroup.WithContext(ctx)
	logCfg := zap.NewProductionConfig()

	logCfg.Level, err = zap.ParseAtomicLevel(cfg.LogLevel)
	if err != nil {
		return err
	}
	logCfg.OutputPaths = []string{"stdout"}
	logCfg.ErrorOutputPaths = []string{"stdout"}
	logCfg.Sampling = nil
	logger := zap.Must(logCfg.Build(zap.AddCaller(), zap.AddStacktrace(zap.ErrorLevel)))
	defer func() {
		_ = logger.Sync() // flushes buffer, if any.
	}()
	zap.ReplaceGlobals(logger)

	conn, err := pgx.Connect(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		return err
	}
	defer conn.Close(ctx)
	db := database.NewDatabase(ctx, conn)

	if err := publisher.RegisterPublisher("postgres", db); err != nil {
		return err
	}

	winetSvc := winet.New(cfg.WinetCfg, errorChan)

	eg.Go(func() error {
		return cronDbCleanup(db, errorChan)
	})

	eg.Go(func() error {
		return processAmberPrices(ctx, db, errorChan)
	})

	eg.Go(func() error {
		for {
			winetSvc = winet.New(cfg.WinetCfg, errorChan)
			if err := winetSvc.Connect(ctx); err != nil {
				return err
			}
			if err := <-winetSvc.SubscribeToTimeout(); errors.Is(err, winet.ErrTimeout) {
				logger.Error("timeout error", zap.Error(err))
				continue
			}
		}
	})

	eg.Go(func() error {
		srv := &http.Server{
			Handler: api.HandlerWithOptions(server.New(winetSvc, db), api.GorillaServerOptions{
				Middlewares: []api.MiddlewareFunc{server.LoggingMiddleware},
			}),
			Addr:         "0.0.0.0:8000",
			WriteTimeout: 15 * time.Second,
			ReadTimeout:  15 * time.Second,
		}

		return srv.ListenAndServe()
	})

	eg.Go(func() error {
		// handle any async errors from service
		for {
			select {
			case err := <-errorChan:
				if errors.Is(err, errCron) {
					logger.Error("cron error", zap.Error(err))
					return err
				}
				if strings.Contains(err.Error(), "failed to deallocate") {
					return err
				}
			case <-ctx.Done():
				logger.Info("context done")
				return ctx.Err()
			}
		}
	})

	return eg.Wait()
}

var errCron = errors.New("cron error")

func cronDbCleanup(db *database.Database, errChan chan error) error {
	if err := db.Cleanup(context.Background()); err != nil {
		return err
	}

	// CRON automation
	c := cron.New()
	if _, err := c.AddFunc("CRON_TZ=Australia/Adelaide 0 3 * * *", func() {
		if err := db.Cleanup(context.Background()); err != nil {
			zap.L().Error("error cleaning up database", zap.Error(err))
			errChan <- errCron
			return
		}
		zap.L().Info("automated discharge of battery")
	}); err != nil {
		return err
	}

	c.Run()
	return nil
}

func processAmberPrices(ctx context.Context, db *database.Database, errChan chan error) error {
	svc, err := amber.New(ctx, os.Getenv("AMBER_HOST"), os.Getenv("AMBER_TOKEN"))
	if err != nil {
		return err
	}
	site := svc.GetSites()[0]

	prices, err := svc.GetPrices(ctx, site.Id)
	if err != nil {
		return err
	}

	if err := db.WriteAmberPrices(ctx, prices); err != nil {
		return err
	}

	c := cron.New()
	if _, err := c.AddFunc("CRON_TZ=Australia/Adelaide */5 * * * *", func() {
		time.Sleep(5 * time.Second) // just to ensure we get the latest prices.
		prices, err = svc.GetPrices(ctx, site.Id)
		if err != nil {
			errChan <- errCron
			return
		}
		if err = db.WriteAmberPrices(ctx, prices); err != nil {
			errChan <- errCron
			return
		}
		zap.L().Info("automated discharge of battery")
	}); err != nil {
		return err
	}

	c.Run()
	return err
}
