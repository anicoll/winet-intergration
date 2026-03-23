package feedin

import (
	"strconv"
	"sync"
	"time"

	"go.uber.org/zap"

	dbpkg "github.com/anicoll/winet-integration/internal/pkg/database/db"
	"github.com/anicoll/winet-integration/internal/pkg/model"
)

const (
	commandTTL      = 10 * time.Minute
	cutoffHour      = 17
	cutoffMinute    = 30
	exportPowerSlug = "export_power" // Sungrow "Export Power" data point (kW)
	feedinChannel   = "feedIn"
)

// FeedinSetter is the minimal interface to enable/disable feed-in limitation.
type FeedinSetter interface {
	SetFeedInLimitation(feedinLimited bool) (bool, error)
}

// Controller evaluates current conditions each time a fresh Amber price update arrives
// and conditionally enables grid feed-in export.
//
// State is kept entirely in memory:
//   - lastCommandAt tracks when the feed-in command was last sent (10-min TTL prevents spam
//     even if the electricity provider overrides the inverter setting).
//   - latestExportPower is updated from real-time inverter data so we skip the command if
//     the inverter is already exporting.
type Controller struct {
	mu                sync.Mutex
	lastCommandAt     time.Time
	latestExportPower *float64 // nil = no data yet; 0 = not exporting; >0 = already exporting
	inverter          FeedinSetter
	location          *time.Location
	logger            *zap.Logger
}

// New creates a Controller.
func New(inverter FeedinSetter, location *time.Location) *Controller {
	return &Controller{
		inverter: inverter,
		location: location,
		logger:   zap.L(),
	}
}

// UpdateFromStatuses is called from handleRealMessage with each poll's device statuses.
// It extracts and caches the current export power so Evaluate can check it without a DB
// round-trip. It is intentionally non-blocking — only a mutex-protected memory write.
func (c *Controller) UpdateFromStatuses(statuses []model.DeviceStatus) {
	for _, s := range statuses {
		if s.Slug != exportPowerSlug {
			continue
		}
		c.mu.Lock()
		if s.Value != nil {
			if v, err := strconv.ParseFloat(*s.Value, 64); err == nil {
				c.latestExportPower = &v
			}
		} else {
			c.latestExportPower = nil
		}
		c.mu.Unlock()
		return
	}
	// If the slug is absent (e.g. battery-only device round), preserve the last known value.
}

// Evaluate is called each time fresh Amber prices are fetched (every ~5 min).
// Prices are passed in directly from the fetch result — no extra DB query needed.
// It decides whether to enable feed-in based on time-of-day, sell price, inverter state,
// and the in-memory TTL.
func (c *Controller) Evaluate(prices []dbpkg.Amberprice) {
	now := time.Now().In(c.location)

	// Don't bother past 5:30 PM — sun is going down or feed-in is already running.
	cutoff := time.Date(now.Year(), now.Month(), now.Day(), cutoffHour, cutoffMinute, 0, 0, c.location)
	if now.After(cutoff) {
		return
	}

	c.mu.Lock()
	lastCmd := c.lastCommandAt
	exportPower := c.latestExportPower
	c.mu.Unlock()

	// Respect the TTL — re-send at most once per 10 minutes so that a provider override
	// is corrected but we don't spam the inverter every price tick.
	if !lastCmd.IsZero() && time.Since(lastCmd) < commandTTL {
		return
	}

	// No inverter data yet — wait for the first real-data poll before acting.
	if exportPower == nil {
		c.logger.Debug("feedin: no inverter data yet, skipping")
		return
	}

	// If the inverter is already exporting there is nothing to do.
	if *exportPower > 0 {
		c.logger.Debug("feedin: already exporting, skipping", zap.Float64("export_kw", *exportPower))
		return
	}

	price, found := currentFeedinPrice(prices)
	if !found {
		c.logger.Debug("feedin: no current feed-in price found")
		return
	}
	if price.PerKwh <= 0 {
		c.logger.Debug("feedin: feed-in price not positive, skipping", zap.Float64("per_kwh", price.PerKwh))
		return
	}

	c.logger.Info("feedin: enabling feed-in export", zap.Float64("per_kwh", price.PerKwh))
	success, err := c.inverter.SetFeedInLimitation(false)
	if err != nil {
		c.logger.Error("feedin: failed to enable feed-in", zap.Error(err))
		return
	}
	if success {
		c.mu.Lock()
		c.lastCommandAt = time.Now()
		c.mu.Unlock()
		c.logger.Info("feedin: feed-in enabled successfully")
	}
}

// currentFeedinPrice returns the non-forecast feed-in Amber price covering right now.
func currentFeedinPrice(prices []dbpkg.Amberprice) (dbpkg.Amberprice, bool) {
	now := time.Now().UTC()
	for _, p := range prices {
		if p.ChannelType != feedinChannel {
			continue
		}
		if !p.Forecast && p.StartTime.UTC().Before(now) && p.EndTime.UTC().After(now) {
			return p, true
		}
	}
	return dbpkg.Amberprice{}, false
}
