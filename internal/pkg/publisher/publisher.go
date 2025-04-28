package publisher

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/anicoll/winet-integration/internal/pkg/model"
	"go.uber.org/zap"
)

var errAlreadyRegistered = errors.New("publisher already registered")

var (
	registerdPublishers = make(map[string]publisher)
	sensors             sync.Map
)

type publisher interface {
	// PublishData publishes the device status data to the registered adapters
	Write(ctx context.Context, data []map[string]any) error
	RegisterDevice(device *model.Device) error
}

func RegisterPublisher(name string, publisher publisher) error {
	if _, ok := registerdPublishers[name]; ok {
		return errAlreadyRegistered
	}
	registerdPublishers[name] = publisher
	return nil
}

func PublishData(ctx context.Context, deviceStatusMap map[model.Device][]model.DeviceStatus) error {
	count := 0
	data := make([]map[string]any, 0)
	for device, statuses := range deviceStatusMap {
		for _, status := range statuses {
			isTextSensor := model.TextSensors.HasSlug(status.Slug)
			val := ""
			if (!isTextSensor && status.Value == nil) || *status.Value == "--" {
				status.Value = func() *string {
					s := "0.00"
					return &s
				}()
			}

			if !isTextSensor {
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
				val = value.FloatString(4)
			} else {
				val = *status.Value
			}

			slugIdentifier := fmt.Sprintf("%s_%s", strings.Replace(device.Model, ".", "", -1), device.SerialNumber)

			if !shouldUpdate(slugIdentifier, status.Slug, val) {
				continue
			}
			count++
			payload := map[string]any{
				"value":               val,
				"slug":                status.Slug,
				"timestamp":           time.Now(),
				"identifier":          slugIdentifier,
				"unit_of_measurement": status.Unit,
			}
			data = append(data, payload)
		}
	}
	for name, publisher := range registerdPublishers {
		if err := publisher.Write(ctx, data); err != nil {
			zap.L().Error("failed to publish data", zap.Error(err), zap.String("publisher", name))
			continue
		}
		zap.L().Debug("updated sensors", zap.Int("count", count), zap.String("publisher", name))
	}
	return nil
}

func RegisterDevice(device *model.Device) error {
	for name, publisher := range registerdPublishers {
		if err := publisher.RegisterDevice(device); err != nil {
			zap.L().Error("failed to register device", zap.Error(err), zap.String("publisher", name))
			continue
		}
		zap.L().Debug("registered device", zap.String("device", device.SerialNumber), zap.String("publisher", name))
	}
	return nil
}

func shouldUpdate(identifier, slug, newValue string) bool {
	key := fmt.Sprintf("%s_%s", identifier, slug)
	oldValue, exists := sensors.Load(key)
	if exists && strings.EqualFold(newValue, oldValue.(string)) {
		return false
	}
	if !exists {
		zap.L().Info("Configured sensor:", zap.String("device", identifier), zap.String("sensor", slug), zap.String("value", newValue))
	} else {
		zap.L().Info("Configured sensor:", zap.String("device", identifier), zap.String("sensor", slug), zap.String("value", newValue))
	}
	sensors.Store(key, newValue)
	return true
}
