package mqtt

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/anicoll/winet-integration/internal/pkg/model"
	"github.com/anicoll/winet-integration/internal/pkg/publisher"
)

var configuredDevices map[string]struct{}

func (s *service) Write(ctx context.Context, data []publisher.DataPoint) error {
	for _, d := range data {
		if err := s.publishDataPoint(d); err != nil {
			return err
		}
	}
	return nil
}

func (s *service) RegisterDevice(_ context.Context, device *model.Device) error {
	if _, exists := configuredDevices[device.ID]; exists {
		return nil
	}
	registerMessage := defaultRegisterMsg(device)
	slugIdentifier := fmt.Sprintf("%s_%s", device.Model, device.SerialNumber)

	topic := fmt.Sprintf("homeassistant/sensor/%s/config", slugIdentifier)

	payload, err := json.Marshal(registerMessage)
	if err != nil {
		return err
	}
	token := s.client.Publish(topic, 1, true, payload)
	if err := token.Error(); err != nil {
		return err
	}
	if res := token.WaitTimeout(time.Second * 5); res {
		configuredDevices[device.ID] = struct{}{}
		return nil
	}
	return nil
}

func (s *service) publishDataPoint(data publisher.DataPoint) error {
	isTextSensor := model.TextSensors.HasSlug(data.Slug)
	topic := fmt.Sprintf("homeassistant/sensor/%s/%s/state", data.Identifier, data.Slug)

	payload := map[string]string{
		"value": data.Value,
	}
	if !isTextSensor {
		payload["unit_of_measurement"] = data.UnitOfMeasurement
	}

	publishData, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	token := s.client.Publish(topic, 0, false, publishData)
	res := token.WaitTimeout(time.Second * 10)
	if res {
		return nil
	}
	if err := token.Error(); err != nil {
		return err
	}
	return nil
}

func defaultRegisterMsg(device *model.Device) model.RegisterMessage {
	name := fmt.Sprintf("%s %s", device.Model, device.SerialNumber)
	slugIdentifier := fmt.Sprintf("%s_%s", device.Model, device.SerialNumber)

	return model.RegisterMessage{
		Tilda:      fmt.Sprintf("homeassistant/sensor/%s", slugIdentifier),
		Name:       name,
		ID:         strings.ToLower(slugIdentifier),
		StateTopic: "~/state",
		Device: model.RegisterDevice{
			Name:         name,
			Identifiers:  []string{slugIdentifier},
			Model:        device.Model,
			Manufacturer: "Sungrow",
		},
	}
}
