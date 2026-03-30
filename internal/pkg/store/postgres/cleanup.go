package postgres

import (
	"context"
	"time"
)

// Cleanup removes Property rows older than 8 days.
func (s *Store) Cleanup(ctx context.Context) error {
	return s.queries.CleanupProperties(ctx, time.Now().AddDate(0, 0, -8))
}
