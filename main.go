package main

import (
	"log"
	"os"

	"github.com/anicoll/winet-integration/cmd"
	"github.com/urfave/cli/v2"
)

func main() {
	app := &cli.App{
		Name:   "winet-controller",
		Usage:  "controller for winet-s device",
		Action: cmd.WinetCommand,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "winet-password",
				EnvVars: []string{"WINET_PASSWORD"},
				Value:   "pw8888",
			},
			&cli.StringFlag{
				Name:    "winet-username",
				EnvVars: []string{"WINET_USERNAME"},
				Value:   "admin",
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
