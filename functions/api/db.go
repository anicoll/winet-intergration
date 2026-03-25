package main

import (
	"context"
	"database/sql"
	"iter"
	"slices"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	dbpkg "github.com/anicoll/winet-integration/internal/pkg/database/db"
)

// store implements server.Database, auth.UserStore, and auth.TokenStore
// using hand-written T-SQL queries against Azure SQL Database via go-mssqldb.
type store struct {
	db *sql.DB
}

func newStore(db *sql.DB) *store { return &store{db: db} }

// toTimestamptz converts a time.Time (from go-mssqldb scan) to pgtype.Timestamptz
// as required by the existing sqlc-generated model types.
func toTimestamptz(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

// --- server.Database ----------------------------------------------------------

const queryLatestProperties = `
SELECT id, time_stamp, unit_of_measurement, value, identifier, slug
FROM (
    SELECT id, time_stamp, unit_of_measurement, value, identifier, slug,
           ROW_NUMBER() OVER (PARTITION BY identifier, slug ORDER BY time_stamp DESC) AS rn
    FROM Property
    WHERE time_stamp > DATEADD(day, -1, SYSDATETIMEOFFSET())
) t
WHERE rn = 1`

func (s *store) GetLatestProperties(ctx context.Context) (iter.Seq[dbpkg.Property], error) {
	rows, err := s.db.QueryContext(ctx, queryLatestProperties)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var props []dbpkg.Property
	for rows.Next() {
		var p dbpkg.Property
		if err := rows.Scan(&p.ID, &p.TimeStamp, &p.UnitOfMeasurement, &p.Value, &p.Identifier, &p.Slug); err != nil {
			return nil, err
		}
		props = append(props, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return slices.Values(props), nil
}

const queryProperties = `
SELECT id, time_stamp, unit_of_measurement, value, identifier, slug
FROM Property
WHERE identifier = @p1
  AND slug       = @p2
  AND time_stamp BETWEEN @p3 AND @p4
ORDER BY time_stamp DESC`

func (s *store) GetProperties(ctx context.Context, identifier, slug string, from, to *time.Time) ([]dbpkg.Property, error) {
	if from == nil || to == nil {
		t := time.Now().AddDate(0, 0, -2)
		from = &t
		now := time.Now()
		to = &now
	}
	rows, err := s.db.QueryContext(ctx, queryProperties,
		sql.Named("p1", identifier),
		sql.Named("p2", slug),
		sql.Named("p3", *from),
		sql.Named("p4", *to),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var props []dbpkg.Property
	for rows.Next() {
		var p dbpkg.Property
		if err := rows.Scan(&p.ID, &p.TimeStamp, &p.UnitOfMeasurement, &p.Value, &p.Identifier, &p.Slug); err != nil {
			return nil, err
		}
		props = append(props, p)
	}
	return props, rows.Err()
}

const queryAmberPrices = `
SELECT id, per_kwh, spot_per_kwh, start_time, end_time, duration, forecast,
       channel_type, created_at, updated_at
FROM AmberPrice
WHERE start_time BETWEEN @p1 AND @p2
ORDER BY start_time DESC`

func (s *store) GetAmberPrices(ctx context.Context, from, to time.Time, _ *string) ([]dbpkg.Amberprice, error) {
	rows, err := s.db.QueryContext(ctx, queryAmberPrices,
		sql.Named("p1", from),
		sql.Named("p2", to),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var prices []dbpkg.Amberprice
	for rows.Next() {
		var p dbpkg.Amberprice
		var createdAt, updatedAt time.Time
		if err := rows.Scan(
			&p.ID, &p.PerKwh, &p.SpotPerKwh, &p.StartTime, &p.EndTime,
			&p.Duration, &p.Forecast, &p.ChannelType,
			&createdAt, &updatedAt,
		); err != nil {
			return nil, err
		}
		p.CreatedAt = toTimestamptz(createdAt)
		p.UpdatedAt = toTimestamptz(updatedAt)
		prices = append(prices, p)
	}
	return prices, rows.Err()
}

const queryAmberUsage = `
SELECT id, per_kwh, spot_per_kwh, start_time, end_time, duration,
       channel_type, channel_identifier, kwh, quality, cost, created_at, updated_at
FROM AmberUsage
WHERE start_time BETWEEN @p1 AND @p2
ORDER BY start_time DESC`

func (s *store) GetAmberUsage(ctx context.Context, from, to time.Time) ([]dbpkg.Amberusage, error) {
	rows, err := s.db.QueryContext(ctx, queryAmberUsage,
		sql.Named("p1", from),
		sql.Named("p2", to),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var usage []dbpkg.Amberusage
	for rows.Next() {
		var u dbpkg.Amberusage
		var createdAt, updatedAt time.Time
		if err := rows.Scan(
			&u.ID, &u.PerKwh, &u.SpotPerKwh, &u.StartTime, &u.EndTime,
			&u.Duration, &u.ChannelType, &u.ChannelIdentifier,
			&u.Kwh, &u.Quality, &u.Cost,
			&createdAt, &updatedAt,
		); err != nil {
			return nil, err
		}
		u.CreatedAt = toTimestamptz(createdAt)
		u.UpdatedAt = toTimestamptz(updatedAt)
		usage = append(usage, u)
	}
	return usage, rows.Err()
}

// --- auth.UserStore -----------------------------------------------------------

const queryGetUserByUsername = `
SELECT TOP 1 id, username, password_hash, created_at, updated_at
FROM users
WHERE username = @p1`

func (s *store) GetUserByUsername(ctx context.Context, username string) (dbpkg.User, error) {
	row := s.db.QueryRowContext(ctx, queryGetUserByUsername, sql.Named("p1", username))
	var u dbpkg.User
	var createdAt, updatedAt time.Time
	if err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &createdAt, &updatedAt); err != nil {
		return dbpkg.User{}, err
	}
	u.CreatedAt = toTimestamptz(createdAt)
	u.UpdatedAt = toTimestamptz(updatedAt)
	return u, nil
}

// --- auth.TokenStore ----------------------------------------------------------

const queryStoreRefreshToken = `
MERGE refresh_tokens AS target
USING (
    SELECT @p1 AS token_hash, @p2 AS user_id, @p3 AS username, @p4 AS expires_at
) AS source
ON target.token_hash = source.token_hash
WHEN MATCHED THEN
    UPDATE SET expires_at = source.expires_at
WHEN NOT MATCHED THEN
    INSERT (token_hash, user_id, username, expires_at)
    VALUES (source.token_hash, source.user_id, source.username, source.expires_at);`

func (s *store) StoreRefreshToken(ctx context.Context, arg dbpkg.StoreRefreshTokenParams) error {
	_, err := s.db.ExecContext(ctx, queryStoreRefreshToken,
		sql.Named("p1", arg.TokenHash),
		sql.Named("p2", arg.UserID),
		sql.Named("p3", arg.Username),
		sql.Named("p4", arg.ExpiresAt),
	)
	return err
}

const queryGetRefreshToken = `
SELECT TOP 1 token_hash, user_id, username, expires_at, created_at
FROM refresh_tokens
WHERE token_hash = @p1`

func (s *store) GetRefreshToken(ctx context.Context, tokenHash string) (dbpkg.RefreshToken, error) {
	row := s.db.QueryRowContext(ctx, queryGetRefreshToken, sql.Named("p1", tokenHash))
	var t dbpkg.RefreshToken
	var createdAt time.Time
	if err := row.Scan(&t.TokenHash, &t.UserID, &t.Username, &t.ExpiresAt, &createdAt); err != nil {
		return dbpkg.RefreshToken{}, err
	}
	t.CreatedAt = toTimestamptz(createdAt)
	return t, nil
}

const queryDeleteRefreshToken = `
DELETE FROM refresh_tokens WHERE token_hash = @p1`

func (s *store) DeleteRefreshToken(ctx context.Context, tokenHash string) error {
	_, err := s.db.ExecContext(ctx, queryDeleteRefreshToken, sql.Named("p1", tokenHash))
	return err
}

const queryDeleteExpiredRefreshTokens = `
DELETE FROM refresh_tokens WHERE expires_at < SYSDATETIMEOFFSET()`

func (s *store) DeleteExpiredRefreshTokens(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, queryDeleteExpiredRefreshTokens)
	return err
}
