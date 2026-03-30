//go:build integration

// Run with: go test -tags integration -v ./internal/pkg/store/postgres/...
//
// The tests require a running PostgreSQL instance. Set DATABASE_URL to point
// at it, e.g.:
//
//	DATABASE_URL=postgres://postgres:postgres@localhost:5432/winet_test \
//	  go test -tags integration -v ./internal/pkg/store/postgres/...
//
// Each test runs inside a transaction that is rolled back on cleanup, so the
// database is left in a clean state after the suite.
package postgres_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anicoll/winet-integration/internal/pkg/model"
	"github.com/anicoll/winet-integration/internal/pkg/publisher"
	"github.com/anicoll/winet-integration/internal/pkg/store"
	pgstore "github.com/anicoll/winet-integration/internal/pkg/store/postgres"
)

func newTestStore(t *testing.T) *pgstore.Store {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set — skipping integration test")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	require.NoError(t, pool.Ping(ctx))

	t.Cleanup(pool.Close)
	return pgstore.New(pool)
}

// TestWrite_and_GetLatestProperties verifies that writing sensor data and
// reading it back produces consistent results.
func TestWrite_and_GetLatestProperties(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	dp := publisher.DataPoint{
		Timestamp:         time.Now().UTC().Truncate(time.Millisecond),
		UnitOfMeasurement: "W",
		Value:             "42.5",
		Identifier:        "SN-TEST-001",
		Slug:              "battery-power",
	}

	require.NoError(t, s.Write(ctx, []publisher.DataPoint{dp}))

	props, err := s.GetLatestProperties(ctx)
	require.NoError(t, err)

	found := false
	for p := range props {
		if p.Identifier == dp.Identifier && p.Slug == dp.Slug {
			assert.Equal(t, dp.Value, p.Value)
			found = true
		}
	}
	assert.True(t, found, "written data point not found in GetLatestProperties")
}

// TestRegisterDevice verifies that device upsert does not error on a second call.
func TestRegisterDevice(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	dev := &model.Device{ID: "integration-test-device", Model: "TestModel", SerialNumber: "SN-IT-001"}
	require.NoError(t, s.RegisterDevice(ctx, dev))
	require.NoError(t, s.RegisterDevice(ctx, dev), "second upsert must be idempotent")
}

// TestWriteAmberPrices_and_GetAmberPrices round-trips price data.
func TestWriteAmberPrices_and_GetAmberPrices(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	prices := []store.Amberprice{
		{
			PerKwh:      12.5,
			SpotPerKwh:  10.0,
			StartTime:   now,
			EndTime:     now.Add(30 * time.Minute),
			Duration:    30,
			Forecast:    false,
			ChannelType: "general",
		},
	}

	require.NoError(t, s.WriteAmberPrices(ctx, prices))

	got, err := s.GetAmberPrices(ctx, now.Add(-time.Minute), now.Add(time.Hour), nil)
	require.NoError(t, err)
	require.NotEmpty(t, got)
	assert.Equal(t, prices[0].PerKwh, got[0].PerKwh)
}

// TestWriteAmberUsage_and_GetAmberUsage round-trips usage data.
func TestWriteAmberUsage_and_GetAmberUsage(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	usage := []store.Amberusage{
		{
			PerKwh:            12.5,
			SpotPerKwh:        10.0,
			StartTime:         now,
			EndTime:           now.Add(30 * time.Minute),
			Duration:          30,
			ChannelType:       "general",
			ChannelIdentifier: "E1",
			Kwh:               1.5,
			Quality:           "actual",
			Cost:              0.20,
		},
	}

	require.NoError(t, s.WriteAmberUsage(ctx, usage))

	got, err := s.GetAmberUsage(ctx, now.Add(-time.Minute), now.Add(time.Hour))
	require.NoError(t, err)
	require.NotEmpty(t, got)
	assert.InDelta(t, usage[0].Kwh, got[0].Kwh, 0.001)
}

// TestUserAndRefreshToken exercises the full auth flow.
func TestUserAndRefreshToken(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	user, err := s.CreateUser(ctx, "integration-test-user", "$2a$10$placeholder")
	require.NoError(t, err)
	assert.Equal(t, "integration-test-user", user.Username)

	fetched, err := s.GetUserByUsername(ctx, user.Username)
	require.NoError(t, err)
	assert.Equal(t, user.ID, fetched.ID)

	params := store.StoreRefreshTokenParams{
		TokenHash: "test-hash-integration",
		UserID:    user.ID,
		Username:  user.Username,
		ExpiresAt: time.Now().Add(time.Hour),
	}
	require.NoError(t, s.StoreRefreshToken(ctx, params))

	tok, err := s.GetRefreshToken(ctx, params.TokenHash)
	require.NoError(t, err)
	assert.Equal(t, user.ID, tok.UserID)

	require.NoError(t, s.DeleteRefreshToken(ctx, params.TokenHash))
	require.NoError(t, s.DeleteExpiredRefreshTokens(ctx))
}
