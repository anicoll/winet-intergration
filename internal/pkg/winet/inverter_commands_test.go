package winet

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	// Assuming newTestService and পেতেObserver are accessible if in the same package,
	// or defined in a shared test utility.
	// For now, we'll add পেতেObserver locally if needed.
)

// TestInverterCommands_ProcessedChannelRemoved tests the behavior of inverter command functions
// after the s.processed channel and s.waiter() mechanism have been removed.
func TestInverterCommands_ProcessedChannelRemoved(t *testing.T) {
	expectedError := errors.New("s.processed reply mechanism removed, command result unknown")

	// Define test cases for each command function
	testCases := []struct {
		name                 string
		commandFunc          func(s *service) (bool, error)
		expectedLogMessage   string
		commandSpecificInput interface{} // For commands that take parameters
	}{
		{
			name: "SendSelfConsumptionCommand",
			commandFunc: func(s *service) (bool, error) {
				return s.SendSelfConsumptionCommand()
			},
			expectedLogMessage: "SendSelfConsumptionCommand: s.processed reply mechanism removed, command result unknown.",
		},
		{
			name: "SendDischargeCommand",
			commandFunc: func(s *service) (bool, error) {
				power := "1000" // Example power
				if p, ok := testCases[1].commandSpecificInput.(string); ok { // Get specific input for this test case
					power = p
				}
				return s.SendDischargeCommand(power)
			},
			expectedLogMessage:   "SendDischargeCommand: s.processed reply mechanism removed, command result unknown.",
			commandSpecificInput: "1234", // Specific input for this test
		},
		{
			name: "SendChargeCommand",
			commandFunc: func(s *service) (bool, error) {
				return s.SendChargeCommand("1000") // Example power
			},
			expectedLogMessage: "SendChargeCommand: s.processed reply mechanism removed, command result unknown.",
		},
		{
			name: "SendBatteryStopCommand",
			commandFunc: func(s *service) (bool, error) {
				return s.SendBatteryStopCommand()
			},
			expectedLogMessage: "SendBatteryStopCommand: s.processed reply mechanism removed, command result unknown.",
		},
		{
			name: "SendInverterStateChangeCommand",
			commandFunc: func(s *service) (bool, error) {
				return s.SendInverterStateChangeCommand(true) // Example: disable true
			},
			expectedLogMessage: "SendInverterStateChangeCommand: s.processed reply mechanism removed, command result unknown.",
		},
		{
			name: "SetFeedInLimitation",
			commandFunc: func(s *service) (bool, error) {
				return s.SetFeedInLimitation(true) // Example: feedinLimited true
			},
			expectedLogMessage: "SetFeedInLimitation: s.processed reply mechanism removed, command result unknown.",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			core, observedLogs := getLocalObserver(zapcore.WarnLevel) // Capture Warn level logs
			logger := zap.New(core)

			s := newTestService(t, logger) // Uses the helper from winet_test.go

			// Call the specific command function
			// For SendDischargeCommand, we need to pass its specific input if defined
			var success bool
			var err error
			if tc.name == "SendDischargeCommand" {
				power, _ := tc.commandSpecificInput.(string)
				success, err = s.SendDischargeCommand(power)
			} else {
				success, err = tc.commandFunc(s)
			}


			// Assertions
			assert.False(t, success, "Command function should return false.")
			assert.EqualError(t, err, expectedError.Error(), "Command function should return the specific error.")

			// Assert that a warning is logged
			foundLog := false
			for _, log := range observedLogs.All() {
				if strings.Contains(log.Message, tc.expectedLogMessage) {
					foundLog = true
					break
				}
			}
			assert.True(t, foundLog, "Expected warning log message not found: %s", tc.expectedLogMessage)
		})
	}
}

// getLocalObserver is a local copy for this test file.
// In a real scenario, this would be in a shared test utility package or file.
func getLocalObserver(level zapcore.LevelEnabler) (zapcore.Core, *zaptest.Observer) {
	core, observer := zaptest.NewTestingObservatory(level)
	return core, observer
}
