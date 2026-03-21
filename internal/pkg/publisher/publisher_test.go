package publisher

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anicoll/winet-integration/internal/pkg/model"
)

// stubPublisher is a simple in-process Publisher for testing fan-out behaviour.
type stubPublisher struct {
	writes    [][]DataPoint
	registers []*model.Device
	writeErr  error
}

func (s *stubPublisher) Write(_ context.Context, data []DataPoint) error {
	cp := make([]DataPoint, len(data))
	copy(cp, data)
	s.writes = append(s.writes, cp)
	return s.writeErr
}

func (s *stubPublisher) RegisterDevice(_ context.Context, device *model.Device) error {
	s.registers = append(s.registers, device)
	return nil
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

// --- Normalizer ---

func TestNormalizer_IgnoredSlug_Skips(t *testing.T) {
	n := Normalizer{}
	device := model.Device{ID: "1", Model: "XH3000", SerialNumber: "SN001"}
	v := "50.0"
	_, skip := n.Normalize(device, model.DeviceStatus{Slug: "grid_frequency", Unit: "Hz", Value: &v})
	assert.True(t, skip)
}

func TestNormalizer_UnitConversions(t *testing.T) {
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

	n := Normalizer{}
	device := model.Device{ID: "1", Model: "XH3000", SerialNumber: "SN-CONV"}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.inputUnit, func(t *testing.T) {
			v := tc.inputValue
			dp, skip := n.Normalize(device, model.DeviceStatus{Slug: "test_power", Unit: tc.inputUnit, Value: &v})
			require.False(t, skip)
			assert.Equal(t, tc.wantUnit, dp.UnitOfMeasurement, "unit mismatch")
			assert.Equal(t, tc.wantValue, dp.Value, "value mismatch")
		})
	}
}

func TestNormalizer_NilValueBecomesZero(t *testing.T) {
	n := Normalizer{}
	device := model.Device{ID: "1", Model: "XH3000", SerialNumber: "SN-NIL"}
	dp, skip := n.Normalize(device, model.DeviceStatus{Slug: "battery_power", Unit: "kW", Value: nil})
	require.False(t, skip)
	assert.Equal(t, "0.0000", dp.Value)
}

func TestNormalizer_DashValueBecomesZero(t *testing.T) {
	n := Normalizer{}
	device := model.Device{ID: "1", Model: "XH3000", SerialNumber: "SN-DASH"}
	v := "--"
	dp, skip := n.Normalize(device, model.DeviceStatus{Slug: "battery_power", Unit: "kW", Value: &v})
	require.False(t, skip)
	assert.Equal(t, "0.0000", dp.Value)
}

func TestNormalizer_TextSensor_PassesThroughRawValue(t *testing.T) {
	n := Normalizer{}
	device := model.Device{ID: "1", Model: "XH3000", SerialNumber: "SN-TEXT"}
	v := "Running"
	dp, skip := n.Normalize(device, model.DeviceStatus{Slug: "running_status", Unit: "", Value: &v})
	require.False(t, skip)
	assert.Equal(t, "Running", dp.Value)
}

func TestNormalizer_TextSensor_NilValue_Skips(t *testing.T) {
	n := Normalizer{}
	device := model.Device{ID: "1", Model: "XH3000", SerialNumber: "SN-TXNIL"}
	_, skip := n.Normalize(device, model.DeviceStatus{Slug: "running_status", Unit: "", Value: nil})
	assert.True(t, skip)
}

func TestNormalizer_SlugIdentifier_DotsStripped(t *testing.T) {
	n := Normalizer{}
	device := model.Device{ID: "1", Model: "SH5.0RS", SerialNumber: "SN001"}
	v := "1.0"
	dp, skip := n.Normalize(device, model.DeviceStatus{Slug: "battery_power", Unit: "kW", Value: &v})
	require.False(t, skip)
	assert.Equal(t, "SH50RS_SN001", dp.Identifier)
}

// --- MultiPublisher ---

func TestMultiPublisher_PublishData_FansOutToAllBackends(t *testing.T) {
	p1, p2 := &stubPublisher{}, &stubPublisher{}
	mp := NewMultiPublisher(p1, p2)

	v := "10.0"
	device := model.Device{ID: "1", Model: "XH3000", SerialNumber: "SN-FANOUT"}
	require.NoError(t, mp.PublishData(context.Background(), map[model.Device][]model.DeviceStatus{
		device: {{Slug: "battery_power", Unit: "kW", Value: &v}},
	}))

	assert.Len(t, p1.writes, 1)
	assert.Len(t, p1.writes[0], 1)
	assert.Len(t, p2.writes, 1)
	assert.Len(t, p2.writes[0], 1)
}

func TestMultiPublisher_PublishData_IgnoredSlugNotSent(t *testing.T) {
	p1 := &stubPublisher{}
	mp := NewMultiPublisher(p1)

	v := "50.0"
	device := model.Device{ID: "1", Model: "XH3000", SerialNumber: "SN-IGN"}
	require.NoError(t, mp.PublishData(context.Background(), map[model.Device][]model.DeviceStatus{
		device: {{Slug: "grid_frequency", Unit: "Hz", Value: &v}},
	}))

	require.Len(t, p1.writes, 1)
	assert.Empty(t, p1.writes[0], "ignored slug must produce an empty data set")
}

func TestMultiPublisher_PublishData_DeduplicatesUnchangedValues(t *testing.T) {
	p1 := &stubPublisher{}
	mp := NewMultiPublisher(p1)

	device := model.Device{ID: "1", Model: "XH3000", SerialNumber: "SN-DEDUP"}
	publish := func(val string) {
		v := val
		_ = mp.PublishData(context.Background(), map[model.Device][]model.DeviceStatus{
			device: {{Slug: "battery_power", Unit: "kW", Value: &v}},
		})
	}

	publish("10.0") // new → written
	publish("10.0") // unchanged → skipped
	publish("11.0") // changed → written

	require.Len(t, p1.writes, 3, "Write called once per PublishData invocation")
	assert.Len(t, p1.writes[0], 1, "first call: 1 new datapoint")
	assert.Empty(t, p1.writes[1], "second call: 0 datapoints (unchanged)")
	assert.Len(t, p1.writes[2], 1, "third call: 1 updated datapoint")
}

func TestMultiPublisher_RegisterDevice_FansOutToAllBackends(t *testing.T) {
	p1, p2 := &stubPublisher{}, &stubPublisher{}
	mp := NewMultiPublisher(p1, p2)

	device := &model.Device{ID: "42", Model: "XH3000", SerialNumber: "SN-REG"}
	require.NoError(t, mp.RegisterDevice(context.Background(), device))

	assert.Equal(t, []*model.Device{device}, p1.registers)
	assert.Equal(t, []*model.Device{device}, p2.registers)
}

func TestMultiPublisher_PublishData_BackendErrorDoesNotStopOtherBackends(t *testing.T) {
	p1 := &stubPublisher{writeErr: errors.New("backend down")}
	p2 := &stubPublisher{}
	mp := NewMultiPublisher(p1, p2)

	v := "5.0"
	device := model.Device{ID: "1", Model: "XH3000", SerialNumber: "SN-ERR"}
	// MultiPublisher logs errors but always returns nil.
	require.NoError(t, mp.PublishData(context.Background(), map[model.Device][]model.DeviceStatus{
		device: {{Slug: "battery_power", Unit: "kW", Value: &v}},
	}))

	// p2 must still have been called despite p1 failing.
	assert.Len(t, p2.writes, 1)
}
