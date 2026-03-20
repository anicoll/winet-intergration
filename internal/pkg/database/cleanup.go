package database

import (
	"context"
	"time"
)

// Cleanup removes old data from the property table that is older than a week.
func (d *Database) Cleanup(ctx context.Context) error {
	return d.queries.CleanupProperties(ctx, time.Now().AddDate(0, 0, -8))
}
