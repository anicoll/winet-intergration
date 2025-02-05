package cmd

import (
	"context"
	"net/http"
	"time"

	"github.com/anicoll/winet-integration/internal/pkg/config"
	"github.com/anicoll/winet-integration/internal/pkg/handler"
	"github.com/anicoll/winet-integration/internal/pkg/mqtt"
	"github.com/anicoll/winet-integration/internal/pkg/winet"
	paho_mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/gorilla/mux"
	"github.com/urfave/cli/v2"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

func WinetCommand(ctx *cli.Context) error {
	cfg := &config.Config{
		WinetCfg: &config.WinetConfig{
			Password: ctx.String("winet-password"),
			Username: ctx.String("winet-username"),
			Host:     ctx.String("winet-host"),
			Ssl:      ctx.Bool("winet-ssl"),
		},
		MqttCfg: &config.WinetConfig{
			Host: "localhost:1883",
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
		r := mux.NewRouter()
		r.HandleFunc("/battery", handler.Battery(winetSvc))
		r.HandleFunc("/inverter", handler.Inverter(winetSvc))
		r.Use(handler.LoggingMiddleware)

		srv := &http.Server{
			Handler:      r,
			Addr:         "127.0.0.1:8000",
			WriteTimeout: 15 * time.Second,
			ReadTimeout:  15 * time.Second,
		}

		return srv.ListenAndServe()
	})

	eg.Go(func() error {
		// handle any async errors from service
		for {
			err := <-errorChan
			logger.Error(err.Error())
		}
	})

	return eg.Wait()
}
