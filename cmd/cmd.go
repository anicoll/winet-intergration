package cmd

import (
	"context"
	"net/http"
	"os"
	"time"

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
		return cronDbCleanup(db)
	})

	eg.Go(func() error {
		return winetSvc.Connect(ctx)
	})

	eg.Go(func() error {
		srv := &http.Server{
			Handler: api.HandlerWithOptions(server.New(winetSvc, db), api.GorillaServerOptions{
				Middlewares: []api.MiddlewareFunc{server.LoggingMiddleware},
			}),
			Addr:         "127.0.0.1:8000",
			WriteTimeout: 15 * time.Second,
			ReadTimeout:  15 * time.Second,
		}

		return srv.ListenAndServe()
	})

	eg.Go(func() error {
		// handle any async errors from service
		select {
		case err := <-errorChan:
			logger.Error(err.Error())
		case <-ctx.Done():
			logger.Info("context done")
			return nil
		}
		return nil
	})

	return eg.Wait()
}

func cronDbCleanup(db *database.Database) error {
	if err := db.Cleanup(context.Background()); err != nil {
		return err
	}

	// CRON automation
	c := cron.New()
	if _, err := c.AddFunc("CRON_TZ=Australia/Adelaide 0 3 * * *", func() {
		if err := db.Cleanup(context.Background()); err != nil {
			zap.L().Error("error cleaning up database", zap.Error(err))
			return
		}
		zap.L().Info("automated discharge of battery")
	}); err != nil {
		return err
	}

	c.Run()
	return nil
}
