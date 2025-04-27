package database

import (
	"context"

	"github.com/anicoll/winet-integration/internal/pkg/model"
)

func (d *Database) Write(ctx context.Context, data []map[string]any) error {
	tx, err := d.conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	for _, record := range data {
		if _, err := tx.Exec(ctx, `
			INSERT INTO Property (time_stamp, unit_of_measurement, value, identifier, slug)
			VALUES ($1, $2, $3, $4, $5)
		`, record["timestamp"], record["unit_of_measurement"], record["value"], record["identifier"], record["slug"]); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

func (d *Database) RegisterDevice(device *model.Device) error {
	_, err := d.conn.Exec(context.Background(), `
		INSERT INTO Device (id, model, serial_number)
		VALUES ($1, $2, $3)
		ON CONFLICT DO NOTHING;`, device.ID, device.Model, device.SerialNumber)
	if err != nil {
		return err
	}

	return nil
}
