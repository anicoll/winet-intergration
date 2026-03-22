package database

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	dbq "github.com/anicoll/winet-integration/internal/pkg/database/db"
	"github.com/anicoll/winet-integration/internal/pkg/model"
	"github.com/anicoll/winet-integration/internal/pkg/publisher"
)

func (d *Database) StoreRefreshToken(ctx context.Context, arg dbq.StoreRefreshTokenParams) error {
	return d.queries.StoreRefreshToken(ctx, arg)
}

func (d *Database) GetRefreshToken(ctx context.Context, tokenHash string) (dbq.RefreshToken, error) {
	return d.queries.GetRefreshToken(ctx, tokenHash)
}

func (d *Database) DeleteRefreshToken(ctx context.Context, tokenHash string) error {
	return d.queries.DeleteRefreshToken(ctx, tokenHash)
}

func (d *Database) DeleteExpiredRefreshTokens(ctx context.Context) error {
	return d.queries.DeleteExpiredRefreshTokens(ctx)
}

func (d *Database) Write(ctx context.Context, data []publisher.DataPoint) error {
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := d.queries.WithTx(tx)
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

func (d *Database) RegisterDevice(ctx context.Context, device *model.Device) error {
	return d.queries.UpsertDevice(ctx, dbq.UpsertDeviceParams{
		ID:           device.ID,
		Model:        pgtype.Text{String: device.Model, Valid: true},
		SerialNumber: pgtype.Text{String: device.SerialNumber, Valid: true},
	})
}

func (d *Database) WriteAmberUsage(ctx context.Context, usage []dbq.Amberusage) error {
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	qtx := d.queries.WithTx(tx)
	for _, u := range usage {
		if err := qtx.UpsertAmberUsage(ctx, dbq.UpsertAmberUsageParams{
			PerKwh:            u.PerKwh,
			SpotPerKwh:        u.SpotPerKwh,
			StartTime:         u.StartTime,
			EndTime:           u.EndTime,
			Duration:          u.Duration,
			ChannelType:       u.ChannelType,
			ChannelIdentifier: u.ChannelIdentifier,
			Kwh:               u.Kwh,
			Quality:           u.Quality,
			Cost:              u.Cost,
		}); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (d *Database) WriteAmberPrices(ctx context.Context, prices []dbq.Amberprice) error {
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := d.queries.WithTx(tx)
	for _, price := range prices {
		if err := qtx.UpsertAmberPrice(ctx, dbq.UpsertAmberPriceParams{
			PerKwh:      price.PerKwh,
			SpotPerKwh:  price.SpotPerKwh,
			StartTime:   price.StartTime,
			EndTime:     price.EndTime,
			Duration:    price.Duration,
			Forecast:    price.Forecast,
			ChannelType: price.ChannelType,
		}); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
