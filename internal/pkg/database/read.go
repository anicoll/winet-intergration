package database

import (
	"context"
	"iter"
	"slices"
	"time"

	dbq "github.com/anicoll/winet-integration/internal/pkg/database/db"
)

func (d *Database) GetProperties(ctx context.Context, identifier, slug string, from, to *time.Time) ([]dbq.Property, error) {
	if from == nil || to == nil {
		t := time.Now().AddDate(0, 0, -2)
		from = &t
		now := time.Now()
		to = &now
	}
	return d.queries.GetProperties(ctx, dbq.GetPropertiesParams{
		Identifier:  identifier,
		Slug:        slug,
		TimeStamp:   *from,
		TimeStamp_2: *to,
	})
}

func (d *Database) GetLatestProperties(ctx context.Context) (iter.Seq[dbq.Property], error) {
	properties, err := d.queries.GetLatestProperties(ctx)
	if err != nil {
		return nil, err
	}
	return slices.Values(properties), nil
}

func (d *Database) GetAmberPrices(ctx context.Context, from, to time.Time, site *string) ([]dbq.Amberprice, error) {
	return d.queries.GetAmberPrices(ctx, dbq.GetAmberPricesParams{
		StartTime:   from,
		StartTime_2: to,
	})
}
