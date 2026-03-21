package publisher

import (
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/anicoll/winet-integration/internal/pkg/model"
)

// Normalizer converts raw DeviceStatus readings into typed DataPoints.
// It applies unit normalization and filters out ignored slugs.
type Normalizer struct{}

// Normalize converts a single device status into a DataPoint.
// Returns (DataPoint{}, true) if the reading should be skipped.
func (Normalizer) Normalize(device model.Device, status model.DeviceStatus) (DataPoint, bool) {
	if ignoreSlug(status.Slug) {
		return DataPoint{}, true
	}

	slugIdentifier := fmt.Sprintf("%s_%s", strings.Replace(device.Model, ".", "", -1), device.SerialNumber)
	isTextSensor := model.TextSensors.HasSlug(status.Slug)

	var val string
	if !isTextSensor {
		if status.Value == nil || *status.Value == "--" {
			z := "0.00"
			status.Value = &z
		}
		value := new(big.Rat)
		value, _ = value.SetString(*status.Value)
		switch status.Unit {
		case "kWp":
			status.Unit = "kW"
		case "℃":
			status.Unit = "°C"
		case "kvar":
			status.Unit = "var"
			value = value.Mul(value, new(big.Rat).SetInt64(1000))
		case "kVA":
			status.Unit = "VA"
			value = value.Mul(value, new(big.Rat).SetInt64(1000))
		}
		val = value.FloatString(4)
	} else {
		if status.Value == nil {
			return DataPoint{}, true
		}
		val = *status.Value
	}

	return DataPoint{
		Value:             val,
		Slug:              status.Slug,
		Timestamp:         time.Now(),
		Identifier:        slugIdentifier,
		UnitOfMeasurement: status.Unit,
	}, false
}

func ignoreSlug(slug string) bool {
	switch slug {
	case "grid_frequency",
		"phase_a_voltage",
		"phase_a_current",
		"phase_a_backup_current",
		"phase_b_backup_current",
		"phase_c_backup_current",
		"phase_a_backup_voltage",
		"phase_b_backup_voltage",
		"phase_c_backup_voltage",
		"backup_frequency",
		"phase_a_backup_power",
		"phase_b_backup_power",
		"phase_c_backup_power",
		"total_backup_power",
		"meter_grid_freq",
		"reactive_power_uploaded_by_meter",
		"meter_phase_a_voltage",
		"meter_phase_b_voltage",
		"meter_phase_c_voltage",
		"meter_phase_a_current",
		"meter_phase_b_current",
		"meter_phase_c_current",
		"bus_voltage",
		"array_insulation_resistance",
		"battery_current":
		return true
	}
	return false
}
