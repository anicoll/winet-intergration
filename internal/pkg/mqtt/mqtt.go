package mqtt

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/anicoll/winet-integration/internal/pkg/model"
	paho_mqtt "github.com/eclipse/paho.mqtt.golang"
)

type service struct {
	client            paho_mqtt.Client
	configuredDevices map[string]struct{}
}

func New(client paho_mqtt.Client) *service {
	return &service{
		client:            client,
		configuredDevices: make(map[string]struct{}),
	}
}

func (s *service) Connect() error {
	token := s.client.Connect()
	res := token.WaitTimeout(time.Second * 5)
	if res {
		return nil
	}
	if err := token.Error(); err != nil {
		return err
	}
	return errors.New("unable to connect in time")
}

func (s *service) PublishData(deviceStatusMap map[model.Device][]model.DeviceStatus) error {
	for device, statuses := range deviceStatusMap {
		for _, status := range statuses {
			if err := s.RegisterDevice(&device); err != nil {
				return err
			}
			value := new(big.Rat)
			value, _ = value.SetString(*status.Value)
			if status.Unit == "kWp" {
				status.Unit = "kW"
			}
			if status.Unit == "℃" {
				status.Unit = "°C"
			}
			if status.Unit == "kvar" {
				status.Unit = "var"
				value = value.Mul(value, new(big.Rat).SetInt64(1000))
			}
			if status.Unit == "kVA" {
				status.Unit = "VA"
				value = value.Mul(value, new(big.Rat).SetInt64(1000))
			}

			slugIdentifier := fmt.Sprintf("%s_%s", device.Model, device.SerialNumber)
			topic := fmt.Sprintf("homeassistant/sensor/%s/%s/state", slugIdentifier, status.Slug)

			payload := map[string]string{
				"value": value.FloatString(4),
			}
			if !model.TextSensors.HasSlug(status.Slug) {
				payload["unit_of_measurement"] = status.Unit
			}
			data, err := json.Marshal(payload)
			if err != nil {
				return err
			}
			token := s.client.Publish(topic, 0, false, data)
			res := token.WaitTimeout(time.Second * 10)
			if res {
				return nil
			}
			if err := token.Error(); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *service) RegisterDevice(device *model.Device) error {
	if device == nil {
		return nil
	}
	if _, exists := s.configuredDevices[device.ID]; exists {
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
	if res := token.WaitTimeout(time.Second * 5); res {
		return nil
	}
	if err := token.Error(); err != nil {
		return err
	}
	s.configuredDevices[device.ID] = struct{}{}
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
