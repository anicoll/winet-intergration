package cmd

import (
	"context"

	"github.com/anicoll/winet-integration/internal/pkg/config"
	"github.com/anicoll/winet-integration/internal/pkg/winet"
	"github.com/urfave/cli/v2"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

func WinetCommand(ctx *cli.Context) error {
	cfg := &config.Config{
		WinetCfg: &config.WinetConfig{
			Password: ctx.String("winet-password"),
			Username: ctx.String("winet-username"),
			HostPort: ctx.String("winet-hostport"),
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
	defer logger.Sync() // flushes buffer, if any
	zap.ReplaceGlobals(logger)

	winetSvc := winet.New(cfg.WinetCfg, errorChan)

	eg.Go(func() error {
		return winetSvc.Connect(ctx)
	})

	eg.Go(func() error {
		// handle any async errors from service
		return <-errorChan
	})

	return eg.Wait()
}
