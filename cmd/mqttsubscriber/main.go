package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	paho_mqtt "github.com/eclipse/paho.mqtt.golang"

	"github.com/anicoll/winet-integration/internal/pkg/mqttsubscriber"
)

func main() {
	host := os.Getenv("MQTT_HOST")
	username := os.Getenv("MQTT_USERNAME")
	password := os.Getenv("MQTT_PASSWORD")

	if host == "" {
		fmt.Fprintln(os.Stderr, "MQTT_HOST is required")
		os.Exit(1)
	}

	opts := paho_mqtt.NewClientOptions()
	opts.AddBroker(host)
	opts.SetUsername(username)
	opts.SetPassword(password)
	opts.SetClientID("winet-mqttsubscriber-test")

	client := paho_mqtt.NewClient(opts)
	if token := client.Connect(); !token.WaitTimeout(5e9) || token.Error() != nil {
		fmt.Fprintf(os.Stderr, "failed to connect: %v\n", token.Error())
		os.Exit(1)
	}
	fmt.Printf("connected to %s\n", host)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	sub := mqttsubscriber.New(client)
	ch, err := sub.Subscribe(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to subscribe: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("listening for sensor updates (ctrl+c to stop)...")
	for dp := range ch {
		fmt.Printf("[%s] %s / %s = %s %s\n",
			dp.ReceivedAt.Format("15:04:05"),
			dp.Identifier,
			dp.Slug,
			dp.Value,
			dp.UnitOfMeasurement,
		)
	}
}
