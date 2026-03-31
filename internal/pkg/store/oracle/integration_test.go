// Run with: go test -v ./internal/pkg/store/oracle/...
//
// A free Oracle container (gvenzl/oracle-free) is started automatically via
// testcontainers. The container is shared across the entire suite.
//
// Oracle Free starts slowly (~60-90s on first pull). Subsequent runs reuse the
// cached image and start in ~20s.
package oracle_test

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	_ "github.com/sijms/go-ora/v2"
	"github.com/stretchr/testify/suite"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/anicoll/winet-integration/internal/pkg/database/migration"
	"github.com/anicoll/winet-integration/internal/pkg/model"
	"github.com/anicoll/winet-integration/internal/pkg/publisher"
	"github.com/anicoll/winet-integration/internal/pkg/store"
	orastore "github.com/anicoll/winet-integration/internal/pkg/store/oracle"
)

const (
	oracleImage    = "gvenzl/oracle-free:23-slim-faststart"
	oraclePassword = "TestPassword1"
	oracleService  = "FREEPDB1"
	oracleUser     = "system"
	oraclePort     = "1521/tcp"
)

// migrationsPath resolves the oracle migrations directory relative to this
// test file, regardless of where `go test` is invoked from.
func migrationsPath() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "../../../../migrations/oracle")
}

// OracleSuite starts a single Oracle Free container for the entire suite.
type OracleSuite struct {
	suite.Suite
	container testcontainers.Container
	store     *orastore.Store
}

func TestOracleSuite(t *testing.T) {
	t.Parallel()
	suite.Run(t, new(OracleSuite))
}

func (s *OracleSuite) SetupSuite() {
	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        oracleImage,
		ExposedPorts: []string{oraclePort},
		Env: map[string]string{
			"ORACLE_PASSWORD": oraclePassword,
		},
		WaitingFor: wait.ForLog("DATABASE IS READY TO USE!").
			WithStartupTimeout(5 * time.Minute),
	}

	ctr, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	s.Require().NoError(err)
	s.container = ctr

	host, err := ctr.Host(ctx)
	s.Require().NoError(err)
	mappedPort, err := ctr.MappedPort(ctx, oraclePort)
	s.Require().NoError(err)

	dsn := fmt.Sprintf("oracle://%s:%s@%s:%s/%s",
		oracleUser, oraclePassword, host, mappedPort.Port(), oracleService)

	db, err := sql.Open("oracle", dsn)
	s.Require().NoError(err)
	s.Require().NoError(db.PingContext(ctx))

	s.Require().NoError(migration.MigrateOracle(db, migrationsPath()))

	s.store = orastore.New(db)
}

func (s *OracleSuite) TearDownSuite() {
	if s.container != nil {
		_ = s.container.Terminate(context.Background())
	}
}

func (s *OracleSuite) TestWrite_and_GetLatestProperties() {
	ctx := context.Background()

	dp := publisher.DataPoint{
		Timestamp:         time.Now().UTC().Truncate(time.Second),
		UnitOfMeasurement: "W",
		Value:             "42.5",
		Identifier:        "SN-ORA-001",
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

func (s *OracleSuite) TestRegisterDevice() {
	ctx := context.Background()

	dev := &model.Device{ID: "oracle-integration-device", Model: "TestModel", SerialNumber: "SN-ORA-IT-001"}
	s.Require().NoError(s.store.RegisterDevice(ctx, dev))
	s.Require().NoError(s.store.RegisterDevice(ctx, dev), "second upsert must be idempotent")
}

func (s *OracleSuite) TestWriteAmberPrices_and_GetAmberPrices() {
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

func (s *OracleSuite) TestWriteAmberUsage_and_GetAmberUsage() {
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

func (s *OracleSuite) TestUserAndRefreshToken() {
	ctx := context.Background()

	user, err := s.store.CreateUser(ctx, "oracle-integration-user", "$2a$10$placeholder")
	s.Require().NoError(err)
	s.Equal("oracle-integration-user", user.Username)

	fetched, err := s.store.GetUserByUsername(ctx, user.Username)
	s.Require().NoError(err)
	s.Equal(user.ID, fetched.ID)

	params := store.StoreRefreshTokenParams{
		TokenHash: "ora-test-hash-integration",
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
