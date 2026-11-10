package v2_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"game-realtime-gm/backend/internal/pve/store"
	"game-realtime-gm/backend/internal/pve/worker"
)

func TestWorkerDispatchAndAttemptFencing(t *testing.T) {
	fixture := setup(t)
	selectedTime := time.Date(2001, 2, 3, 4, 5, 6, 123*int(time.Millisecond), time.UTC)

	type operationState struct {
		status    string
		attempts  int
		scheduled time.Time
		lastError sql.NullString
	}
	for _, scenario := range []struct {
		name, kind, advance, status                 string
		missingHandler, fail, retry, invalidHandler bool
		initialAttempts, attempts                   int
	}{
		{name: "settlement success", kind: "settlement", status: "done", attempts: 1},
		{name: "task completion dispatch", kind: "task_completion", status: "done", attempts: 1},
		{name: "unsupported type", kind: "review_unknown", status: "needs_repair", attempts: 1},
		{name: "missing task handler", kind: "task_completion", missingHandler: true, status: "needs_repair", attempts: 1},
		{name: "missing settlement handler", kind: "settlement", missingHandler: true, status: "needs_repair", attempts: 1},
		{name: "stale failure after completion", kind: "settlement", advance: "done", fail: true, status: "done", attempts: 1},
		{name: "stale failure after newer attempt", kind: "settlement", advance: "attempt", fail: true, status: "retryable_failed", attempts: 1},
		{name: "stale success after reschedule", kind: "settlement", advance: "schedule", status: "pending", attempts: 0},
		{name: "failure and retry success", kind: "settlement", fail: true, retry: true, status: "retryable_failed", attempts: 1},
		{name: "invalid attempt count", kind: "settlement", initialAttempts: -1, status: "needs_repair", attempts: 0},
		{name: "retry budget exhausted", kind: "settlement", initialAttempts: 4, fail: true, status: "needs_repair", attempts: 5},
		{name: "permanent handler error", kind: "settlement", invalidHandler: true, status: "needs_repair", attempts: 1},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			// Worker.Once selects globally. Refuse to consume another fixture's
			// due work, even when the dedicated integration storage is reused.
			var due int
			if err := fixture.db.QueryRowContext(fixture.ctx, "SELECT COUNT(*) FROM pve_pending_operations WHERE status IN ('pending','retryable_failed') AND next_attempt_at<=UTC_TIMESTAMP(3)").Scan(&due); err != nil {
				t.Fatal(err)
			}
			if due != 0 {
				t.Fatalf("worker regression requires no unrelated due operations: %d", due)
			}
			operationID, aggregate := store.ID("worker_refresh"), store.ID("worker_refresh_run")
			payload := store.JSON(map[string]any{"player_id": fixture.players[0]})
			if _, err := fixture.db.ExecContext(fixture.ctx, "INSERT INTO pve_pending_operations(operation_id,operation_type,aggregate_id,payload,attempts,next_attempt_at,last_error) VALUES(?,?,?,?,?,'2001-02-03 04:05:06.123','previous error')", operationID, scenario.kind, aggregate, payload, scenario.initialAttempts); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if _, err := fixture.db.ExecContext(fixture.ctx, "DELETE FROM pve_pending_operations WHERE operation_id=?", operationID); err != nil {
					t.Error(err)
				}
			})
			read := func() operationState {
				t.Helper()
				var state operationState
				if err := fixture.db.QueryRowContext(fixture.ctx, "SELECT status,attempts,next_attempt_at,last_error FROM pve_pending_operations WHERE operation_id=?", operationID).Scan(&state.status, &state.attempts, &state.scheduled, &state.lastError); err != nil {
					t.Fatal(err)
				}
				return state
			}
			if state := read(); !state.scheduled.Equal(selectedTime) {
				t.Fatalf("DATETIME(3) did not scan with its stored precision: %s", state.scheduled.Format(time.RFC3339Nano))
			}
			settlementCalls, taskCalls := 0, 0
			var advanced operationState
			handle := func(ctx context.Context, id string) error {
				if id != aggregate {
					t.Fatalf("worker consumed unrelated operation %s", id)
				}
				// Advance the real row after selection, before the older handler
				// returns. No fake driver supplies the conditional-update result.
				var mutation string
				switch scenario.advance {
				case "done":
					mutation = "UPDATE pve_pending_operations SET status='done',attempts=1,last_error=NULL WHERE operation_id=?"
				case "attempt":
					mutation = "UPDATE pve_pending_operations SET status='retryable_failed',attempts=1,last_error='newer attempt' WHERE operation_id=?"
				case "schedule":
					mutation = "UPDATE pve_pending_operations SET next_attempt_at=DATE_ADD(UTC_TIMESTAMP(3),INTERVAL 1 MINUTE),last_error='rescheduled' WHERE operation_id=?"
				}
				if mutation != "" {
					if _, err := fixture.db.ExecContext(ctx, mutation, operationID); err != nil {
						t.Fatal(err)
					}
					advanced = read()
				}
				if scenario.invalidHandler {
					return store.Invalid
				}
				if scenario.fail && settlementCalls+taskCalls == 1 {
					return errors.New("synthetic worker failure")
				}
				return nil
			}
			processor := worker.Worker{DB: fixture.db}
			processor.Settle = func(ctx context.Context, id string) error {
				settlementCalls++
				return handle(ctx, id)
			}
			processor.CompleteTask = func(ctx context.Context, id string, input json.RawMessage) error {
				taskCalls++
				var received struct {
					PlayerID int64 `json:"player_id"`
				}
				if json.Unmarshal(input, &received) != nil || received.PlayerID != fixture.players[0] {
					t.Fatalf("task payload changed: %s", input)
				}
				return handle(ctx, id)
			}
			if scenario.missingHandler {
				if scenario.kind == "settlement" {
					processor.Settle = nil
				} else {
					processor.CompleteTask = nil
				}
			}
			if err := processor.Once(fixture.ctx); err != nil {
				t.Fatal(err)
			}
			state := read()
			if state.status != scenario.status || state.attempts != scenario.attempts {
				t.Fatalf("status=%s attempts=%d; want %s/%d", state.status, state.attempts, scenario.status, scenario.attempts)
			}
			if scenario.advance != "" {
				if !state.scheduled.Equal(advanced.scheduled) || state.lastError != advanced.lastError {
					t.Fatalf("stale handler overwrote newer schedule or error: %+v; want %+v", state, advanced)
				}
			} else if state.status == "done" {
				if state.lastError.Valid || !state.scheduled.Equal(selectedTime) {
					t.Fatalf("success failed to acknowledge exactly the selected attempt: %+v", state)
				}
			} else if !state.lastError.Valid || state.lastError.String != "operation failed; inspect correlated logs" || !state.scheduled.After(selectedTime) {
				t.Fatalf("failure did not persist retry/quarantine fields: %+v", state)
			}
			wantSettlement, wantTask := 0, 0
			if !scenario.missingHandler && scenario.initialAttempts >= 0 {
				if scenario.kind == "settlement" {
					wantSettlement = 1
				} else if scenario.kind == "task_completion" {
					wantTask = 1
				}
			}
			if settlementCalls != wantSettlement || taskCalls != wantTask {
				t.Fatalf("dispatch calls settlement=%d task=%d; want %d/%d", settlementCalls, taskCalls, wantSettlement, wantTask)
			}
			if scenario.retry {
				// Make the persisted retry due through SQL rather than waiting on
				// wall time. The next Once must scan and fence that stored value.
				if _, err := fixture.db.ExecContext(fixture.ctx, "UPDATE pve_pending_operations SET next_attempt_at='2001-02-03 04:05:06.123' WHERE operation_id=?", operationID); err != nil {
					t.Fatal(err)
				}
				if err := processor.Once(fixture.ctx); err != nil {
					t.Fatal(err)
				}
				state = read()
				if state.status != "done" || state.attempts != 2 || state.lastError.Valid || !state.scheduled.Equal(selectedTime) || settlementCalls != 2 || taskCalls != 0 {
					t.Fatalf("persisted retry did not finish exactly once: %+v settlement=%d task=%d", state, settlementCalls, taskCalls)
				}
			}
		})
	}
}
