package main

import (
	"context"
	"log"
	_ "time/tzdata"

	"github.com/anicoll/winet-integration/cmd"
	"github.com/anicoll/winet-integration/internal/pkg/config"
)

//go:generate go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.6.0 --config=./gen/config.yaml ./gen/api.yaml
//go:generate go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.6.0 --config=./gen/amber/config.yaml ./gen/amber/api.json

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	if err := cmd.Run(context.Background(), cfg); err != nil {
		log.Fatal(err)
	}
}
