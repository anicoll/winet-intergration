package mqtt

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/anicoll/winet-integration/internal/pkg/model"
)

var configuredDevices map[string]struct{}

func (s *service) Write(ctx context.Context, data []map[string]any) error {
	for _, d := range data {
		if err := s.PublishData(d); err != nil {
			return err
		}
	}
	return nil
}

func (s *service) RegisterDevice(device *model.Device) error {
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

func (s *service) PublishData(data map[string]any) error {
	isTextSensor := model.TextSensors.HasSlug(data["slug"].(string))
	topic := fmt.Sprintf("homeassistant/sensor/%s/%s/state", data["identifier"], data["slug"].(string))

	payload := map[string]string{
		"value": data["value"].(string),
	}
	if !isTextSensor {
		payload["unit_of_measurement"] = data["unit_of_measurement"].(string)
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
