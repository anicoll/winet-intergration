package database

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	dbq "github.com/anicoll/winet-integration/internal/pkg/database/db"
	"github.com/anicoll/winet-integration/internal/pkg/model"
	"github.com/anicoll/winet-integration/internal/pkg/publisher"
)

func (d *Database) Write(ctx context.Context, data []publisher.DataPoint) error {
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

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

func (d *Database) WriteAmberPrices(ctx context.Context, prices []dbq.Amberprice) error {
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

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
