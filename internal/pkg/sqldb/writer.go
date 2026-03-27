// Package sqldb provides a direct Azure SQL (MSSQL) writer, bypassing the
// ingestion function and writing sensor data, devices, and Amber data straight
// to Azure SQL Database.
package sqldb

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/microsoft/go-mssqldb"

	dbpkg "github.com/anicoll/winet-integration/internal/pkg/database/db"
	"github.com/anicoll/winet-integration/internal/pkg/model"
	"github.com/anicoll/winet-integration/internal/pkg/publisher"
)

// Writer writes sensor data, devices, and Amber data directly to Azure SQL Database.
// It implements publisher.Publisher as well as the AmberPricesWriter and
// AmberUsageWriter interfaces used by the Amber services in cmd.
type Writer struct {
	db *sql.DB
}

// New opens an MSSQL connection to dsn and returns a Writer.
// The caller is responsible for calling Close when done.
func New(dsn string) (*Writer, error) {
	db, err := sql.Open("sqlserver", dsn)
	if err != nil {
		return nil, fmt.Errorf("sqldb: open: %w", err)
	}
	return &Writer{db: db}, nil
}

// Close releases the underlying database connection pool.
func (w *Writer) Close() error {
	return w.db.Close()
}

// Write implements publisher.Publisher. It inserts a batch of normalised sensor
// readings into the Property table.
func (w *Writer) Write(ctx context.Context, data []publisher.DataPoint) error {
	const q = `
INSERT INTO Property (time_stamp, unit_of_measurement, value, identifier, slug)
VALUES (@p1, @p2, @p3, @p4, @p5)`

	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sqldb: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, dp := range data {
		if _, err := tx.ExecContext(ctx, q,
			sql.Named("p1", dp.Timestamp),
			sql.Named("p2", dp.UnitOfMeasurement),
			sql.Named("p3", dp.Value),
			sql.Named("p4", dp.Identifier),
			sql.Named("p5", dp.Slug),
		); err != nil {
			return fmt.Errorf("sqldb: insert property: %w", err)
		}
	}
	return tx.Commit()
}

// RegisterDevice implements publisher.Publisher. It upserts a device record
// into the Device table.
func (w *Writer) RegisterDevice(ctx context.Context, device *model.Device) error {
	const q = `
MERGE Device AS target
USING (SELECT @p1 AS id, @p2 AS model, @p3 AS serial_number) AS source
ON target.id = source.id
WHEN NOT MATCHED THEN
    INSERT (id, model, serial_number)
    VALUES (source.id, source.model, source.serial_number);`

	if _, err := w.db.ExecContext(ctx, q,
		sql.Named("p1", device.ID),
		sql.Named("p2", device.Model),
		sql.Named("p3", device.SerialNumber),
	); err != nil {
		return fmt.Errorf("sqldb: upsert device: %w", err)
	}
	return nil
}

// WriteAmberPrices upserts electricity price intervals into the AmberPrice table.
func (w *Writer) WriteAmberPrices(ctx context.Context, prices []dbpkg.Amberprice) error {
	const q = `
MERGE AmberPrice AS target
USING (
    SELECT @p1 AS per_kwh, @p2 AS spot_per_kwh, @p3 AS start_time,
           @p4 AS end_time, @p5 AS duration, @p6 AS forecast, @p7 AS channel_type
) AS source
ON target.start_time = source.start_time AND target.channel_type = source.channel_type
WHEN MATCHED THEN
    UPDATE SET
        per_kwh      = source.per_kwh,
        spot_per_kwh = source.spot_per_kwh,
        duration     = source.duration,
        forecast     = source.forecast,
        updated_at   = SYSDATETIMEOFFSET()
WHEN NOT MATCHED THEN
    INSERT (per_kwh, spot_per_kwh, start_time, end_time, duration, forecast, channel_type)
    VALUES (source.per_kwh, source.spot_per_kwh, source.start_time, source.end_time,
            source.duration, source.forecast, source.channel_type);`

	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sqldb: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, p := range prices {
		if _, err := tx.ExecContext(ctx, q,
			sql.Named("p1", p.PerKwh),
			sql.Named("p2", p.SpotPerKwh),
			sql.Named("p3", p.StartTime),
			sql.Named("p4", p.EndTime),
			sql.Named("p5", p.Duration),
			sql.Named("p6", p.Forecast),
			sql.Named("p7", p.ChannelType),
		); err != nil {
			return fmt.Errorf("sqldb: upsert amber price: %w", err)
		}
	}
	return tx.Commit()
}

// WriteAmberUsage upserts energy usage intervals into the AmberUsage table.
func (w *Writer) WriteAmberUsage(ctx context.Context, usage []dbpkg.Amberusage) error {
	const q = `
MERGE AmberUsage AS target
USING (
    SELECT @p1  AS per_kwh,   @p2  AS spot_per_kwh,        @p3 AS start_time,
           @p4  AS end_time,  @p5  AS duration,             @p6 AS channel_type,
           @p7  AS channel_identifier,                      @p8 AS kwh,
           @p9  AS quality,   @p10 AS cost
) AS source
ON target.start_time = source.start_time AND target.channel_identifier = source.channel_identifier
WHEN MATCHED THEN
    UPDATE SET
        per_kwh      = source.per_kwh,
        spot_per_kwh = source.spot_per_kwh,
        duration     = source.duration,
        kwh          = source.kwh,
        quality      = source.quality,
        cost         = source.cost,
        updated_at   = SYSDATETIMEOFFSET()
WHEN NOT MATCHED THEN
    INSERT (per_kwh, spot_per_kwh, start_time, end_time, duration,
            channel_type, channel_identifier, kwh, quality, cost)
    VALUES (source.per_kwh, source.spot_per_kwh, source.start_time, source.end_time,
            source.duration, source.channel_type, source.channel_identifier,
            source.kwh, source.quality, source.cost);`

	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sqldb: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, u := range usage {
		if _, err := tx.ExecContext(ctx, q,
			sql.Named("p1", u.PerKwh),
			sql.Named("p2", u.SpotPerKwh),
			sql.Named("p3", u.StartTime),
			sql.Named("p4", u.EndTime),
			sql.Named("p5", u.Duration),
			sql.Named("p6", u.ChannelType),
			sql.Named("p7", u.ChannelIdentifier),
			sql.Named("p8", u.Kwh),
			sql.Named("p9", u.Quality),
			sql.Named("p10", u.Cost),
		); err != nil {
			return fmt.Errorf("sqldb: upsert amber usage: %w", err)
		}
	}
	return tx.Commit()
}
