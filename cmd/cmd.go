package cmd

import (
	"context"
	"net/http"
	"time"

	"github.com/anicoll/winet-integration/internal/pkg/config"
	"github.com/anicoll/winet-integration/internal/pkg/mqtt"
	"github.com/anicoll/winet-integration/internal/pkg/server"
	"github.com/anicoll/winet-integration/internal/pkg/winet"
	api "github.com/anicoll/winet-integration/pkg/server"
	paho_mqtt "github.com/eclipse/paho.mqtt.golang"
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
			Host: "localhost:1883",
		},
		LogLevel: ctx.String("log-level"),
	}

	// ac, err := amber.New(ctx.Context, "https://api.amber.com.au/v1", "psk_f74f7d2495dcfc98186a5c16ffee655b")
	// _ = err
	// sites := ac.GetSites()
	// _ = ac.GetPrices(context.Background(), sites[0].Id)
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
	mqttOpts := paho_mqtt.NewClientOptions()
	mqttOpts.SetPassword(cfg.MqttCfg.Password)
	mqttOpts.SetUsername(cfg.MqttCfg.Username)
	mqttOpts.AddBroker(cfg.MqttCfg.Host)
	pahoClient := paho_mqtt.NewClient(mqttOpts)

	mqttSvc := mqtt.New(pahoClient)
	if err := mqttSvc.Connect(); err != nil {
		return err
	}

	winetSvc := winet.New(cfg.WinetCfg, mqttSvc, errorChan)

	eg.Go(func() error {
		return winetSvc.Connect(ctx)
	})

	eg.Go(func() error {
		// CRON automation
		c := cron.New()
		c.AddFunc("CRON_TZ=Australia/Adelaide 1 17 * * *", func() {
			time.Sleep(time.Second)
			// enable feedin
			if _, err := winetSvc.SetFeedInLimitation(false); err != nil {
				logger.Error(err.Error())
			}
			// discharge batter at 1.6Kw/h
			if _, err := winetSvc.SendDischargeCommand("1.6"); err != nil {
				logger.Error(err.Error())
			}
			logger.Info("automated discharge of battery")
		})

		c.AddFunc("CRON_TZ=Australia/Adelaide 1 21 * * *", func() {
			time.Sleep(time.Second)
			// enable feedin
			if _, err := winetSvc.SetFeedInLimitation(true); err != nil {
				logger.Error(err.Error())
			}
			// stop discharge
			if _, err := winetSvc.SendSelfConsumptionCommand(); err != nil {
				logger.Error(err.Error())
			}
			logger.Info("automated consumption of battery and disable feedin")
		})
		c.Run()
		return nil
	})

	eg.Go(func() error {
		srv := &http.Server{
			Handler: api.HandlerWithOptions(server.New(winetSvc), api.GorillaServerOptions{
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
