package feedin

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dbpkg "github.com/anicoll/winet-integration/internal/pkg/database/db"
	"github.com/anicoll/winet-integration/internal/pkg/model"
)

// stubInverter records calls to SetFeedInLimitation for assertion.
type stubInverter struct {
	calls   []bool
	success bool
	err     error
}

func (s *stubInverter) SetFeedInLimitation(limited bool) (bool, error) {
	s.calls = append(s.calls, limited)
	return s.success, s.err
}

// feedinPrices returns a non-forecast feedIn price active around time.Now().
// Inside a synctest bubble time.Now() returns the fake clock value, so calling
// this after time.Sleep gives an interval centred on the advanced fake time.
func feedinPrices(perKwh float64) []dbpkg.Amberprice {
	now := time.Now().UTC()
	return []dbpkg.Amberprice{{
		ChannelType: feedinChannel,
		StartTime:   now.Add(-5 * time.Minute),
		EndTime:     now.Add(5 * time.Minute),
		PerKwh:      perKwh,
		Forecast:    false,
	}}
}

// --- UpdateFromStatuses ---

func TestUpdateFromStatuses_SetsExportPower(t *testing.T) {
	c := New(&stubInverter{}, time.UTC)
	c.UpdateFromStatuses([]model.DeviceStatus{
		{Slug: exportPowerSlug, Value: new("2.5")},
	})
	c.mu.Lock()
	defer c.mu.Unlock()
	require.NotNil(t, c.latestExportPower)
	assert.InDelta(t, 2.5, *c.latestExportPower, 0.001)
}

func TestUpdateFromStatuses_ZeroExportPower(t *testing.T) {
	c := New(&stubInverter{}, time.UTC)
	c.UpdateFromStatuses([]model.DeviceStatus{
		{Slug: exportPowerSlug, Value: new("0")},
	})
	c.mu.Lock()
	defer c.mu.Unlock()
	require.NotNil(t, c.latestExportPower)
	assert.Equal(t, 0.0, *c.latestExportPower)
}

func TestUpdateFromStatuses_NilValueClearsExportPower(t *testing.T) {
	c := New(&stubInverter{}, time.UTC)
	initial := 1.0
	c.latestExportPower = &initial

	c.UpdateFromStatuses([]model.DeviceStatus{
		{Slug: exportPowerSlug, Value: nil},
	})
	c.mu.Lock()
	defer c.mu.Unlock()
	assert.Nil(t, c.latestExportPower)
}

func TestUpdateFromStatuses_UnknownSlugPreservesLastValue(t *testing.T) {
	c := New(&stubInverter{}, time.UTC)
	initial := 3.0
	c.latestExportPower = &initial

	c.UpdateFromStatuses([]model.DeviceStatus{
		{Slug: "battery_power", Value: new("5.0")},
	})
	c.mu.Lock()
	defer c.mu.Unlock()
	require.NotNil(t, c.latestExportPower)
	assert.Equal(t, 3.0, *c.latestExportPower)
}

func TestUpdateFromStatuses_InvalidNumberIgnored(t *testing.T) {
	c := New(&stubInverter{}, time.UTC)
	initial := 1.0
	c.latestExportPower = &initial

	c.UpdateFromStatuses([]model.DeviceStatus{
		{Slug: exportPowerSlug, Value: new("not-a-number")},
	})
	c.mu.Lock()
	defer c.mu.Unlock()
	require.NotNil(t, c.latestExportPower)
	assert.Equal(t, 1.0, *c.latestExportPower)
}

// --- Evaluate: time-of-day cutoff ---
// synctest fake clock starts at 2000-01-01 00:00:00 UTC (midnight), which is
// naturally before the 17:30 cutoff. Advancing with time.Sleep controls the TOD.

func TestEvaluate_AfterCutoff_NoCommand(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		inv := &stubInverter{success: true}
		c := New(inv, time.UTC)
		c.latestExportPower = new(0.0)

		// Advance fake clock to 18:00 UTC — past the 17:30 cutoff.
		time.Sleep(18 * time.Hour)

		c.Evaluate(feedinPrices(0.10))
		assert.Empty(t, inv.calls, "no command should be sent after cutoff")
	})
}

func TestEvaluate_OneSecondPastCutoff_NoCommand(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		inv := &stubInverter{success: true}
		c := New(inv, time.UTC)
		c.latestExportPower = new(0.0)

		// Advance fake clock to 17:30:01 — one second past the cutoff.
		time.Sleep(time.Duration(cutoffHour)*time.Hour + time.Duration(cutoffMinute)*time.Minute + time.Second)

		c.Evaluate(feedinPrices(0.10))
		assert.Empty(t, inv.calls)
	})
}

func TestEvaluate_BeforeCutoff_AllowsCommand(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		inv := &stubInverter{success: true}
		c := New(inv, time.UTC)
		c.latestExportPower = new(0.0)

		// Midnight — well before cutoff, no sleep needed.
		c.Evaluate(feedinPrices(0.10))
		require.Len(t, inv.calls, 1)
	})
}

// --- Evaluate: TTL ---

func TestEvaluate_TTL_PreventsSpamThenAllowsRetry(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		inv := &stubInverter{success: true}
		c := New(inv, time.UTC)
		c.latestExportPower = new(0.0)

		// First call at midnight — should send.
		c.Evaluate(feedinPrices(0.10))
		require.Len(t, inv.calls, 1, "first evaluate must send command")

		// Advance to just before TTL expiry — must not send again.
		time.Sleep(commandTTL - time.Second)
		c.Evaluate(feedinPrices(0.10))
		assert.Len(t, inv.calls, 1, "within TTL: no repeat command")

		// Advance past TTL — should send again.
		time.Sleep(2 * time.Second)
		c.Evaluate(feedinPrices(0.10))
		assert.Len(t, inv.calls, 2, "after TTL expired: command re-sent")
	})
}

// --- Evaluate: inverter state ---

func TestEvaluate_NoInverterDataYet_NoCommand(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		inv := &stubInverter{success: true}
		c := New(inv, time.UTC)
		// latestExportPower is nil — no data received yet.

		c.Evaluate(feedinPrices(0.10))
		assert.Empty(t, inv.calls)
	})
}

func TestEvaluate_AlreadyExporting_NoCommand(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		inv := &stubInverter{success: true}
		c := New(inv, time.UTC)
		c.latestExportPower = new(1.2) // actively exporting

		c.Evaluate(feedinPrices(0.10))
		assert.Empty(t, inv.calls)
	})
}

// --- Evaluate: price conditions ---

func TestEvaluate_NoPrices_NoCommand(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		inv := &stubInverter{success: true}
		c := New(inv, time.UTC)
		c.latestExportPower = new(0.0)

		c.Evaluate(nil)
		assert.Empty(t, inv.calls)
	})
}

func TestEvaluate_ForecastPriceOnly_NoCommand(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		inv := &stubInverter{success: true}
		c := New(inv, time.UTC)
		c.latestExportPower = new(0.0)

		now := time.Now().UTC()
		prices := []dbpkg.Amberprice{{
			ChannelType: feedinChannel,
			StartTime:   now.Add(-5 * time.Minute),
			EndTime:     now.Add(5 * time.Minute),
			PerKwh:      0.15,
			Forecast:    true, // forecast must be ignored
		}}

		c.Evaluate(prices)
		assert.Empty(t, inv.calls)
	})
}

func TestEvaluate_ZeroFeedinPrice_NoCommand(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		inv := &stubInverter{success: true}
		c := New(inv, time.UTC)
		c.latestExportPower = new(0.0)

		c.Evaluate(feedinPrices(0.0))
		assert.Empty(t, inv.calls)
	})
}

func TestEvaluate_NegativeFeedinPrice_NoCommand(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		inv := &stubInverter{success: true}
		c := New(inv, time.UTC)
		c.latestExportPower = new(0.0)

		c.Evaluate(feedinPrices(-0.05))
		assert.Empty(t, inv.calls)
	})
}

func TestEvaluate_OnlyFeedinChannelConsidered(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		inv := &stubInverter{success: true}
		c := New(inv, time.UTC)
		c.latestExportPower = new(0.0)

		// Positive "general" price but no feedIn price — must not trigger.
		now := time.Now().UTC()
		prices := []dbpkg.Amberprice{{
			ChannelType: "general",
			StartTime:   now.Add(-5 * time.Minute),
			EndTime:     now.Add(5 * time.Minute),
			PerKwh:      0.30,
			Forecast:    false,
		}}

		c.Evaluate(prices)
		assert.Empty(t, inv.calls, "only the feedIn channel should trigger feed-in enable")
	})
}

// --- Evaluate: command outcome ---

func TestEvaluate_PositivePrice_NotExporting_EnablesFeedin(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		inv := &stubInverter{success: true}
		c := New(inv, time.UTC)
		c.latestExportPower = new(0.0)

		c.Evaluate(feedinPrices(0.12))

		require.Len(t, inv.calls, 1)
		assert.False(t, inv.calls[0], "SetFeedInLimitation(false) = enable export")
	})
}

func TestEvaluate_SuccessfulCommand_UpdatesTTL(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		inv := &stubInverter{success: true}
		c := New(inv, time.UTC)
		c.latestExportPower = new(0.0)

		before := time.Now()
		c.Evaluate(feedinPrices(0.12))

		c.mu.Lock()
		lastCmd := c.lastCommandAt
		c.mu.Unlock()

		assert.True(t, !lastCmd.IsZero() && !lastCmd.Before(before),
			"lastCommandAt must be set after a successful command")
	})
}

func TestEvaluate_FailedCommand_DoesNotUpdateTTL(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		inv := &stubInverter{success: false}
		c := New(inv, time.UTC)
		c.latestExportPower = new(0.0)

		c.Evaluate(feedinPrices(0.12))

		c.mu.Lock()
		lastCmd := c.lastCommandAt
		c.mu.Unlock()

		assert.True(t, lastCmd.IsZero(), "TTL must not be set when the command was not successful")
	})
}
