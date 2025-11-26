package main

import (
	"fmt"
	"log"
	"os"

	"github.com/goburrow/modbus"
	"github.com/urfave/cli/v2"

	"github.com/anicoll/winet-integration/cmd"
)

//go:generate go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen --config=./gen/config.yaml ./gen/api.yaml
//go:generate go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen --config=./gen/amber/config.yaml ./gen/amber/api.json

func main() {
	app := &cli.App{
		Name:   "winet-controller",
		Usage:  "controller for winet-s device",
		Action: cmd.WinetCommand,
		// Action: md,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "winet-password",
				EnvVars: []string{"WINET_PASSWORD"},
				Value:   "",
			},
			&cli.StringFlag{
				Name:    "mqtt-host",
				EnvVars: []string{"MQTT_HOST"},
				Value:   "",
			},
			&cli.StringFlag{
				Name:    "mqtt-pass",
				EnvVars: []string{"MQTT_PASS"},
				Value:   "",
			},
			&cli.StringFlag{
				Name:    "mqtt-user",
				EnvVars: []string{"MQTT_USER"},
				Value:   "",
			},
			&cli.StringFlag{
				Name:    "winet-username",
				EnvVars: []string{"WINET_USERNAME"},
				Value:   "",
			},
			&cli.StringFlag{
				Name:     "database-url",
				EnvVars:  []string{"DATABASE_URL"},
				Value:    "",
				Required: true,
			},
			&cli.StringFlag{
				Name:     "migrations-folder",
				EnvVars:  []string{"MIGRATIONS_FOLDER"},
				Value:    "",
				Required: true,
			},
			&cli.BoolFlag{
				Name:    "winet-ssl",
				EnvVars: []string{"WINET_SSL"},
				Value:   false,
			},
			&cli.DurationFlag{
				Name:    "poll-interval",
				EnvVars: []string{"POLL_INTERVAL"},
				Value:   10,
			},
			&cli.StringFlag{
				Name:     "winet-host",
				EnvVars:  []string{"WINET_HOST"},
				Required: true,
			},
			&cli.StringFlag{
				Name:    "log-level",
				EnvVars: []string{"LOG_LEVEL"},
				Value:   "INFO",
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

// WinetCommand is the main entry point for the winet integration CLI command.
// It validates configuration and starts all required services.
func md(ctx *cli.Context) error {
	// Modbus TCP
	// modbus.RTUClient()
	// client := modbus.TCPClient("192.168.107.8:502")
	handler := modbus.NewTCPClientHandler("192.168.107.8:502")
	fmt.Println(handler.Connect())

	// handler.SlaveId = byte('1')
	client := modbus.NewClient(handler)
	// Read input register 9
	results, err := client.ReadHoldingRegisters(4990, 10)
	if err != nil {
		return err
	}

	// results, err = client.ReadDiscreteInputs(4990, 10)
	// results, err = client.ReadDiscreteInputs(4989, 10)
	// if err != nil {
	// 	return err
	// }
	for _, res := range results {
		fmt.Println(res)
	}
	return nil
}
