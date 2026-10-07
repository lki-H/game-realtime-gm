package retention

import (
	"context"
	"database/sql"
	"encoding/json"
	"sync"
	"time"

	"game-realtime-gm/backend/internal/pve/store"
)

type Service struct {
	DB        *sql.DB
	Enabled   bool
	Days      int
	BatchSize int
	Now       func() time.Time
	mu        sync.Mutex
	lastRun   time.Time
}

func (service *Service) Tick(ctx context.Context) error {
	if !service.Enabled {
		return nil
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	now := time.Now()
	if service.Now != nil {
		now = service.Now()
	}
	if !service.lastRun.IsZero() && now.Sub(service.lastRun) < time.Minute {
		return nil
	}
	if err := service.Compact(ctx); err != nil {
		return err
	}
	service.lastRun = now
	return nil
}

func (service *Service) Compact(ctx context.Context) error {
	if !service.Enabled {
		return nil
	}
	if service.Days <= 0 || service.BatchSize <= 0 || service.BatchSize > 1000 {
		return store.Invalid
	}
	now := time.Now().UTC()
	if service.Now != nil {
		now = service.Now().UTC()
	}
	cutoff := now.AddDate(0, 0, -service.Days)
	return store.Transaction(ctx, service.DB, func(transaction *sql.Tx) error {
		marker := store.JSON(map[string]bool{"_pve_archived": true})
		rows, err := transaction.QueryContext(ctx, `SELECT r.id,r.final_sequence,r.state
FROM pve_runs r LEFT JOIN pve_run_archives a ON a.run_id=r.id
WHERE r.status IN ('closed','aborted') AND r.ended_at<? AND a.run_id IS NULL
AND NOT EXISTS (SELECT 1 FROM pve_participant_results p WHERE p.run_id=r.id AND p.result_status<>'settled')
AND NOT EXISTS (SELECT 1 FROM pve_pending_operations o WHERE o.aggregate_id=r.id AND o.status<>'done')
AND NOT EXISTS (SELECT 1 FROM pve_player_activity_locks l WHERE l.activity_id=r.id)
ORDER BY r.ended_at,r.id LIMIT ? FOR UPDATE`, cutoff, service.BatchSize)
		if err != nil {
			return err
		}
		type terminal struct {
			id       string
			sequence sql.NullInt64
			state    json.RawMessage
		}
		terminals := []terminal{}
		for rows.Next() {
			var current terminal
			if err := rows.Scan(&current.id, &current.sequence, &current.state); err != nil {
				rows.Close()
				return err
			}
			terminals = append(terminals, current)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, current := range terminals {
			var events, applications int64
			if err := transaction.QueryRowContext(ctx, "SELECT COUNT(*) FROM pve_run_events WHERE run_id=?", current.id).Scan(&events); err != nil {
				return err
			}
			if err := transaction.QueryRowContext(ctx, "SELECT COUNT(*) FROM pve_event_applications a JOIN pve_run_events e ON e.event_id=a.event_id WHERE e.run_id=?", current.id).Scan(&applications); err != nil {
				return err
			}
			if _, err := transaction.ExecContext(ctx, "INSERT INTO pve_run_archives(run_id,final_sequence,summary,fingerprint,event_count,application_count,archived_at) VALUES(?,?,?,?,?,?,?)", current.id, current.sequence.Int64, current.state, store.Hash(store.JSON(current.state)), events, applications, now); err != nil {
				return err
			}
		}
		eventRows, err := transaction.QueryContext(ctx, `SELECT e.event_id FROM pve_run_events e JOIN pve_run_archives a ON a.run_id=e.run_id
WHERE COALESCE(JSON_EXTRACT(e.payload,'$._pve_archived'),false)=false ORDER BY e.event_id LIMIT ? FOR UPDATE`, service.BatchSize)
		if err != nil {
			return err
		}
		eventIDs := []string{}
		for eventRows.Next() {
			var id string
			if err := eventRows.Scan(&id); err != nil {
				eventRows.Close()
				return err
			}
			eventIDs = append(eventIDs, id)
		}
		err = eventRows.Err()
		eventRows.Close()
		if err != nil {
			return err
		}
		for _, id := range eventIDs {
			if _, err := transaction.ExecContext(ctx, "UPDATE pve_run_events SET payload=?,contributors=NULL,target_id=NULL WHERE event_id=?", marker, id); err != nil {
				return err
			}
		}
		if _, err := transaction.ExecContext(ctx, `UPDATE pve_outbox_records SET payload=?
WHERE status='published' AND published_at<? AND COALESCE(JSON_EXTRACT(payload,'$._pve_archived'),false)=false LIMIT ?`, marker, cutoff, service.BatchSize); err != nil {
			return err
		}
		_, err = transaction.ExecContext(ctx, `UPDATE pve_command_results c SET result=?
WHERE c.created_at<? AND COALESCE(JSON_EXTRACT(c.result,'$._pve_archived'),false)=false
AND NOT EXISTS (SELECT 1 FROM pve_player_activity_locks l WHERE l.player_id=c.player_id)
AND NOT EXISTS (SELECT 1 FROM pve_party_members m WHERE m.player_id=c.player_id AND m.status='active')
AND NOT EXISTS (SELECT 1 FROM pve_recruitment_preferences p WHERE p.player_id=c.player_id AND p.expires_at>UTC_TIMESTAMP(3))
AND NOT EXISTS (SELECT 1 FROM pve_run_participants p JOIN pve_runs r ON r.id=p.run_id WHERE p.player_id=c.player_id AND r.status NOT IN ('closed','aborted'))
LIMIT ?`, marker, cutoff, service.BatchSize)
		return err
	})
}
