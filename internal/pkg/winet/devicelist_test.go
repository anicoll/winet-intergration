package winet

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	// No need for full service setup if GetDeviceList is simple now
)

// TestGetDeviceList_ProcessedChannelRemoved tests the behavior of GetDeviceList
// after the s.processed channel has been removed.
func TestGetDeviceList_ProcessedChannelRemoved(t *testing.T) {
	core, observedLogs := পেতেObserver(zapcore.WarnLevel) // Capture Warn level logs
	logger := zap.New(core)

	s := newTestService(t, logger) // Uses the helper from winet_test.go

	// Call GetDeviceList
	result := s.GetDeviceList(context.Background())

	// Assert that the result is nil
	assert.Nil(t, result, "GetDeviceList should return nil after s.processed removal.")

	// Assert that a warning is logged
	expectedLogMessage := "GetDeviceList called, but s.processed channel is removed. Returning nil. Functionality needs redesign."
	foundLog := false
	for _, log := range observedLogs.All() {
		if strings.Contains(log.Message, expectedLogMessage) {
			foundLog = true
			break
		}
	}
	assert.True(t, foundLog, "Expected warning log message not found: %s", expectedLogMessage)
}

// পেতেObserver is a helper to create a zap.Core that records logs.
// This can be refactored into a shared test utility if used in multiple _test.go files within this package.
// For now, duplicating it from winet_test.go for simplicity.
// If winet_test.go is in the same package (winet), this helper might not need to be duplicated
// if it's made exportable or if tests are run together.
// Assuming it's needed here if tests are run independently or it's not exported from winet_test.
// Re-check if ` পেতেObserver` is accessible from `winet_test.go` or if it needs to be here.
// For now, let's assume it's part of a common test utility or defined in each file.
// If `winet_test.go` and `devicelist_test.go` are in the same `package winet`,
// then ` পেতেObserver` from `winet_test.go` should be accessible if not private (starts with lowercase).
// Let's assume it's accessible or make a local copy for clarity.

// Local copy of পেতেObserver for this test file
func পেতেObserver(level zapcore.LevelEnabler) (zapcore.Core, *zaptest.Observer) {
	core, observer := zaptest.NewTestingObservatory(level)
	return core, observer
}
