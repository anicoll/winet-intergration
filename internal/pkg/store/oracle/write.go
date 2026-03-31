package oracle

import (
	"context"
	"database/sql"
	"time"

	"github.com/anicoll/winet-integration/internal/pkg/model"
	"github.com/anicoll/winet-integration/internal/pkg/publisher"
	"github.com/anicoll/winet-integration/internal/pkg/store"
)

func (s *Store) Write(ctx context.Context, data []publisher.DataPoint) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO Property (time_stamp, unit_of_measurement, value, identifier, slug)
		VALUES (:time_stamp, NVL(:unit_of_measurement, '-'), :value, :identifier, :slug)`)
	if err != nil {
		return err
	}
	defer func() { _ = stmt.Close() }()

	for _, dp := range data {
		if _, err := stmt.ExecContext(ctx,
			sql.Named("time_stamp", dp.Timestamp),
			sql.Named("unit_of_measurement", dp.UnitOfMeasurement),
			sql.Named("value", dp.Value),
			sql.Named("identifier", dp.Identifier),
			sql.Named("slug", dp.Slug),
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) RegisterDevice(ctx context.Context, device *model.Device) error {
	_, err := s.db.ExecContext(ctx, `
		MERGE INTO Device d
		USING (SELECT :id AS id FROM dual) src
		ON (d.id = src.id)
		WHEN NOT MATCHED THEN
			INSERT (id, model, serial_number) VALUES (:id, :model, :serial_number)`,
		sql.Named("id", device.ID),
		sql.Named("model", device.Model),
		sql.Named("serial_number", device.SerialNumber),
	)
	return err
}

func (s *Store) WriteAmberPrices(ctx context.Context, prices []store.Amberprice) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx, `
		MERGE INTO AmberPrice ap
		USING (SELECT :start_time AS start_time, :channel_type AS channel_type FROM dual) src
		ON (ap.start_time = src.start_time AND ap.channel_type = src.channel_type)
		WHEN MATCHED THEN
			UPDATE SET per_kwh = :per_kwh, spot_per_kwh = :spot_per_kwh,
			           duration = :duration, forecast = :forecast, updated_at = SYSTIMESTAMP
		WHEN NOT MATCHED THEN
			INSERT (per_kwh, spot_per_kwh, start_time, end_time, duration, forecast, channel_type)
			VALUES (:per_kwh, :spot_per_kwh, :start_time, :end_time, :duration, :forecast, :channel_type)`)
	if err != nil {
		return err
	}
	defer func() { _ = stmt.Close() }()

	for _, p := range prices {
		forecast := 0
		if p.Forecast {
			forecast = 1
		}
		if _, err := stmt.ExecContext(ctx,
			sql.Named("start_time", p.StartTime),
			sql.Named("channel_type", p.ChannelType),
			sql.Named("per_kwh", p.PerKwh),
			sql.Named("spot_per_kwh", p.SpotPerKwh),
			sql.Named("duration", p.Duration),
			sql.Named("forecast", forecast),
			sql.Named("end_time", p.EndTime),
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) WriteAmberUsage(ctx context.Context, usage []store.Amberusage) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx, `
		MERGE INTO AmberUsage au
		USING (SELECT :start_time AS start_time, :channel_identifier AS channel_identifier FROM dual) src
		ON (au.start_time = src.start_time AND au.channel_identifier = src.channel_identifier)
		WHEN MATCHED THEN
			UPDATE SET per_kwh = :per_kwh, spot_per_kwh = :spot_per_kwh, duration = :duration,
			           kwh = :kwh, quality = :quality, cost = :cost, updated_at = SYSTIMESTAMP
		WHEN NOT MATCHED THEN
			INSERT (per_kwh, spot_per_kwh, start_time, end_time, duration,
			        channel_type, channel_identifier, kwh, quality, cost)
			VALUES (:per_kwh, :spot_per_kwh, :start_time, :end_time, :duration,
			        :channel_type, :channel_identifier, :kwh, :quality, :cost)`)
	if err != nil {
		return err
	}
	defer func() { _ = stmt.Close() }()

	for _, u := range usage {
		if _, err := stmt.ExecContext(ctx,
			sql.Named("start_time", u.StartTime),
			sql.Named("channel_identifier", u.ChannelIdentifier),
			sql.Named("per_kwh", u.PerKwh),
			sql.Named("spot_per_kwh", u.SpotPerKwh),
			sql.Named("duration", u.Duration),
			sql.Named("kwh", u.Kwh),
			sql.Named("quality", u.Quality),
			sql.Named("cost", u.Cost),
			sql.Named("end_time", u.EndTime),
			sql.Named("channel_type", u.ChannelType),
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) CreateUser(ctx context.Context, username, passwordHash string) (store.User, error) {
	now := time.Now()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO users (username, password_hash, created_at, updated_at)
		VALUES (:1, :2, :3, :4)`,
		username, passwordHash, now, now)
	if err != nil {
		return store.User{}, err
	}
	return s.GetUserByUsername(ctx, username)
}

func (s *Store) StoreRefreshToken(ctx context.Context, arg store.StoreRefreshTokenParams) error {
	_, err := s.db.ExecContext(ctx, `
		MERGE INTO refresh_tokens rt
		USING (SELECT :token_hash AS token_hash FROM dual) src
		ON (rt.token_hash = src.token_hash)
		WHEN MATCHED THEN
			UPDATE SET expires_at = :expires_at
		WHEN NOT MATCHED THEN
			INSERT (token_hash, user_id, username, expires_at)
			VALUES (:token_hash, :user_id, :username, :expires_at)`,
		sql.Named("token_hash", arg.TokenHash),
		sql.Named("expires_at", arg.ExpiresAt),
		sql.Named("user_id", arg.UserID),
		sql.Named("username", arg.Username),
	)
	return err
}

func (s *Store) DeleteRefreshToken(ctx context.Context, tokenHash string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM refresh_tokens WHERE token_hash = :1`, tokenHash)
	return err
}

func (s *Store) DeleteExpiredRefreshTokens(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM refresh_tokens WHERE expires_at < SYSTIMESTAMP`)
	return err
}

func (s *Store) Cleanup(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM Property WHERE time_stamp < SYSTIMESTAMP - INTERVAL '8' DAY`)
	return err
}
