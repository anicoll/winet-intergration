// Run with: go test -v ./internal/pkg/store/postgres/...
//
// A PostgreSQL container is started automatically via testcontainers.
package postgres_test

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/suite"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/anicoll/winet-integration/internal/pkg/database/migration"
	"github.com/anicoll/winet-integration/internal/pkg/model"
	"github.com/anicoll/winet-integration/internal/pkg/publisher"
	"github.com/anicoll/winet-integration/internal/pkg/store"
	pgstore "github.com/anicoll/winet-integration/internal/pkg/store/postgres"
)

// migrationsPath resolves the postgres migrations directory relative to this
// test file, regardless of where `go test` is invoked from.
func migrationsPath() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "../../../../migrations/postgres")
}

// PostgresSuite starts a single container for the entire suite.
type PostgresSuite struct {
	suite.Suite
	container *postgres.PostgresContainer
	store     *pgstore.Store
}

func TestPostgresSuite(t *testing.T) {
	suite.Run(t, new(PostgresSuite))
}

func (s *PostgresSuite) SetupSuite() {
	ctx := context.Background()

	ctr, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("winet_test"),
		postgres.WithUsername("postgres"),
		postgres.WithPassword("postgres"),
		postgres.BasicWaitStrategies(),
	)
	s.Require().NoError(err)
	s.container = ctr

	dsn, err := ctr.ConnectionString(ctx, "sslmode=disable")
	s.Require().NoError(err)

	pool, err := pgxpool.New(ctx, dsn)
	s.Require().NoError(err)
	s.Require().NoError(pool.Ping(ctx))

	s.Require().NoError(migration.MigratePostgres(pool, migrationsPath()))

	s.store = pgstore.New(pool)
}

func (s *PostgresSuite) TearDownSuite() {
	if s.container != nil {
		_ = s.container.Terminate(context.Background())
	}
}

func (s *PostgresSuite) TestWrite_and_GetLatestProperties() {
	ctx := context.Background()

	dp := publisher.DataPoint{
		Timestamp:         time.Now().UTC().Truncate(time.Millisecond),
		UnitOfMeasurement: "W",
		Value:             "42.5",
		Identifier:        "SN-TEST-001",
		Slug:              "battery-power",
	}

	s.Require().NoError(s.store.Write(ctx, []publisher.DataPoint{dp}))

	props, err := s.store.GetLatestProperties(ctx)
	s.Require().NoError(err)

	found := false
	for p := range props {
		if p.Identifier == dp.Identifier && p.Slug == dp.Slug {
			s.Equal(dp.Value, p.Value)
			found = true
		}
	}
	s.True(found, "written data point not found in GetLatestProperties")
}

func (s *PostgresSuite) TestRegisterDevice() {
	ctx := context.Background()

	dev := &model.Device{ID: "pg-integration-device", Model: "TestModel", SerialNumber: "SN-IT-001"}
	s.Require().NoError(s.store.RegisterDevice(ctx, dev))
	s.Require().NoError(s.store.RegisterDevice(ctx, dev), "second upsert must be idempotent")
}

func (s *PostgresSuite) TestWriteAmberPrices_and_GetAmberPrices() {
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

	s.Require().NoError(s.store.WriteAmberPrices(ctx, prices))

	got, err := s.store.GetAmberPrices(ctx, now.Add(-time.Minute), now.Add(time.Hour), nil)
	s.Require().NoError(err)
	s.Require().NotEmpty(got)
	s.Equal(prices[0].PerKwh, got[0].PerKwh)
}

func (s *PostgresSuite) TestWriteAmberUsage_and_GetAmberUsage() {
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

	s.Require().NoError(s.store.WriteAmberUsage(ctx, usage))

	got, err := s.store.GetAmberUsage(ctx, now.Add(-time.Minute), now.Add(time.Hour))
	s.Require().NoError(err)
	s.Require().NotEmpty(got)
	s.InDelta(usage[0].Kwh, got[0].Kwh, 0.001)
}

func (s *PostgresSuite) TestUserAndRefreshToken() {
	ctx := context.Background()

	user, err := s.store.CreateUser(ctx, "pg-integration-user", "$2a$10$placeholder")
	s.Require().NoError(err)
	s.Equal("pg-integration-user", user.Username)

	fetched, err := s.store.GetUserByUsername(ctx, user.Username)
	s.Require().NoError(err)
	s.Equal(user.ID, fetched.ID)

	params := store.StoreRefreshTokenParams{
		TokenHash: "pg-test-hash-integration",
		UserID:    user.ID,
		Username:  user.Username,
		ExpiresAt: time.Now().Add(time.Hour),
	}
	s.Require().NoError(s.store.StoreRefreshToken(ctx, params))

	tok, err := s.store.GetRefreshToken(ctx, params.TokenHash)
	s.Require().NoError(err)
	s.Equal(user.ID, tok.UserID)

	s.Require().NoError(s.store.DeleteRefreshToken(ctx, params.TokenHash))
	s.Require().NoError(s.store.DeleteExpiredRefreshTokens(ctx))
}
