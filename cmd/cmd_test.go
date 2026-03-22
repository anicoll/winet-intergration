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

	dbpkg "github.com/anicoll/winet-integration/internal/pkg/database/db"
	cmdmocks "github.com/anicoll/winet-integration/mocks/cmd"
)

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
	usage := []dbpkg.Amberusage{
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
		Return([]dbpkg.Amberusage{{ID: 1}}, nil)
	db.EXPECT().WriteAmberUsage(mock.Anything, mock.Anything).Return(errors.New("db write failed"))

	err := fetchAndStoreUsage(context.Background(), svc, db, "site-xyz")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to write usage to database")
}

func TestFetchAndStoreUsage_EmptyUsage_WritesEmptySlice(t *testing.T) {
	svc := cmdmocks.NewAmberUsageFetcher(t)
	db := cmdmocks.NewAmberUsageWriter(t)

	svc.EXPECT().GetUsage(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return([]dbpkg.Amberusage{}, nil)
	db.EXPECT().WriteAmberUsage(mock.Anything, []dbpkg.Amberusage{}).Return(nil)

	require.NoError(t, fetchAndStoreUsage(context.Background(), svc, db, "site-xyz"))
}
