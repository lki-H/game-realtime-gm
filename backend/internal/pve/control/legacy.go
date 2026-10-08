package control

import (
	"context"
	"database/sql"
	"fmt"
)

func RequireLegacyIdle(ctx context.Context, db *sql.DB) error {
	var tables int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='pve_runs'").Scan(&tables); err != nil {
		return err
	}
	if tables == 0 {
		return nil
	}
	var occupied int
	if err := db.QueryRowContext(ctx, "SELECT (SELECT COUNT(*) FROM pve_runs WHERE status NOT IN ('closed','aborted'))+(SELECT COUNT(*) FROM pve_pending_operations WHERE status<>'done')+(SELECT COUNT(*) FROM pve_player_activity_locks)+(SELECT COUNT(*) FROM pve_match_tickets WHERE status IN ('queued','proposed'))+(SELECT COUNT(*) FROM pve_match_proposals WHERE status='pending')+(SELECT COUNT(*) FROM pve_outbox_records WHERE status='pending')").Scan(&occupied); err != nil {
		return err
	}
	if occupied != 0 {
		return fmt.Errorf("legacy regression cannot start while V2 activity or work remains")
	}
	return nil
}
