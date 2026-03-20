package publisher

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/anicoll/winet-integration/internal/pkg/model"
)

// DataPoint is a normalized sensor reading ready for publishing.
type DataPoint struct {
	Value             string
	Slug              string
	Timestamp         time.Time
	Identifier        string
	UnitOfMeasurement string
}

// Publisher is implemented by each backend (MQTT, database, etc.).
type Publisher interface {
	Write(ctx context.Context, data []DataPoint) error
	RegisterDevice(ctx context.Context, device *model.Device) error
}

// DataPublisher is the interface injected into the winet service.
// MultiPublisher implements it.
type DataPublisher interface {
	PublishData(ctx context.Context, devices map[model.Device][]model.DeviceStatus) error
	RegisterDevice(ctx context.Context, device *model.Device) error
}

// MultiPublisher normalizes device data and fans it out to a set of Publisher backends.
type MultiPublisher struct {
	publishers []Publisher
	normalizer Normalizer
	sensors    sync.Map
}

// NewMultiPublisher returns a MultiPublisher that writes to all given backends.
func NewMultiPublisher(publishers ...Publisher) *MultiPublisher {
	return &MultiPublisher{publishers: publishers}
}

// PublishData normalizes device statuses and writes deduplicated DataPoints to all backends.
func (m *MultiPublisher) PublishData(ctx context.Context, deviceStatusMap map[model.Device][]model.DeviceStatus) error {
	data := make([]DataPoint, 0)
	for device, statuses := range deviceStatusMap {
		for _, status := range statuses {
			dp, skip := m.normalizer.Normalize(device, status)
			if skip {
				continue
			}
			key := fmt.Sprintf("%s_%s", dp.Identifier, dp.Slug)
			if !m.shouldUpdate(key, dp.Value) {
				continue
			}
			data = append(data, dp)
		}
	}
	for _, p := range m.publishers {
		if err := p.Write(ctx, data); err != nil {
			zap.L().Error("failed to publish data", zap.Error(err))
		}
	}
	zap.L().Debug("published data points", zap.Int("count", len(data)))
	return nil
}

// RegisterDevice registers the device with all backends.
func (m *MultiPublisher) RegisterDevice(ctx context.Context, device *model.Device) error {
	for _, p := range m.publishers {
		if err := p.RegisterDevice(ctx, device); err != nil {
			zap.L().Error("failed to register device", zap.Error(err), zap.String("device", device.SerialNumber))
		}
	}
	return nil
}

func (m *MultiPublisher) shouldUpdate(key, newValue string) bool {
	oldValue, exists := m.sensors.Load(key)
	if exists && strings.EqualFold(newValue, oldValue.(string)) {
		return false
	}
	if !exists {
		zap.L().Info("configured sensor", zap.String("key", key), zap.String("value", newValue))
	} else {
		zap.L().Debug("updated sensor", zap.String("key", key), zap.String("value", newValue))
	}
	m.sensors.Store(key, newValue)
	return true
}
