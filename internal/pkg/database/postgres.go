package database

import (
	"context"
	"io"
	"time"

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
CREATE TABLE IF NOT EXISTS Properties (
    id SERIAL PRIMARY KEY,
    timeStamp TIMESTAMP WITH TIME ZONE NOT NULL,
    unit_of_measurement TEXT NOT NULL,
    value TEXT NOT NULL,
    identifier TEXT NOT NULL,
    slug TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_properties_identifier ON Properties (Identifier);
CREATE INDEX IF NOT EXISTS idx_properties_timestamp ON Properties (TimeStamp);
`
	if _, err := conn.Exec(ctx, createPropertiesTableSQL); err != nil {
		panic(err)
	}
}

func (db *Database) Close() error {
	if db.conn == nil {
		return nil
	}
	return db.conn.Close(context.Background())
}

type Property struct {
	Id         int64     `json:"id"`
	TimeStamp  time.Time `json:"timestamp"`
	Unit       string    `json:"unit_of_measurement"`
	Value      string    `json:"value"`
	Identifier string    `json:"identifier"`
	Slug       string    `json:"slug"`
}
type Properties []Property

func (db *Database) GetLatestProperties(ctx context.Context) (Properties, error) {
	const query = `
	SELECT DISTINCT ON (slug) id, timeStamp, unit_of_measurement, value, identifier, slug
	FROM Properties
	ORDER BY slug, timeStamp DESC;
	`

	rows, err := db.conn.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var properties Properties
	for rows.Next() {
		var property Property
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

func (db *Database) WriteProperty(ctx context.Context, prop Property) error {
	const insertSQL = `
	INSERT INTO Properties (timeStamp, unit_of_measurement, value, identifier, slug)
	VALUES ($1, $2, $3, $4, $5)
	`
	if _, err := db.conn.Exec(ctx, insertSQL, prop.TimeStamp, prop.Unit, prop.Value, prop.Identifier, prop.Slug); err != nil {
		return err
	}
	return nil
}
