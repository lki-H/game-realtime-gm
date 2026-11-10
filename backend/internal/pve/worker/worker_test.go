package worker

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestUnsupportedOperationDoesNotRunSettlement(t *testing.T) {
	for _, kind := range []string{"unknown", "repair_audit", "task_completion"} {
		t.Run(kind, func(t *testing.T) {
			job := newTestJob(kind)
			calls := 0
			processor := Worker{DB: workerTestDB(t, job), Settle: func(context.Context, string) error { calls++; return nil }}
			if err := processor.Once(context.Background()); err != nil {
				t.Fatal(err)
			}
			if calls != 0 || job.status != "needs_repair" {
				t.Fatalf("unsupported job ran settlement or was lost: calls=%d status=%s", calls, job.status)
			}
		})
	}
}

func TestMissingSettlementHandlerIsQuarantined(t *testing.T) {
	job := newTestJob("settlement")
	processor := Worker{DB: workerTestDB(t, job)}
	if err := processor.Once(context.Background()); err != nil {
		t.Fatal(err)
	}
	if job.status != "needs_repair" {
		t.Fatalf("missing handler status=%s", job.status)
	}
}

func TestTaskCompletionUsesItsOwnHandler(t *testing.T) {
	job := newTestJob("task_completion")
	calls := 0
	processor := Worker{DB: workerTestDB(t, job), CompleteTask: func(_ context.Context, aggregate string, payload json.RawMessage) error {
		if aggregate != job.aggregate || string(payload) != `{}` {
			t.Fatal("task handler input changed")
		}
		calls++
		return nil
	}}
	if err := processor.Once(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || job.status != "done" {
		t.Fatalf("task handler calls=%d status=%s", calls, job.status)
	}
}

func TestStaleFailureCannotOverwriteCompletedOperation(t *testing.T) {
	job := newTestJob("settlement")
	processor := Worker{DB: workerTestDB(t, job), Settle: func(context.Context, string) error {
		// Another worker finishes the same idempotent operation before this
		// worker receives its older failed result.
		job.status, job.attempts = "done", 1
		return errors.New("older attempt failed")
	}}
	if err := processor.Once(context.Background()); err != nil {
		t.Fatal(err)
	}
	if job.status != "done" || job.attempts != 1 {
		t.Fatalf("stale attempt overwrote durable completion: status=%s attempts=%d", job.status, job.attempts)
	}
}

func TestStaleSuccessCannotOverwriteRescheduledOperation(t *testing.T) {
	job := newTestJob("settlement")
	processor := Worker{DB: workerTestDB(t, job), Settle: func(context.Context, string) error {
		// A repair may reset the attempt counter. The schedule also fences the
		// original attempt so it cannot acknowledge a newly scheduled job.
		job.scheduled = job.scheduled.Add(time.Minute)
		return nil
	}}
	if err := processor.Once(context.Background()); err != nil {
		t.Fatal(err)
	}
	if job.status != "pending" || job.attempts != 0 {
		t.Fatalf("stale success acknowledged repaired job: status=%s attempts=%d", job.status, job.attempts)
	}
}

func TestInvalidAttemptCountDoesNotExecuteOrShift(t *testing.T) {
	job := newTestJob("settlement")
	job.attempts = -1
	calls := 0
	processor := Worker{DB: workerTestDB(t, job), Settle: func(context.Context, string) error { calls++; return errors.New("failed") }}
	if err := processor.Once(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 0 || job.status != "needs_repair" {
		t.Fatalf("invalid attempt executed: calls=%d status=%s", calls, job.status)
	}
}

// This narrow database/sql driver exercises Worker.Once without a database or
// new dependency. It models the pending-operation conditional update and an
// empty outbox; handler callbacks can advance the stored job before the update.
type testJob struct {
	kind, aggregate, status string
	attempts                int64
	scheduled               time.Time
}

func newTestJob(kind string) *testJob {
	return &testJob{kind: kind, aggregate: "run_test", status: "pending", scheduled: time.Now().UTC().Truncate(time.Millisecond)}
}

var testDriverID atomic.Uint64

func workerTestDB(t *testing.T, job *testJob) *sql.DB {
	t.Helper()
	name := fmt.Sprintf("worker-test-%d", testDriverID.Add(1))
	sql.Register(name, &workerDriver{job: job})
	db, err := sql.Open(name, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

type workerDriver struct{ job *testJob }
type workerConnection struct{ job *testJob }

func (d *workerDriver) Open(string) (driver.Conn, error) { return &workerConnection{job: d.job}, nil }
func (c *workerConnection) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("unexpected prepare")
}
func (c *workerConnection) Close() error              { return nil }
func (c *workerConnection) Begin() (driver.Tx, error) { return workerTransaction{}, nil }
func (c *workerConnection) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	return workerTransaction{}, nil
}

type workerTransaction struct{}

func (workerTransaction) Commit() error   { return nil }
func (workerTransaction) Rollback() error { return nil }

func (c *workerConnection) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	if strings.Contains(query, "FROM pve_pending_operations") {
		columns := []string{"operation_id", "aggregate_id", "operation_type", "payload", "attempts"}
		values := []driver.Value{"operation_test", c.job.aggregate, c.job.kind, []byte(`{}`), c.job.attempts}
		if strings.Contains(query, "attempts,next_attempt_at") {
			columns, values = append(columns, "next_attempt_at"), append(values, c.job.scheduled)
		}
		return &workerRows{columns: columns, values: values}, nil
	}
	if strings.Contains(query, "FROM pve_outbox_records") {
		return &workerRows{columns: []string{"id", "payload"}}, nil
	}
	return nil, fmt.Errorf("unexpected query: %s", query)
}

func (c *workerConnection) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if !strings.HasPrefix(query, "UPDATE pve_pending_operations SET ") {
		return nil, fmt.Errorf("unexpected update: %s", query)
	}
	completed := strings.Contains(query, "SET status='done'")
	if strings.Contains(query, "AND attempts=? AND next_attempt_at=?") {
		offset := 1
		if !completed {
			offset = 3
		}
		if (c.job.status != "pending" && c.job.status != "retryable_failed") || c.job.attempts != args[offset].Value.(int64) || !c.job.scheduled.Equal(args[offset+1].Value.(time.Time)) {
			return driver.RowsAffected(0), nil
		}
	}
	if completed {
		c.job.status = "done"
	} else {
		c.job.status, c.job.scheduled = args[0].Value.(string), args[1].Value.(time.Time)
	}
	c.job.attempts++
	return driver.RowsAffected(1), nil
}

type workerRows struct {
	columns []string
	values  []driver.Value
}

func (r *workerRows) Columns() []string { return r.columns }
func (r *workerRows) Close() error      { return nil }
func (r *workerRows) Next(values []driver.Value) error {
	if r.values == nil {
		return io.EOF
	}
	copy(values, r.values)
	r.values = nil
	return nil
}
