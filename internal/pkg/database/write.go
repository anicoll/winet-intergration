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
			INSERT INTO Properties (timeStamp, unit_of_measurement, value, identifier, slug)
			VALUES ($1, $2, $3, $4, $5)
		`, record["timestamp"], record["unit_of_measurement"], record["value"], record["identifier"], record["slug"]); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

// RegisterDevice is a placeholder for the actual implementation of device registration.
// postgres doesnt need a device registration step, it just needs to write the data.
func (d *Database) RegisterDevice(device *model.Device) error {
	return nil
}
