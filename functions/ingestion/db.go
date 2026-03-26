package main

import (
	"context"
	"database/sql"
	"time"
)

// store wraps *sql.DB and executes the T-SQL queries defined in
// internal/pkg/database/queries/ against Azure SQL Database.
// Parameters are passed as sql.Named("p1", v) to match the @p1 placeholders
// used in go-mssqldb.
type store struct {
	db *sql.DB
}

func newStore(db *sql.DB) *store { return &store{db: db} }

// --- Property -----------------------------------------------------------------

func (s *store) insertProperties(ctx context.Context, rows []propertyRow) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	const q = `
INSERT INTO Property (time_stamp, unit_of_measurement, value, identifier, slug)
VALUES (@p1, @p2, @p3, @p4, @p5)`

	for _, r := range rows {
		if _, err := tx.ExecContext(ctx, q,
			sql.Named("p1", r.Timestamp),
			sql.Named("p2", r.UnitOfMeasurement),
			sql.Named("p3", r.Value),
			sql.Named("p4", r.Identifier),
			sql.Named("p5", r.Slug),
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

type propertyRow struct {
	Timestamp         time.Time
	UnitOfMeasurement string
	Value             string
	Identifier        string
	Slug              string
}

// --- Device -------------------------------------------------------------------

func (s *store) upsertDevice(ctx context.Context, id, model, serialNumber string) error {
	const q = `
MERGE Device AS target
USING (SELECT @p1 AS id, @p2 AS model, @p3 AS serial_number) AS source
ON target.id = source.id
WHEN NOT MATCHED THEN
    INSERT (id, model, serial_number)
    VALUES (source.id, source.model, source.serial_number);`

	_, err := s.db.ExecContext(ctx, q,
		sql.Named("p1", id),
		sql.Named("p2", model),
		sql.Named("p3", serialNumber),
	)
	return err
}

// --- AmberPrice ---------------------------------------------------------------

func (s *store) upsertAmberPrices(ctx context.Context, prices []amberPriceRow) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

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
			return err
		}
	}
	return tx.Commit()
}

type amberPriceRow struct {
	PerKwh      float64
	SpotPerKwh  float64
	StartTime   time.Time
	EndTime     time.Time
	Duration    int32
	Forecast    bool
	ChannelType string
}

// --- AmberUsage ---------------------------------------------------------------

func (s *store) upsertAmberUsage(ctx context.Context, rows []amberUsageRow) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

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

	for _, u := range rows {
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
			return err
		}
	}
	return tx.Commit()
}

type amberUsageRow struct {
	PerKwh            float64
	SpotPerKwh        float64
	StartTime         time.Time
	EndTime           time.Time
	Duration          int32
	ChannelType       string
	ChannelIdentifier string
	Kwh               float64
	Quality           string
	Cost              float64
}

// --- PendingCommands ----------------------------------------------------------

type pendingCommandRow struct {
	ID          string
	DeviceID    string
	CommandType string
	Payload     string
	CreatedAt   time.Time
}

func (s *store) getPendingCommands(ctx context.Context, deviceID string) ([]pendingCommandRow, error) {
	const q = `
SELECT id, device_id, command_type, payload, created_at
FROM pending_commands
WHERE device_id = @p1
  AND acked_at IS NULL
ORDER BY created_at ASC`

	rows, err := s.db.QueryContext(ctx, q, sql.Named("p1", deviceID))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var result []pendingCommandRow
	for rows.Next() {
		var r pendingCommandRow
		if err := rows.Scan(&r.ID, &r.DeviceID, &r.CommandType, &r.Payload, &r.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

func (s *store) ackCommand(ctx context.Context, commandID string, success bool) error {
	const q = `
UPDATE pending_commands
SET acked_at = SYSDATETIMEOFFSET(),
    success  = @p2
WHERE id = @p1`

	_, err := s.db.ExecContext(ctx, q,
		sql.Named("p1", commandID),
		sql.Named("p2", success),
	)
	return err
}
