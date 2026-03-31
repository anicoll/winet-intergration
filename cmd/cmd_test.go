package cmd

import (
	"context"
	"errors"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/anicoll/winet-integration/internal/pkg/store"
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

// --- fetchAndStoreUsage ---

func TestFetchAndStoreUsage_CallsCorrectSiteID(t *testing.T) {
	svc := cmdmocks.NewAmberUsageFetcher(t)
	db := cmdmocks.NewAmberUsageWriter(t)

	svc.EXPECT().GetUsage(mock.Anything, "site-xyz", mock.Anything, mock.Anything).Return(nil, nil)
	db.EXPECT().WriteAmberUsage(mock.Anything, mock.Anything).Return(nil)

	require.NoError(t, fetchAndStoreUsage(context.Background(), svc, db, "site-xyz"))
}

func TestFetchAndStoreUsage_DateRangeIsSevenDaysEndingYesterday(t *testing.T) {
	svc := cmdmocks.NewAmberUsageFetcher(t)
	db := cmdmocks.NewAmberUsageWriter(t)

	var capturedStart, capturedEnd openapi_types.Date

	svc.EXPECT().
		GetUsage(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Run(func(_ context.Context, _ string, start, end openapi_types.Date) {
			capturedStart = start
			capturedEnd = end
		}).
		Return(nil, nil)
	db.EXPECT().WriteAmberUsage(mock.Anything, mock.Anything).Return(nil)

	before := time.Now()
	require.NoError(t, fetchAndStoreUsage(context.Background(), svc, db, "site-xyz"))
	after := time.Now()

	// endDate should be yesterday relative to when the call was made
	assert.True(t, capturedEnd.After(before.AddDate(0, 0, -1).Add(-time.Second)))
	assert.True(t, capturedEnd.Before(after.AddDate(0, 0, -1).Add(time.Second)))

	// startDate = now-7d, endDate = now-1d → 6-day window
	diff := capturedEnd.Sub(capturedStart.Time)
	assert.Equal(t, 6*24*time.Hour, diff)
}

func TestFetchAndStoreUsage_WritesReturnedUsageToDatabase(t *testing.T) {
	now := time.Now().UTC()
	usage := []store.Amberusage{
		{ID: 1, ChannelIdentifier: "E1", Kwh: 1.5, StartTime: now, EndTime: now.Add(30 * time.Minute)},
		{ID: 2, ChannelIdentifier: "B2", Kwh: -0.3, StartTime: now, EndTime: now.Add(30 * time.Minute)},
	}

	svc := cmdmocks.NewAmberUsageFetcher(t)
	db := cmdmocks.NewAmberUsageWriter(t)

	svc.EXPECT().GetUsage(mock.Anything, "site-xyz", mock.Anything, mock.Anything).Return(usage, nil)
	db.EXPECT().WriteAmberUsage(mock.Anything, usage).Return(nil)

	require.NoError(t, fetchAndStoreUsage(context.Background(), svc, db, "site-xyz"))
}

func TestFetchAndStoreUsage_ServiceError_PropagatesError(t *testing.T) {
	svc := cmdmocks.NewAmberUsageFetcher(t)
	db := cmdmocks.NewAmberUsageWriter(t)

	svc.EXPECT().GetUsage(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("amber API unavailable"))

	err := fetchAndStoreUsage(context.Background(), svc, db, "site-xyz")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get usage")
}

func TestFetchAndStoreUsage_DatabaseWriteError_PropagatesError(t *testing.T) {
	svc := cmdmocks.NewAmberUsageFetcher(t)
	db := cmdmocks.NewAmberUsageWriter(t)

	svc.EXPECT().GetUsage(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return([]store.Amberusage{{ID: 1}}, nil)
	db.EXPECT().WriteAmberUsage(mock.Anything, mock.Anything).Return(errors.New("db write failed"))

	err := fetchAndStoreUsage(context.Background(), svc, db, "site-xyz")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to write usage to database")
}

func TestFetchAndStoreUsage_EmptyUsage_WritesEmptySlice(t *testing.T) {
	svc := cmdmocks.NewAmberUsageFetcher(t)
	db := cmdmocks.NewAmberUsageWriter(t)

	svc.EXPECT().GetUsage(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return([]store.Amberusage{}, nil)
	db.EXPECT().WriteAmberUsage(mock.Anything, []store.Amberusage{}).Return(nil)

	require.NoError(t, fetchAndStoreUsage(context.Background(), svc, db, "site-xyz"))
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
