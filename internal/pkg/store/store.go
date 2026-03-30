package store

import (
	"context"
	"iter"
	"time"

	"github.com/anicoll/winet-integration/internal/pkg/model"
	"github.com/anicoll/winet-integration/internal/pkg/publisher"
)

// Store is the single interface that every database backend must satisfy.
// Add a new implementation in its own sub-package (e.g. store/postgres,
// store/mssql) and write an integration test there to prove behavioural
// equivalence.
type Store interface {
	// --- sensor data ---

	// Write persists a batch of normalised sensor readings.
	Write(ctx context.Context, data []publisher.DataPoint) error
	// RegisterDevice upserts a device record.
	RegisterDevice(ctx context.Context, device *model.Device) error
	// GetProperties returns readings for identifier/slug, defaulting to the
	// last 2 days when from/to are nil.
	GetProperties(ctx context.Context, identifier, slug string, from, to *time.Time) ([]Property, error)
	// GetLatestProperties returns the most recent reading per identifier+slug.
	GetLatestProperties(ctx context.Context) (iter.Seq[Property], error)
	// Cleanup removes sensor readings older than the implementation's
	// configured retention window.
	Cleanup(ctx context.Context) error

	// --- amber ---

	GetAmberPrices(ctx context.Context, from, to time.Time, site *string) ([]Amberprice, error)
	GetAmberUsage(ctx context.Context, from, to time.Time) ([]Amberusage, error)
	WriteAmberPrices(ctx context.Context, prices []Amberprice) error
	WriteAmberUsage(ctx context.Context, usage []Amberusage) error

	// --- auth ---

	GetUserByUsername(ctx context.Context, username string) (User, error)
	CreateUser(ctx context.Context, username, passwordHash string) (User, error)
	StoreRefreshToken(ctx context.Context, arg StoreRefreshTokenParams) error
	GetRefreshToken(ctx context.Context, tokenHash string) (RefreshToken, error)
	DeleteRefreshToken(ctx context.Context, tokenHash string) error
	DeleteExpiredRefreshTokens(ctx context.Context) error
}
