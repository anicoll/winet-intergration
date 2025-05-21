package winet

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/anicoll/winet-integration/internal/pkg/config"
	"github.com/anicoll/winet-integration/internal/pkg/model"
	ws "github.com/anicoll/winet-integration/pkg/sockets"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
	"go.uber.org/zap/zapcore"
)

// MockWsConnection is a mock implementation of ws.Connection
type MockWsConnection struct {
	SendFunc func(msg ws.Msg) error
	DialFunc func(ctx context.Context, urlStr string, userAgent string) error
	CloseFunc func() error
	IsClosedFunc func() bool
	GetReceiveChanFunc func() <-chan ws.Msg
}

func (m *MockWsConnection) Send(msg ws.Msg) error {
	if m.SendFunc != nil {
		return m.SendFunc(msg)
	}
	return nil
}
func (m *MockWsConnection) Dial(ctx context.Context, urlStr string, userAgent string) error { return nil }
func (m *MockWsConnection) Close() error { return nil }
func (m *MockWsConnection) IsClosed() bool { return false }
func (m *MockWsConnection) GetReceiveChan() <-chan ws.Msg { return nil }


func newTestService(t *testing.T, logger *zap.Logger) *service {
	t.Helper()
	if logger == nil {
		logger = zaptest.NewLogger(t)
	}
	// Replace global logger for the duration of this service's lifecycle in the test
	// This is important because s.logger = zap.L() in New()
	originalLogger := zap.L()
	zap.ReplaceGlobals(logger)
	t.Cleanup(func() {
		zap.ReplaceGlobals(originalLogger)
	})

	return New(&config.WinetConfig{}).(*service) // Cast to *service to access unexported fields like storedData
}


// TestOnMessage_StoredData_ExceedsMaxSize_ThenCleared tests that s.storedData is cleared
// if appending new data would exceed maxStoredDataSize, and then the new data is added.
func TestOnMessage_StoredData_ExceedsMaxSize_ThenCleared(t *testing.T) {
	core, observedLogs := পেতেObserver(zapcore.WarnLevel)
	logger := zap.New(core)
	
	s := newTestService(t, logger)
	mockConn := &MockWsConnection{}

	// 1. Fill storedData just below the limit with initial partial data
	// Assuming maxStoredDataSize is 1MB (1024*1024).
	// Let's use a smaller, more manageable maxStoredDataSize for testing if possible,
	// or simulate it by pre-filling storedData.
	// For this test, we'll directly manipulate s.storedData for setup,
	// as creating actual >1MB JSON strings is cumbersome.
	// The constant maxStoredDataSize is not exported, so we use its known value.
	// const localMaxStoredDataSize = 1 * 1024 * 1024
	// We'll use a smaller mock size for easier testing if we could redefine it per test,
	// but since it's a package const, we rely on its actual value.
	// Let's assume maxStoredDataSize is large. We'll make storedData almost full.

	almostMaxSize := maxStoredDataSize - 100
	s.storedData = []byte(strings.Repeat("{", almostMaxSize)) // Invalid JSON, but fills buffer

	// 2. Prepare incoming data that, when appended, would exceed maxStoredDataSize
	// This data itself is also partial/invalid JSON to trigger the buffering path.
	incomingData := []byte(strings.Repeat("{", 150)) // 150 bytes, (almostMaxSize + 150) > maxStoredDataSize

	// Expect a warning log
	expectedLogMessage := "s.storedData is about to exceed max size. Clearing buffer before appending. Some message data may be lost."

	// Call onMessage
	s.onMessage(incomingData, mockConn)

	// Assertions
	// Check logs for the warning
	foundLog := false
	for _, log := range observedLogs.All() {
		if strings.Contains(log.Message, expectedLogMessage) {
			foundLog = true
			// Check for expected fields in the log
			assert.Contains(t, log.ContextMap(), "current_stored_size")
			assert.Contains(t, log.ContextMap(), "incoming_data_size")
			assert.Contains(t, log.ContextMap(), "max_size")
			assert.Equal(t, int64(almostMaxSize), log.ContextMap()["current_stored_size"])
			assert.Equal(t, int64(len(incomingData)), log.ContextMap()["incoming_data_size"])
			assert.Equal(t, int64(maxStoredDataSize), log.ContextMap()["max_size"])
			break
		}
	}
	assert.True(t, foundLog, "Expected log message not found: %s", expectedLogMessage)

	// s.storedData should now contain only incomingData because it was cleared first.
	assert.Equal(t, incomingData, s.storedData, "s.storedData should contain only the new incomingData after being cleared.")
	assert.Equal(t, len(incomingData), len(s.storedData), "Length of s.storedData should be length of incomingData.")

	// Test the secondary check: if a single fragment itself makes storedData too large after append
	// and it's still a syntax error.
	s.storedData = []byte{} // Reset
	largeFragment := []byte(strings.Repeat("{", maxStoredDataSize+10))
	expectedLogMessageTooLarge := "s.storedData (after appending a fragment) exceeds max size. Clearing buffer."
	
	s.onMessage(largeFragment, mockConn) // This should trigger append, then fail unmarshal, then check size

	foundLogTooLarge := false
	for _, log := range observedLogs.All() { // Re-check logs from the beginning or get new observer
		if strings.Contains(log.Message, expectedLogMessageTooLarge) {
			foundLogTooLarge = true
			assert.Equal(t, int64(len(largeFragment)), log.ContextMap()["stored_data_size"])
			assert.Equal(t, int64(maxStoredDataSize), log.ContextMap()["max_size"])
			break
		}
	}
	assert.True(t, foundLogTooLarge, "Expected log message for too large fragment not found: %s", expectedLogMessageTooLarge)
	assert.Empty(t, s.storedData, "s.storedData should be empty after a too-large fragment caused clearing.")

}

// TestOnMessage_StoredData_ClearedOnSuccessfulUnmarshal tests that s.storedData is cleared
// after its content is successfully unmarshalled with new incoming data.
func TestOnMessage_StoredData_ClearedOnSuccessfulUnmarshal(t *testing.T) {
	s := newTestService(t, nil)
	mockConn := &MockWsConnection{}

	// Valid JSON object that will be sent in two parts
	fullMessage := model.GenericResult{
		Request: model.Request{Service: "test"},
		ResultData: model.GenericReponse[model.GenericUnit]{ // Using GenericUnit as a placeholder for ResultData content
			Service: "test_service",
		},
		ResultMessage: "success",
	}
	fullMessageBytes, err := json.Marshal(fullMessage)
	assert.NoError(t, err)

	// Split the message into two parts
	// Ensure part1 is an invalid JSON fragment (syntax error)
	part1 := fullMessageBytes[:len(fullMessageBytes)/2]
	// Ensure part1 is actually a syntax error - sometimes slicing valid JSON can result in valid (but different) JSON.
	// A common way is to cut mid-string or mid-number.
	// For this structure, cutting after a field name but before value is good.
	// Example: `{"request":{"service":"test"},"result_data":`
	var tempMap map[string]interface{}
	if json.Unmarshal(part1, &tempMap) == nil && !strings.HasSuffix(string(part1), ",") && !strings.HasSuffix(string(part1), ":") {
		// If part1 is somehow valid and not ending with typical partial markers, adjust it.
		// This is a bit heuristic. A more robust way is to find a guaranteed syntax error point.
		// For `{"request":{"service":"test"},"result_data":{"service":"test_service"},"result_message":"success"}`
		// Let's cut it at `{"request":{"service":"test"},"result_da`
		idx := strings.Index(string(fullMessageBytes), `"result_data"`)
		if idx > 0 {
			part1 = fullMessageBytes[:idx+len(`"result_da`)] // Guaranteed syntax error
		} else {
			t.Log("Could not reliably create a syntax error for part1, test might be less effective.")
		}
	}

	part2 := fullMessageBytes[len(part1):]

	// 1. Send the first part (should be stored in s.storedData)
	s.onMessage(part1, mockConn)
	assert.NotEmpty(t, s.storedData, "s.storedData should not be empty after receiving the first part.")
	assert.Equal(t, part1, s.storedData, "s.storedData should contain part1.")

	// 2. Send the second part
	s.onMessage(part2, mockConn)

	// Assertions
	assert.Empty(t, s.storedData, "s.storedData should be empty after successful unmarshal of combined data.")
}


// পেতেObserver is a helper to create a zap.Core that records logs.
func পেতেObserver(level zapcore.LevelEnabler) (zapcore.Core, *zaptest.Observer) {
	core, observer := zaptest.NewTestingObservatory(level)
	return core, observer
}

// TestOnMessage_NoBufferingForCompleteValidJSON tests that if a complete,
// valid JSON message arrives, it's processed directly without interacting with s.storedData
// if s.storedData is empty.
func TestOnMessage_NoBufferingForCompleteValidJSON(t *testing.T) {
	s := newTestService(t, nil)
	mockConn := &MockWsConnection{}

	validMessage := model.GenericResult{
		Request: model.Request{Service: "valid_test"},
		ResultData: model.GenericReponse[model.GenericUnit]{
			Service: "valid_service",
		},
		ResultMessage: "ok",
	}
	validMessageBytes, err := json.Marshal(validMessage)
	assert.NoError(t, err)

	// Ensure s.storedData is initially empty
	s.storedData = []byte{}

	// Call onMessage with the complete valid message
	s.onMessage(validMessageBytes, mockConn)

	// Assertions
	assert.Empty(t, s.storedData, "s.storedData should remain empty as the message was processed directly.")
	// Further assertions could be made if onMessage had side effects we could observe,
	// like sending a message via mockConn or changing other state.
	// For this test, we mainly care that s.storedData wasn't used.
}

// TestOnMessage_StoredDataNotClearedIfNewMessageIsValidAndBufferHasUnrelatedData
// This tests a specific behavior of the current onMessage logic:
// If s.storedData has some old partial data, and a new, complete, valid message arrives,
// the new message is processed, but the old s.storedData is NOT cleared.
func TestOnMessage_StoredDataNotClearedIfNewMessageIsValidAndBufferHasUnrelatedData(t *testing.T) {
	s := newTestService(t, nil)
	mockConn := &MockWsConnection{}

	// Pre-fill s.storedData with some unrelated partial data
	unrelatedPartialData := []byte(`{"partial":"data"`) // Syntax error
	s.storedData = append(s.storedData, unrelatedPartialData...)

	// A new, complete, and valid message
	validMessage := model.GenericResult{
		Request: model.Request{Service: "new_valid_test"},
		ResultData: model.GenericReponse[model.GenericUnit]{
			Service: "new_valid_service",
		},
		ResultMessage: "ok_new",
	}
	validMessageBytes, err := json.Marshal(validMessage)
	assert.NoError(t, err)

	// Call onMessage with the new complete valid message
	s.onMessage(validMessageBytes, mockConn)

	// Assertions
	assert.Equal(t, unrelatedPartialData, s.storedData, "s.storedData containing unrelated partial data should NOT be cleared when a new, complete message is processed.")
}
