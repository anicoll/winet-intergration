package database

import (
	"context"

	"github.com/anicoll/winet-integration/internal/pkg/model"
	"github.com/anicoll/winet-integration/internal/pkg/models"
)

func (d *Database) Write(ctx context.Context, data []map[string]any) error {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, record := range data {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO Property (time_stamp, unit_of_measurement, value, identifier, slug)
			VALUES ($1, $2, $3, $4, $5)
		`, record["timestamp"], record["unit_of_measurement"], record["value"], record["identifier"], record["slug"]); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (d *Database) RegisterDevice(ctx context.Context, device *model.Device) error {
	_, err := d.db.ExecContext(context.Background(), `
		INSERT INTO Device (id, model, serial_number)
		VALUES ($1, $2, $3)
		ON CONFLICT DO NOTHING;`, device.ID, device.Model, device.SerialNumber)
	if err != nil {
		return err
	}

	return nil
}

func (db *Database) WriteProperty(ctx context.Context, prop models.Property) error {
	return prop.Insert(ctx, db.db)
}

func (d *Database) WriteAmberPrices(ctx context.Context, prices []models.Amberprice) error {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, price := range prices {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO AmberPrice (per_kwh, spot_per_kwh, start_time, end_time, duration, forecast, channel_type)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (start_time, channel_type) DO UPDATE
			SET per_kwh = EXCLUDED.per_kwh,
				spot_per_kwh = EXCLUDED.spot_per_kwh,
				duration = EXCLUDED.duration,
				forecast = EXCLUDED.forecast,
				channel_type = EXCLUDED.channel_type,
				updated_at = NOW()
		`, price.PerKwh, price.SpotPerKwh, price.StartTime, price.EndTime, price.Duration, price.Forecast, price.ChannelType); err != nil {
			return err
		}
	}

	return tx.Commit()
}
