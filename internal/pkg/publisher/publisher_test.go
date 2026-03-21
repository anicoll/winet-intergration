package publisher

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anicoll/winet-integration/internal/pkg/model"
	publishermocks "github.com/anicoll/winet-integration/mocks/publisher"
)

// resetState clears all package-level state so tests are isolated.
func resetState() {
	publishersMu.Lock()
	registerdPublishers = make(map[string]Publisher)
	publishersMu.Unlock()
	sensors = sync.Map{}
}

// --- ignoreSlug ---

func TestIgnoreSlug_KnownIgnoredSlugs(t *testing.T) {
	ignored := []string{
		"grid_frequency",
		"phase_a_voltage",
		"phase_a_current",
		"phase_a_backup_current",
		"phase_b_backup_current",
		"phase_c_backup_current",
		"bus_voltage",
		"battery_current",
		"meter_phase_a_voltage",
	}
	for _, slug := range ignored {
		assert.True(t, ignoreSlug(slug), "slug %q should be ignored", slug)
	}
}

func TestIgnoreSlug_NonIgnoredSlugs(t *testing.T) {
	notIgnored := []string{
		"battery_power",
		"total_active_power",
		"battery_state_of_charge",
		"running_status",
		"pv_generation_today",
	}
	for _, slug := range notIgnored {
		assert.False(t, ignoreSlug(slug), "slug %q should NOT be ignored", slug)
	}
}

// --- RegisterPublisher ---

func TestRegisterPublisher_Success(t *testing.T) {
	resetState()
	err := RegisterPublisher("pub-a", publishermocks.NewPublisher(t))
	require.NoError(t, err)
}

func TestRegisterPublisher_DuplicateNameReturnsError(t *testing.T) {
	resetState()
	require.NoError(t, RegisterPublisher("dup", publishermocks.NewPublisher(t)))
	err := RegisterPublisher("dup", publishermocks.NewPublisher(t))
	require.ErrorIs(t, err, errAlreadyRegistered)
}

// --- PublishData unit conversions ---

func TestPublishData_UnitConversions(t *testing.T) {
	cases := []struct {
		inputUnit  string
		inputValue string
		wantUnit   string
		wantValue  string
	}{
		{"kWp", "5.0", "kW", "5.0000"},
		{"℃", "25.0", "°C", "25.0000"},
		{"kvar", "1.0", "var", "1000.0000"},
		{"kVA", "2.5", "VA", "2500.0000"},
		{"kW", "3.2", "kW", "3.2000"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.inputUnit, func(t *testing.T) {
			resetState()
			pub := publishermocks.NewPublisher(t)
			require.NoError(t, RegisterPublisher("conv", pub))

			var capturedData []map[string]any
			pub.EXPECT().Write(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, data []map[string]any) error {
				capturedData = data
				return nil
			})

			device := model.Device{ID: "1", Model: "XH3000", SerialNumber: "SN-CONV"}
			val := tc.inputValue
			slug := "test_power_" + tc.inputUnit
			err := PublishData(context.Background(), map[model.Device][]model.DeviceStatus{
				device: {{Name: "Test", Slug: slug, Unit: tc.inputUnit, Value: &val}},
			})
			require.NoError(t, err)

			require.Len(t, capturedData, 1)
			assert.Equal(t, tc.wantUnit, capturedData[0]["unit_of_measurement"], "unit mismatch for %s", tc.inputUnit)
			assert.Equal(t, tc.wantValue, capturedData[0]["value"], "value mismatch for %s %s", tc.inputValue, tc.inputUnit)
		})
	}
}

// --- PublishData value behaviour ---

func TestPublishData_NilValueBecomesZero(t *testing.T) {
	resetState()
	pub := publishermocks.NewPublisher(t)
	require.NoError(t, RegisterPublisher("nil-val", pub))

	var capturedData []map[string]any
	pub.EXPECT().Write(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, data []map[string]any) error {
		capturedData = data
		return nil
	})

	device := model.Device{ID: "1", Model: "XH3000", SerialNumber: "SN-NILVAL"}
	err := PublishData(context.Background(), map[model.Device][]model.DeviceStatus{
		device: {{Slug: "battery_power_nilval", Unit: "kW", Value: nil}},
	})
	require.NoError(t, err)

	require.Len(t, capturedData, 1, "nil value should be replaced with 0.00 and published")
	assert.Equal(t, "0.0000", capturedData[0]["value"])
}

func TestPublishData_DashDashValueBecomesZero(t *testing.T) {
	resetState()
	pub := publishermocks.NewPublisher(t)
	require.NoError(t, RegisterPublisher("dash-val", pub))

	var capturedData []map[string]any
	pub.EXPECT().Write(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, data []map[string]any) error {
		capturedData = data
		return nil
	})

	device := model.Device{ID: "1", Model: "XH3000", SerialNumber: "SN-DASH"}
	v := "--"
	err := PublishData(context.Background(), map[model.Device][]model.DeviceStatus{
		device: {{Slug: "battery_power_dash", Unit: "kW", Value: &v}},
	})
	require.NoError(t, err)

	require.Len(t, capturedData, 1)
	assert.Equal(t, "0.0000", capturedData[0]["value"])
}

func TestPublishData_IgnoredSlugsNotPublished(t *testing.T) {
	resetState()
	pub := publishermocks.NewPublisher(t)
	require.NoError(t, RegisterPublisher("ignore-slug", pub))

	var capturedData []map[string]any
	pub.EXPECT().Write(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, data []map[string]any) error {
		capturedData = data
		return nil
	})

	v := "50.0"
	device := model.Device{ID: "1", Model: "XH3000", SerialNumber: "SN-IGNORE"}
	err := PublishData(context.Background(), map[model.Device][]model.DeviceStatus{
		device: {{Slug: "grid_frequency", Unit: "Hz", Value: &v}},
	})
	require.NoError(t, err)

	assert.Empty(t, capturedData, "ignored slugs should produce an empty data set")
}

func TestPublishData_DeduplicatesUnchangedValues(t *testing.T) {
	resetState()
	pub := publishermocks.NewPublisher(t)
	require.NoError(t, RegisterPublisher("dedup", pub))

	var allWrites [][]map[string]any
	pub.Mock.On("Write", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		data, _ := args.Get(1).([]map[string]any)
		cp := make([]map[string]any, len(data))
		copy(cp, data)
		allWrites = append(allWrites, cp)
	}).Return(nil)

	device := model.Device{ID: "1", Model: "XH3000", SerialNumber: "SN-DEDUP"}
	publish := func(val string) {
		v := val
		_ = PublishData(context.Background(), map[model.Device][]model.DeviceStatus{
			device: {{Slug: "dedup_power", Unit: "kW", Value: &v}},
		})
	}

	publish("10.0") // new value → written
	publish("10.0") // unchanged → skipped
	publish("11.0") // changed → written

	require.Len(t, allWrites, 3, "Write must be called once per PublishData invocation")
	assert.Len(t, allWrites[0], 1, "first call: 1 new datapoint")
	assert.Empty(t, allWrites[1], "second call: 0 datapoints (same value)")
	assert.Len(t, allWrites[2], 1, "third call: 1 changed datapoint")
}

func TestPublishData_TextSensorPassesThroughRawValue(t *testing.T) {
	resetState()
	pub := publishermocks.NewPublisher(t)
	require.NoError(t, RegisterPublisher("text", pub))

	var capturedData []map[string]any
	pub.EXPECT().Write(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, data []map[string]any) error {
		capturedData = data
		return nil
	})

	v := "Running"
	device := model.Device{ID: "1", Model: "XH3000", SerialNumber: "SN-TEXT"}
	err := PublishData(context.Background(), map[model.Device][]model.DeviceStatus{
		device: {{Slug: "running_status", Unit: "", Value: &v}},
	})
	require.NoError(t, err)

	require.Len(t, capturedData, 1)
	assert.Equal(t, "Running", capturedData[0]["value"], "text sensor value must not be processed through big.Rat")
}

// --- RegisterDevice ---

func TestRegisterDevice_CallsEachPublisher(t *testing.T) {
	resetState()
	pub := publishermocks.NewPublisher(t)
	require.NoError(t, RegisterPublisher("dev-reg", pub))

	device := &model.Device{ID: "42", Model: "XH3000", SerialNumber: "SN-REG"}
	pub.EXPECT().RegisterDevice(mock.Anything, device).Return(nil)

	err := RegisterDevice(device)
	require.NoError(t, err)
}
