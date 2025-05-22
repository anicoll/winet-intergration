package cmd

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/anicoll/winet-integration/internal/pkg/config"
	"github.com/anicoll/winet-integration/internal/pkg/database"
	"github.com/anicoll/winet-integration/internal/pkg/winet" // For winet.ErrConnect, winet.ErrTimeout
	"go.uber.org/zap/zaptest"
)

// TestRun_ErrConnect tests that run() returns ErrConnect when winetSvc.Connect fails.
func TestRun_ErrConnect(t *testing.T) {
	t.Parallel()
	logger := zaptest.NewLogger(t)
	cfg := &config.Config{} // Minimal config for this test

	mockWinetSvc := &MockWinetService{
		ConnectFunc: func(ctx context.Context) error {
			return winet.ErrConnect // Simulate connection error
		},
		SubscribeToTimeoutFunc: func() chan error {
			// Return a channel that will block, as Connect should fail first
			return make(chan error)
		},
	}

	// Use a cancellable context to ensure the test terminates even if run hangs
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	errorChan := make(chan error) // Unused directly in this error path, but needed by run
	defer close(errorChan)

	// For this test, we are only interested in the winetSvc.Connect error,
	// so db can be nil as cron jobs and http server won't be the source of the error.
	// However, run() will try to start them, so we need to make sure they don't cause panics.
	// The refactored run checks for nil db for cron jobs.
	// The HTTP server part also needs winetSvc and db.
	// We'll pass nil for db, and the HTTP server setup will be skipped in run.
	// The Connect error should be returned before other goroutines fully initialize or cause issues.

	err := run(ctx, cfg, mockWinetSvc, errorChan, logger, nil)

	if !errors.Is(err, winet.ErrConnect) {
		t.Errorf("expected error %v, got %v", winet.ErrConnect, err)
	}
}

// TestRun_ErrTimeout tests that run() returns ErrTimeout when SubscribeToTimeout sends it.
func TestRun_ErrTimeout(t *testing.T) {
	t.Parallel()
	logger := zaptest.NewLogger(t)
	cfg := &config.Config{}

	timeoutChan := make(chan error, 1) // Buffered to prevent send block if SubscribeToTimeout is called multiple times
	mockWinetSvc := &MockWinetService{
		ConnectFunc: func(ctx context.Context) error {
			return nil // Connect succeeds
		},
		SubscribeToTimeoutFunc: func() chan error {
			return timeoutChan
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	errorChan := make(chan error)
	defer close(errorChan)

	var wg sync.WaitGroup
	wg.Add(1) // For the main run function

	go func() {
		defer wg.Done()
		// db can be nil as per reasoning in TestRun_ErrConnect
		err := run(ctx, cfg, mockWinetSvc, errorChan, logger, nil)
		if !errors.Is(err, winet.ErrTimeout) {
			// Use t.Errorf for goroutines as it's thread-safe and marks test as failed.
			// Direct t.Error or t.Fatal can cause issues from non-test goroutines.
			t.Errorf("expected error %v from run(), got %v", winet.ErrTimeout, err)
		}
	}()

	// Send the timeout error after a short delay to ensure run() has started the goroutine
	time.Sleep(100 * time.Millisecond)
	timeoutChan <- winet.ErrTimeout
	close(timeoutChan) // Close to unblock SubscribeToTimeout if it's called again

	wg.Wait() // Wait for the run goroutine to complete its assertions
}

// TestRun_ContextCancellation tests that run() exits gracefully when the context is cancelled.
func TestRun_ContextCancellation(t *testing.T) {
	t.Parallel()
	logger := zaptest.NewLogger(t)
	cfg := &config.Config{}

	blockingChan := make(chan error) // Will block indefinitely
	// defer close(blockingChan) // Not closing as it's meant to block

	mockWinetSvc := &MockWinetService{
		ConnectFunc: func(ctx context.Context) error {
			// Simulate a successful connection that might block or take time before cancellation
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(2 * time.Second): // Simulate some work
				return nil
			}
		},
		SubscribeToTimeoutFunc: func() chan error {
			return blockingChan // This will block the winet goroutine
		},
	}

	// Create a context that we can cancel
	ctx, cancel := context.WithCancel(context.Background())

	errorChan := make(chan error)
	defer close(errorChan)

	var runErr error
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// db can be nil
		runErr = run(ctx, cfg, mockWinetSvc, errorChan, logger, nil)
	}()

	// Let run start and potentially enter blocking calls
	time.Sleep(100 * time.Millisecond)
	cancel() // Cancel the context

	wg.Wait() // Wait for run to exit

	// Check the error returned by run()
	if !errors.Is(runErr, context.Canceled) && !errors.Is(runErr, context.DeadlineExceeded) {
		// It might also be DeadlineExceeded if the ConnectFunc's time.After hits before select on ctx.Done
		// Or if the overall test context (if one was set with t.Deadline) is exceeded.
		// For this test, we primarily expect context.Canceled.
		t.Errorf("expected context.Canceled or context.DeadlineExceeded, got %v", runErr)
	}
}

// TestRun_NilDatabaseNoPanic tests that run() doesn't panic if db is nil (cron jobs should be skipped).
func TestRun_NilDatabaseNoPanic(t *testing.T) {
	t.Parallel()
	logger := zaptest.NewLogger(t)
	cfg := &config.Config{}

	mockWinetSvc := &MockWinetService{
		ConnectFunc: func(ctx context.Context) error {
			// Fail connect quickly to terminate the run function for this test.
			return winet.ErrConnect
		},
		SubscribeToTimeoutFunc: func() chan error {
			return make(chan error)
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	errorChan := make(chan error)
	defer close(errorChan)

	// Call run with nil database
	// We expect it to return ErrConnect from our mock setup.
	// The key is that it shouldn't panic due to nil db when setting up cron jobs.
	err := run(ctx, cfg, mockWinetSvc, errorChan, logger, nil) // db is nil

	if !errors.Is(err, winet.ErrConnect) {
		t.Errorf("expected ErrConnect, got %v (or panic if nil db not handled)", err)
	}
	// If it reaches here without panicking, the nil db handling for cron setup is implicitly tested.
}


// Note: Testing the HTTP server part of run() would require more setup (e.g., mock http.Server or use httptest).
// Testing cron job execution logic (errors sent to errorChan) would also require more intricate setup,
// potentially by making cron setup functions allow mock cron instances, or by directly calling the cron functions.
// The current tests focus on the error propagation from WinetService and context cancellation.

// MockDatabase can be defined if we need to test interactions with the database
// For now, we are passing nil for Database to test specific paths in run.
type MockDatabase struct{}

func (m *MockDatabase) Cleanup(ctx context.Context) error { return nil }
func (m *MockDatabase) WriteAmberPrices(ctx context.Context, prices []amber.Price) error { return nil }
// Add other database.Database methods if needed by tests
// For example, if server.New(winetSvc, db) is called with a non-nil db in some tests.
func (m *MockDatabase) GetNonPricingSettings(ctx context.Context) ([]database.Settings, error) { return nil, nil }
func (m *MockDatabase) GetPricingSettings(ctx context.Context) ([]database.Settings, error) { return nil, nil }
func (m *MockDatabase) UpdateSetting(ctx context.Context, key string, value string) error { return nil }
func (m *MockDatabase) WriteDeviceStatus(ctx context.Context, deviceStatuses map[model.Device][]model.DeviceStatus) error { return nil }
func (m *MockDatabase) WriteProperties(ctx context.Context, properties map[string]string) error { return nil }
func (m *MockDatabase) GetProperties(ctx context.Context) (map[string]string, error) { return nil, nil }
func (m *MockDatabase) RegisterDevice(ctx context.Context, device *model.Device) error { return nil }


// Ensure MockDatabase implements the interface expected by server.New if we were to use it.
// The actual interface is implicit. Based on server.go, it uses:
// GetNonPricingSettings, GetPricingSettings, UpdateSetting.
// For these tests, we are passing nil for database, so these are not strictly needed yet.
// var _ database.Service = &MockDatabase{} // Assuming database.Service is the interface.
// The actual interface used by server.New is internal/pkg/server.databaseService
// For now, these tests pass nil for the database when testing non-HTTP/non-DB parts of run.
// If we were to test the HTTP server part of run, we'd need a mock DB that implements
// the methods used by server.New's databaseService.
// For now, this MockDatabase is a placeholder.
// The existing tests pass `nil` for the database argument to `run`,
// and `run` has checks to skip cron/HTTP server setup if `db` is `nil`.
// This means we don't need a functional MockDatabase for the current tests.
// If future tests need to exercise the HTTP server or cron jobs with a non-nil DB,
// then MockDatabase would need to implement the methods used by those parts.
// The methods server.New calls on `db` are: GetNonPricingSettings, GetPricingSettings, UpdateSetting.
// So, MockDatabase would need to implement those if used with server.New.
// However, the current tests for `run` are focused on `winetSvc` error propagation
// and skip the parts of `run` that deeply involve `db` by passing `nil`.The `cmd/cmd_test.go` file has been created with unit tests for `ErrConnect`, `ErrTimeout`, and context cancellation scenarios within the refactored `run` function. A `TestRun_NilDatabaseNoPanic` test was also added to ensure that passing a `nil` database (as done in the other tests to isolate `winetSvc` errors) doesn't cause panics when `run` attempts to set up cron jobs.

The tests use the `MockWinetService` and `zaptest.NewLogger`. They focus on the error propagation paths related to `winetSvc` and context management in `run`.

Next steps in the plan:
**2. Unit tests for `s.storedData` in `internal/pkg/winet/winet_test.go`:**
   - If `winet_test.go` doesn't exist, create it.
   - Get a `winet.Service` instance (actually, a `*service` instance, as `onMessage` is a method on the concrete type).
   - **Test Case 1: `s.storedData` cleared on exceeding max size:**
     - Define `maxStoredDataSize` (use the same value as in `winet.go`).
     - Create mock WebSocket connection and logger.
     - Directly call `onMessage` multiple times with partial JSON data.
     - Ensure that when `len(s.storedData) + len(newData)` exceeds `maxStoredDataSize`, `s.storedData` is cleared, and a warning is logged.
     - Then append the `newData`. Check `s.storedData` content.
   - **Test Case 2: `s.storedData` cleared after successful unmarshal:**
     - Call `onMessage` with initial partial data.
     - Call `onMessage` again with the remaining part, making the combined data valid JSON.
     - Assert that `s.storedData` is empty after the second call.

Let's create `internal/pkg/winet/winet_test.go`. I'll need a mock WebSocket connection as `onMessage` takes `ws.Connection`.
