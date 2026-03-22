package mqttsubscriber

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	paho_mqtt "github.com/eclipse/paho.mqtt.golang"
	"go.uber.org/zap"
)

// DataPoint represents a sensor reading received from an MQTT state topic.
type DataPoint struct {
	// Identifier is the device identifier extracted from the topic (e.g. "SH10RT_B2xxxxxxx").
	Identifier string
	// Slug is the sensor name extracted from the topic (e.g. "daily_pv_generation").
	Slug string
	// Value is the sensor reading as a string.
	Value string
	// UnitOfMeasurement is empty for text sensors.
	UnitOfMeasurement string
	// ReceivedAt is when the message was received.
	ReceivedAt time.Time
}

type service struct {
	client paho_mqtt.Client
	logger *zap.Logger
	dataCh chan DataPoint
}

// stateTopic subscribes to all sensor state messages published by the winet integration.
// Topic format: homeassistant/sensor/{identifier}/{slug}/state
const stateTopic = "homeassistant/sensor/+/+/state"

// New creates a new MQTT subscriber using the provided client.
// The client must already be connected.
func New(client paho_mqtt.Client) *service {
	return &service{
		client: client,
		logger: zap.L(),
		dataCh: make(chan DataPoint, 100),
	}
}

// Subscribe registers a subscription for all sensor state topics and returns a
// channel that receives incoming DataPoints. The channel is closed when ctx is
// cancelled or the caller calls Unsubscribe.
func (s *service) Subscribe(ctx context.Context) (<-chan DataPoint, error) {
	token := s.client.Subscribe(stateTopic, 0, s.handleMessage)
	if token.WaitTimeout(5 * time.Second) {
		if err := token.Error(); err != nil {
			return nil, err
		}
	}

	go func() {
		<-ctx.Done()
		s.client.Unsubscribe(stateTopic)
		close(s.dataCh)
	}()

	return s.dataCh, nil
}

type statePayload struct {
	Value             string `json:"value"`
	UnitOfMeasurement string `json:"unit_of_measurement"`
}

// handleMessage is the paho callback invoked for each incoming state message.
func (s *service) handleMessage(_ paho_mqtt.Client, msg paho_mqtt.Message) {
	// Topic: homeassistant/sensor/{identifier}/{slug}/state
	parts := strings.Split(msg.Topic(), "/")
	if len(parts) != 5 {
		s.logger.Warn("unexpected topic format", zap.String("topic", msg.Topic()))
		return
	}

	identifier := parts[2]
	slug := parts[3]

	var payload statePayload
	if err := json.Unmarshal(msg.Payload(), &payload); err != nil {
		s.logger.Error("failed to unmarshal state payload",
			zap.Error(err),
			zap.String("topic", msg.Topic()),
		)
		return
	}

	dp := DataPoint{
		Identifier:        identifier,
		Slug:              slug,
		Value:             payload.Value,
		UnitOfMeasurement: payload.UnitOfMeasurement,
		ReceivedAt:        time.Now(),
	}

	select {
	case s.dataCh <- dp:
	default:
		s.logger.Warn("subscriber channel full, dropping data point",
			zap.String("identifier", identifier),
			zap.String("slug", slug),
		)
	}
}
