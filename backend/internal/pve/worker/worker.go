package worker

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"time"

	"game-realtime-gm/backend/internal/pve/store"
)

type Worker struct {
	DB           *sql.DB
	Settle       func(context.Context, string) error
	CompleteTask func(context.Context, string, json.RawMessage) error
	Notify       func([]int64, string, json.RawMessage)
	Tick         func(context.Context) error
}
type outboxItem struct {
	id   int64
	data []byte
}
type notification struct {
	Players []int64
	Kind    string
	Data    json.RawMessage
}

func (worker *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			iteration, cancel := context.WithTimeout(ctx, 10*time.Second)
			if err := worker.Once(iteration); err != nil {
				slog.Error("pve worker failed", "error", err.Error())
			}
			cancel()
		}
	}
}
func (worker *Worker) Once(ctx context.Context) error {
	if worker.Tick != nil {
		if err := worker.Tick(ctx); err != nil {
			slog.Error("pve maintenance deferred", "error", err.Error())
		}
	}
	rows, err := worker.DB.QueryContext(ctx, "SELECT operation_id,aggregate_id,operation_type,payload,attempts FROM pve_pending_operations WHERE status IN ('pending','retryable_failed') AND next_attempt_at<=UTC_TIMESTAMP(3) ORDER BY next_attempt_at LIMIT 20")
	if err != nil {
		return err
	}
	type item struct {
		id, aggregate string
		kind          string
		payload       json.RawMessage
		attempts      int
	}
	items := []item{}
	for rows.Next() {
		var current item
		if err := rows.Scan(&current.id, &current.aggregate, &current.kind, &current.payload, &current.attempts); err != nil {
			rows.Close()
			return err
		}
		items = append(items, current)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, current := range items {
		var err error
		if current.kind == "task_completion" && worker.CompleteTask != nil {
			err = worker.CompleteTask(ctx, current.aggregate, current.payload)
		} else {
			err = worker.Settle(ctx, current.aggregate)
		}
		if err == nil {
			if _, err := worker.DB.ExecContext(ctx, "UPDATE pve_pending_operations SET status='done',attempts=attempts+1,last_error=NULL,updated_at=UTC_TIMESTAMP(3) WHERE operation_id=?", current.id); err != nil {
				return err
			}
		} else {
			status := "retryable_failed"
			if current.attempts >= 4 {
				status = "needs_repair"
			}
			_, updateErr := worker.DB.ExecContext(ctx, "UPDATE pve_pending_operations SET status=?,attempts=attempts+1,next_attempt_at=?,last_error='settlement failed; inspect correlated logs',updated_at=UTC_TIMESTAMP(3) WHERE operation_id=?", status, time.Now().UTC().Add(time.Duration(1<<current.attempts)*time.Second), current.id)
			if updateErr != nil {
				return updateErr
			}
			slog.Error("pve settlement retry", "operation_id", current.id, "run_id", current.aggregate, "error", err.Error())
		}
		if err != nil && current.attempts == 0 {
			slog.Warn("pve work pending", "operation_id", current.id)
		}
	}
	notifications := []notification{}
	err = store.Transaction(ctx, worker.DB, func(transaction *sql.Tx) error {
		notifications = notifications[:0]
		rows, err := transaction.QueryContext(ctx, "SELECT id,payload FROM pve_outbox_records WHERE status='pending' ORDER BY id LIMIT 100 FOR UPDATE SKIP LOCKED")
		if err != nil {
			return err
		}
		outboxItems := []outboxItem{}
		for rows.Next() {
			var current outboxItem
			if err := rows.Scan(&current.id, &current.data); err != nil {
				rows.Close()
				return err
			}
			outboxItems = append(outboxItems, current)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, current := range outboxItems {
			var envelope struct {
				Players []int64         `json:"players"`
				Type    string          `json:"type"`
				Data    json.RawMessage `json:"data"`
			}
			if err := json.Unmarshal(current.data, &envelope); err != nil {
				return err
			}
			notifications = append(notifications, notification{envelope.Players, envelope.Type, envelope.Data})
			if _, err := transaction.ExecContext(ctx, "UPDATE pve_outbox_records SET status='published',published_at=UTC_TIMESTAMP(3) WHERE id=?", current.id); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if worker.Notify != nil {
		for _, notification := range notifications {
			worker.Notify(notification.Players, notification.Kind, notification.Data)
		}
	}
	return nil
}
