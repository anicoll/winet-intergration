package database

import (
	"context"
	"time"
)

// Cleanup removes old data from the property table that is older than a week.
func (db *Database) Cleanup(ctx context.Context) error {
	if _, err := db.db.ExecContext(ctx, "DELETE FROM Property WHERE time_stamp < $1", time.Now().AddDate(0, 0, -8)); err != nil {
		return err
	}
	return nil
}
