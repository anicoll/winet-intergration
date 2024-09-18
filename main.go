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
			&cli.StringFlag{
				Name:     "winet-hostport",
				EnvVars:  []string{"WINET_HOSTPORT"},
				Required: true,
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}

}
