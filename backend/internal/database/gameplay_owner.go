package database

import (
	"context"
	"database/sql"
	"time"
)

func WatchGameplayOwner(ctx context.Context, connection *sql.Conn, interval time.Duration, lost func()) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			check, cancel := context.WithTimeout(ctx, 2*time.Second)
			var owned bool
			err := connection.QueryRowContext(check, "SELECT IS_USED_LOCK(CONCAT('gm-gameplay:',DATABASE()))=CONNECTION_ID()").Scan(&owned)
			cancel()
			if ctx.Err() != nil {
				return
			}
			if err != nil || !owned {
				lost()
				return
			}
		}
	}
}
