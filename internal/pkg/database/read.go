package database

import (
	"context"
	"database/sql"
	"time"

	"github.com/anicoll/winet-integration/internal/pkg/models"
	"github.com/jackc/pgx/v5"
)

func (db *Database) GetProperties(ctx context.Context, identifier, slug string, from, to *time.Time) ([]models.Property, error) {
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

	rows, err := db.db.QueryContext(ctx, query, identifier, slug, from, to)
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

func scanProperties(rows *sql.Rows) ([]models.Property, error) {
	var properties []models.Property
	for rows.Next() {
		var property models.Property
		if err := rows.Scan(&property.ID, &property.TimeStamp, &property.UnitOfMeasurement, &property.Value, &property.Identifier, &property.Slug); err != nil {
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

func (db *Database) GetLatestProperties(ctx context.Context) ([]models.Property, error) {
	const query = `
	SELECT DISTINCT ON (slug) id, time_stamp, unit_of_measurement, value, identifier, slug
	FROM Property
	ORDER BY slug, time_stamp DESC;
	`

	rows, err := db.db.QueryContext(ctx, query)
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

func (db *Database) GetAmberPrices(ctx context.Context, from, to time.Time, site *string) ([]models.Amberprice, error) {
	query := `
	SELECT id, per_kwh, spot_per_kwh, start_time, end_time, duration, forecast, channel_type, created_at, updated_at
	FROM AmberPrice
	WHERE start_time BETWEEN $1 AND $2
	ORDER BY start_time DESC;
	`

	rows, err := db.db.QueryContext(ctx, query, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	prices, err := scanAmberPrices(rows)
	if err != nil {
		return nil, err
	}
	return prices, nil
}

func scanAmberPrices(rows *sql.Rows) ([]models.Amberprice, error) {
	var prices []models.Amberprice
	for rows.Next() {
		var price models.Amberprice
		if err := rows.Scan(&price.ID, &price.PerKwh, &price.SpotPerKwh, &price.StartTime, &price.EndTime, &price.Duration, &price.Forecast, &price.ChannelType, &price.CreatedAt, &price.UpdatedAt); err != nil {
			return nil, err
		}
		prices = append(prices, price)
	}

	if err := rows.Err(); err != nil {
		if err == pgx.ErrNoRows {
			return prices, nil
		}
		return nil, err
	}

	return prices, nil
}
