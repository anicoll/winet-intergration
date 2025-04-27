package database

import (
	"context"
	"io"
	"time"

	"github.com/anicoll/winet-integration/internal/pkg/model"
	"github.com/jackc/pgx/v5"
)

type Database struct {
	conn *pgx.Conn
	io.Closer
}

func NewDatabase(ctx context.Context, conn *pgx.Conn) *Database {
	initialise(ctx, conn)
	return &Database{
		conn: conn,
	}
}

func initialise(ctx context.Context, conn *pgx.Conn) {
	const createPropertiesTableSQL = `
		CREATE TABLE IF NOT EXISTS Property (
				id SERIAL PRIMARY KEY,
				time_stamp TIMESTAMP WITH TIME ZONE NOT NULL,
				unit_of_measurement TEXT NOT NULL,
				value TEXT NOT NULL,
				identifier TEXT NOT NULL,
				slug TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_properties_identifier ON Property (identifier);
		CREATE INDEX IF NOT EXISTS idx_properties_timestamp ON Property (time_stamp);
		`
	if _, err := conn.Exec(ctx, createPropertiesTableSQL); err != nil {
		panic(err)
	}

	const createDeviceTableSQL = `
	CREATE TABLE IF NOT EXISTS Device (
		id           	TEXT PRIMARY KEY,
		model        	TEXT,
		serial_number TEXT,
		created_at 		TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);
	`
	if _, err := conn.Exec(ctx, createDeviceTableSQL); err != nil {
		panic(err)
	}
}

func (db *Database) Close() error {
	if db.conn == nil {
		return nil
	}
	return db.conn.Close(context.Background())
}

func (db *Database) GetProperties(ctx context.Context, identifier, slug string, from, to *time.Time) (model.Properties, error) {
	query := ""
	if from == nil || to == nil {
		from = func() *time.Time {
			t := time.Now().AddDate(0, 0, -2)
			return &t
		}()
		to = func() *time.Time {
			t := time.Now()
			return &t
		}()
	}
	query = `
	SELECT id, time_stamp, unit_of_measurement, value, identifier, slug
	FROM Property
	WHERE identifier = $1 AND slug = $2 AND time_stamp BETWEEN $3 AND $4
	ORDER BY time_stamp DESC;
	`

	rows, err := db.conn.Query(ctx, query, identifier, slug, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	properties, err := scanProperties(rows)
	if err != nil {
		return nil, err
	}
	return properties, nil
}

func scanProperties(rows pgx.Rows) (model.Properties, error) {
	var properties model.Properties
	for rows.Next() {
		var property model.Property
		if err := rows.Scan(&property.Id, &property.TimeStamp, &property.Unit, &property.Value, &property.Identifier, &property.Slug); err != nil {
			return nil, err
		}
		properties = append(properties, property)
	}

	if err := rows.Err(); err != nil {
		if err == pgx.ErrNoRows {
			return properties, nil
		}
		return nil, err
	}

	return properties, nil
}

func (db *Database) GetLatestProperties(ctx context.Context) (model.Properties, error) {
	const query = `
	SELECT DISTINCT ON (slug) id, time_stamp, unit_of_measurement, value, identifier, slug
	FROM Property
	ORDER BY slug, time_stamp DESC;
	`

	rows, err := db.conn.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	properties, err := scanProperties(rows)
	if err != nil {
		return nil, err
	}
	return properties, nil
}

func (db *Database) WriteProperty(ctx context.Context, prop model.Property) error {
	const insertSQL = `
	INSERT INTO Property (time_stamp, unit_of_measurement, value, identifier, slug)
	VALUES ($1, $2, $3, $4, $5)
	`
	if _, err := db.conn.Exec(ctx, insertSQL, prop.TimeStamp, prop.Unit, prop.Value, prop.Identifier, prop.Slug); err != nil {
		return err
	}
	return nil
}
