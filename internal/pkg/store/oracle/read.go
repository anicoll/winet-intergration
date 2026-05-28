package oracle

import (
	"context"
	"database/sql"
	"iter"
	"slices"
	"time"

	"github.com/anicoll/winet-integration/internal/pkg/store"
)

func (s *Store) GetProperties(ctx context.Context, identifier, slug string, from, to *time.Time) ([]store.Property, error) {
	if from == nil || to == nil {
		t := time.Now().AddDate(0, 0, -2)
		from = &t
		now := time.Now()
		to = &now
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, time_stamp, unit_of_measurement, value, identifier, slug
		FROM Property
		WHERE identifier = :1 AND slug = :2 AND time_stamp BETWEEN :3 AND :4
		ORDER BY time_stamp DESC`,
		identifier, slug, *from, *to)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanProperties(rows)
}

// GetLatestProperties returns the most recent reading per identifier+slug.
// Oracle does not have DISTINCT ON; ROW_NUMBER() OVER (PARTITION BY ...) is used instead.
func (s *Store) GetLatestProperties(ctx context.Context) (iter.Seq[store.Property], error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, time_stamp, unit_of_measurement, value, identifier, slug
		FROM (
			SELECT id, time_stamp, unit_of_measurement, value, identifier, slug,
			       ROW_NUMBER() OVER (PARTITION BY identifier, slug ORDER BY time_stamp DESC) AS rn
			FROM Property
			WHERE time_stamp > SYSTIMESTAMP - INTERVAL '1' DAY
		)
		WHERE rn = 1`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	props, err := scanProperties(rows)
	if err != nil {
		return nil, err
	}
	return slices.Values(props), nil
}

func scanProperties(rows *sql.Rows) ([]store.Property, error) {
	var out []store.Property
	for rows.Next() {
		var p store.Property
		if err := rows.Scan(&p.ID, &p.TimeStamp, &p.UnitOfMeasurement, &p.Value, &p.Identifier, &p.Slug); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) GetUserByUsername(ctx context.Context, username string) (store.User, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, username, password_hash, created_at, updated_at
		FROM users
		WHERE username = :1
		FETCH FIRST 1 ROWS ONLY`,
		username)
	var u store.User
	err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.CreatedAt, &u.UpdatedAt)
	return u, err
}

func (s *Store) GetRefreshToken(ctx context.Context, tokenHash string) (store.RefreshToken, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT token_hash, user_id, username, expires_at, created_at
		FROM refresh_tokens
		WHERE token_hash = :1
		FETCH FIRST 1 ROWS ONLY`,
		tokenHash)
	var r store.RefreshToken
	err := row.Scan(&r.TokenHash, &r.UserID, &r.Username, &r.ExpiresAt, &r.CreatedAt)
	return r, err
}
