package cmd

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"

	"github.com/anicoll/winet-integration/internal/pkg/winet"
	cmdmocks "github.com/anicoll/winet-integration/mocks/cmd"
)

// makeEvents returns a buffered channel pre-loaded with the given events.
func makeEvents(events ...winet.SessionEvent) chan winet.SessionEvent {
	ch := make(chan winet.SessionEvent, len(events))
	for _, e := range events {
		ch <- e
	}
	return ch
}

// --- startWinetService ---

// TestStartWinetService_TimeoutTriggersReconnect verifies that a winet login-timeout
// event causes the service to reconnect (loop again) rather than return an error.
func TestStartWinetService_TimeoutTriggersReconnect(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events := makeEvents(winet.SessionEvent{Err: winet.ErrTimeout})

	svc := cmdmocks.NewWinetConnector(t)
	svc.EXPECT().Events().Return(events)
	// First Connect succeeds; second Connect cancels ctx to stop the loop.
	svc.EXPECT().Connect(mock.Anything).Return(nil).Once()
	svc.EXPECT().Connect(mock.Anything).RunAndReturn(func(ctx context.Context) error {
		cancel()
		return nil
	}).Once()

	err := startWinetService(ctx, svc, &healthState{}, zap.NewNop())

	assert.ErrorIs(t, err, context.Canceled)
}

// TestStartWinetService_ConnectionErrorTriggersReconnect verifies the core fix:
// a non-timeout connection error (e.g. "connection reset by peer") must cause a
// reconnect, not propagate as a fatal error that shuts down the whole app.
func TestStartWinetService_ConnectionErrorTriggersReconnect(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events := makeEvents(winet.SessionEvent{Err: errors.New("read tcp: connection reset by peer")})

	svc := cmdmocks.NewWinetConnector(t)
	svc.EXPECT().Events().Return(events)
	svc.EXPECT().Connect(mock.Anything).Return(nil).Once()
	svc.EXPECT().Connect(mock.Anything).RunAndReturn(func(ctx context.Context) error {
		cancel()
		return nil
	}).Once()

	err := startWinetService(ctx, svc, &healthState{}, zap.NewNop())

	assert.ErrorIs(t, err, context.Canceled)
}

// TestStartWinetService_ContextCancelledBeforeConnect_ExitsCleanly verifies that
// a pre-cancelled context causes an immediate clean exit without calling Connect.
func TestStartWinetService_ContextCancelledBeforeConnect_ExitsCleanly(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	svc := cmdmocks.NewWinetConnector(t)
	// No Connect or Events calls expected.

	err := startWinetService(ctx, svc, &healthState{}, zap.NewNop())

	assert.ErrorIs(t, err, context.Canceled)
}

// TestStartWinetService_ContextCancelledAfterConnect_ExitsCleanly verifies that
// cancelling the context while waiting for events causes a clean exit (not a hang).
func TestStartWinetService_ContextCancelledAfterConnect_ExitsCleanly(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	events := make(chan winet.SessionEvent) // never sends

	svc := cmdmocks.NewWinetConnector(t)
	svc.EXPECT().Events().Return(events)
	svc.EXPECT().Connect(mock.Anything).RunAndReturn(func(ctx context.Context) error {
		cancel() // cancel while the service is waiting on Events()
		return nil
	})

	err := startWinetService(ctx, svc, &healthState{}, zap.NewNop())

	assert.ErrorIs(t, err, context.Canceled)
}

// TestStartWinetService_ConnectError_RetriesUntilContextCancelled verifies that
// a Connect failure does not immediately return — the service backs off and retries.
// Context cancellation during the backoff wait terminates cleanly.
func TestStartWinetService_ConnectError_RetriesUntilContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	svc := cmdmocks.NewWinetConnector(t)
	svc.EXPECT().Connect(mock.Anything).RunAndReturn(func(ctx context.Context) error {
		cancel() // cancel immediately to skip the backoff sleep
		return errors.New("dial refused")
	})

	err := startWinetService(ctx, svc, &healthState{}, zap.NewNop())

	assert.ErrorIs(t, err, context.Canceled)
}

// TestStartWinetService_RepeatedCrashesAlwaysReconnect is the crashloop regression
// test: the winet device resets the connection 5 times in quick succession. The
// service must reconnect each time and NEVER return a winet error to the caller
// (which would cancel the errgroup and kill the HTTP server).
func TestStartWinetService_RepeatedCrashesAlwaysReconnect(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Five crash events: a mix of connection resets and timeouts.
	events := makeEvents(
		winet.SessionEvent{Err: errors.New("read tcp: connection reset by peer")},
		winet.SessionEvent{Err: winet.ErrTimeout},
		winet.SessionEvent{Err: errors.New("read tcp: connection reset by peer")},
		winet.SessionEvent{Err: errors.New("EOF")},
		winet.SessionEvent{Err: winet.ErrTimeout},
	)

	svc := cmdmocks.NewWinetConnector(t)
	svc.EXPECT().Events().Return(events)
	// First 5 Connects succeed; 6th cancels the context to stop the loop.
	svc.EXPECT().Connect(mock.Anything).Return(nil).Times(5)
	svc.EXPECT().Connect(mock.Anything).RunAndReturn(func(ctx context.Context) error {
		cancel()
		return nil
	}).Once()

	err := startWinetService(ctx, svc, &healthState{}, zap.NewNop())

	// Must exit with context.Canceled — NOT a winet error — so the errgroup
	// does not propagate a crash to the HTTP server or amber services.
	assert.ErrorIs(t, err, context.Canceled)
}
