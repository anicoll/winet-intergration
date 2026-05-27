package postgres

import (
	"context"
	"iter"
	"slices"
	"time"

	dbq "github.com/anicoll/winet-integration/internal/pkg/database/db"
	"github.com/anicoll/winet-integration/internal/pkg/store"
)

func (s *Store) GetProperties(ctx context.Context, identifier, slug string, from, to *time.Time) ([]store.Property, error) {
	if from == nil || to == nil {
		t := time.Now().AddDate(0, 0, -2)
		from = &t
		now := time.Now()
		to = &now
	}
	rows, err := s.queries.GetProperties(ctx, dbq.GetPropertiesParams{
		Identifier:  identifier,
		Slug:        slug,
		TimeStamp:   *from,
		TimeStamp_2: *to,
	})
	if err != nil {
		return nil, err
	}
	return toProperties(rows), nil
}

func (s *Store) GetLatestProperties(ctx context.Context) (iter.Seq[store.Property], error) {
	rows, err := s.queries.GetLatestProperties(ctx)
	if err != nil {
		return nil, err
	}
	return slices.Values(toProperties(rows)), nil
}



func (s *Store) GetUserByUsername(ctx context.Context, username string) (store.User, error) {
	u, err := s.queries.GetUserByUsername(ctx, username)
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

func (s *Store) GetRefreshToken(ctx context.Context, tokenHash string) (store.RefreshToken, error) {
	r, err := s.queries.GetRefreshToken(ctx, tokenHash)
	if err != nil {
		return store.RefreshToken{}, err
	}
	return store.RefreshToken{
		TokenHash: r.TokenHash,
		UserID:    r.UserID,
		Username:  r.Username,
		ExpiresAt: r.ExpiresAt,
		CreatedAt: r.CreatedAt.Time,
	}, nil
}

// toProperties converts sqlc Property rows (all standard types) to store.Property.
func toProperties(rows []dbq.Property) []store.Property {
	out := make([]store.Property, len(rows))
	for i, r := range rows {
		out[i] = store.Property{
			ID:                r.ID,
			TimeStamp:         r.TimeStamp,
			UnitOfMeasurement: r.UnitOfMeasurement,
			Value:             r.Value,
			Identifier:        r.Identifier,
			Slug:              r.Slug,
		}
	}
	return out
}
