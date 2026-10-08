package control

import (
	"context"
	"database/sql"
	"time"

	"game-realtime-gm/backend/internal/pve/store"
)

type State struct {
	AdmissionState string    `json:"admission_state"`
	Version        int64     `json:"version"`
	Reason         string    `json:"reason"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type Snapshot struct {
	State
	QueuedTickets     int64     `json:"queued_tickets"`
	ProposedTickets   int64     `json:"proposed_tickets"`
	PendingProposals  int64     `json:"pending_proposals"`
	ActiveRuns        int64     `json:"active_runs"`
	EndingRuns        int64     `json:"ending_runs"`
	PendingOperations int64     `json:"pending_operations"`
	NeedsRepair       int64     `json:"needs_repair"`
	ActivityLocks     int64     `json:"activity_locks"`
	PendingOutbox     int64     `json:"pending_outbox"`
	ReadyToStop       bool      `json:"ready_to_stop"`
	ObservedAt        time.Time `json:"observed_at"`
}

func Initialize(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, "INSERT IGNORE INTO pve_service_control(id) VALUES('gameplay')")
	return err
}

func ReadTx(ctx context.Context, transaction *sql.Tx, lock string) (State, error) {
	var state State
	query := "SELECT admission_state,version,reason,updated_at FROM pve_service_control WHERE id='gameplay'"
	if lock == "exclusive" {
		query += " FOR UPDATE"
	} else if lock == "shared" {
		query += " FOR SHARE"
	}
	err := transaction.QueryRowContext(ctx, query).Scan(&state.AdmissionState, &state.Version, &state.Reason, &state.UpdatedAt)
	return state, err
}

func RequireOpen(ctx context.Context, transaction *sql.Tx) error {
	state, err := ReadTx(ctx, transaction, "shared")
	if err != nil {
		return err
	}
	if state.AdmissionState != "open" {
		return store.Maintenance
	}
	return nil
}

func Observe(ctx context.Context, db *sql.DB) (Snapshot, error) {
	var snapshot Snapshot
	transaction, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return snapshot, err
	}
	defer transaction.Rollback()
	snapshot.State, err = ReadTx(ctx, transaction, "")
	if err != nil {
		return snapshot, err
	}
	if err := ReadCountsTx(ctx, transaction, &snapshot); err != nil {
		return snapshot, err
	}
	snapshot.ReadyToStop = snapshot.AdmissionState == "draining" && snapshot.QueuedTickets+snapshot.ProposedTickets+snapshot.PendingProposals+snapshot.ActiveRuns+snapshot.EndingRuns+snapshot.PendingOperations+snapshot.NeedsRepair+snapshot.ActivityLocks+snapshot.PendingOutbox == 0
	snapshot.ObservedAt = time.Now().UTC()
	return snapshot, transaction.Commit()
}

func ReadCountsTx(ctx context.Context, transaction *sql.Tx, snapshot *Snapshot) error {
	queries := []struct {
		query  string
		target *int64
	}{
		{"SELECT COUNT(*) FROM pve_match_tickets WHERE status='queued'", &snapshot.QueuedTickets},
		{"SELECT COUNT(*) FROM pve_match_tickets WHERE status='proposed'", &snapshot.ProposedTickets},
		{"SELECT COUNT(*) FROM pve_match_proposals WHERE status='pending'", &snapshot.PendingProposals},
		{"SELECT COUNT(*) FROM pve_runs WHERE status IN ('provisioning','loading','running')", &snapshot.ActiveRuns},
		{"SELECT COUNT(*) FROM pve_runs WHERE status='ending'", &snapshot.EndingRuns},
		{"SELECT COUNT(*) FROM pve_pending_operations WHERE status IN ('pending','retryable_failed')", &snapshot.PendingOperations},
		{"SELECT COUNT(*) FROM pve_pending_operations WHERE status='needs_repair'", &snapshot.NeedsRepair},
		{"SELECT COUNT(*) FROM pve_player_activity_locks", &snapshot.ActivityLocks},
		{"SELECT COUNT(*) FROM pve_outbox_records WHERE status='pending'", &snapshot.PendingOutbox},
	}
	for _, query := range queries {
		if err := transaction.QueryRowContext(ctx, query.query).Scan(query.target); err != nil {
			return err
		}
	}
	return nil
}
