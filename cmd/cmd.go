package cmd

import (
	"context"
	"time"

	"github.com/anicoll/winet-integration/internal/pkg/config"
	"github.com/anicoll/winet-integration/internal/pkg/winet"
	"github.com/urfave/cli/v2"
	"golang.org/x/sync/errgroup"
)

func WinetCommand(ctx *cli.Context) error {
	cfg := &config.Config{
		WinetCfg: &config.WinetConfig{
			Password: ctx.String("winet-password"),
			Username: ctx.String("winet-username"),
			HostPort: ctx.String("winet-hostport"),
		},
	}

	return run(ctx.Context, cfg)
}

func run(ctx context.Context, cfg *config.Config) error {
	errorChan := make(chan error)
	winetSvc := winet.New(cfg.WinetCfg, errorChan)
	eg, ctx := errgroup.WithContext(ctx)

	eg.Go(func() error {
		return winetSvc.Connect(ctx)
	})
	eg.Go(func() error {
		time.Sleep(time.Hour)
		return nil
	})
	return eg.Wait()
}
