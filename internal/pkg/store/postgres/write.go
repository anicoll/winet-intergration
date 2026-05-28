package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	dbq "github.com/anicoll/winet-integration/internal/pkg/database/db"
	"github.com/anicoll/winet-integration/internal/pkg/model"
	"github.com/anicoll/winet-integration/internal/pkg/publisher"
	"github.com/anicoll/winet-integration/internal/pkg/store"
)

func (s *Store) Write(ctx context.Context, data []publisher.DataPoint) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.queries.WithTx(tx)
	for _, dp := range data {
		if _, err := qtx.InsertProperty(ctx, dbq.InsertPropertyParams{
			TimeStamp:         dp.Timestamp,
			UnitOfMeasurement: dp.UnitOfMeasurement,
			Value:             dp.Value,
			Identifier:        dp.Identifier,
			Slug:              dp.Slug,
		}); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) RegisterDevice(ctx context.Context, device *model.Device) error {
	return s.queries.UpsertDevice(ctx, dbq.UpsertDeviceParams{
		ID:           device.ID,
		Model:        pgtype.Text{String: device.Model, Valid: true},
		SerialNumber: pgtype.Text{String: device.SerialNumber, Valid: true},
	})
}

func (s *Store) CreateUser(ctx context.Context, username, passwordHash string) (store.User, error) {
	u, err := s.queries.CreateUser(ctx, dbq.CreateUserParams{
		Username:     username,
		PasswordHash: passwordHash,
	})
	if err != nil {
		return store.User{}, err
	}
	return store.User{
		ID:           u.ID,
		Username:     u.Username,
		PasswordHash: u.PasswordHash,
		CreatedAt:    u.CreatedAt.Time,
		UpdatedAt:    u.UpdatedAt.Time,
	}, nil
}

func (s *Store) StoreRefreshToken(ctx context.Context, arg store.StoreRefreshTokenParams) error {
	return s.queries.StoreRefreshToken(ctx, dbq.StoreRefreshTokenParams{
		TokenHash: arg.TokenHash,
		UserID:    arg.UserID,
		Username:  arg.Username,
		ExpiresAt: arg.ExpiresAt,
	})
}

func (s *Store) DeleteRefreshToken(ctx context.Context, tokenHash string) error {
	return s.queries.DeleteRefreshToken(ctx, tokenHash)
}

func (s *Store) DeleteExpiredRefreshTokens(ctx context.Context) error {
	return s.queries.DeleteExpiredRefreshTokens(ctx)
}

// toPgTimestamptz converts a time.Time to pgtype.Timestamptz.
// Kept here for completeness; use it if future queries need explicit casting.
func toPgTimestamptz(t time.Time) pgtype.Timestamptz { //nolint:unused
	return pgtype.Timestamptz{Time: t, Valid: true}
}
