package experiment

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestIntegrationBridgeFindsLateCommittedLowerID(t *testing.T) {
	fixture := integration(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	transaction, err := fixture.admin.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer transaction.Rollback()
	payload := fmt.Sprintf(`{"type":"v2.run.result","players":[1],"data":{"run_id":%q}}`, fixture.report.RunID)
	insert := `INSERT INTO pve_outbox_records(operation_id,event_type,aggregate_id,payload) VALUES(?,'v2.run.result','notification',?)`
	lower, err := transaction.ExecContext(ctx, insert, "r5_late_low", payload)
	if err != nil {
		t.Fatal(err)
	}
	lowID, err := lower.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	higher, err := fixture.admin.ExecContext(ctx, insert, "r5_late_high", payload)
	if err != nil {
		t.Fatal(err)
	}
	highID, err := higher.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	defer fixture.admin.ExecContext(context.Background(), "DELETE FROM pve_outbox_records WHERE operation_id IN ('r5_late_low','r5_late_high')")
	bridge := Bridge{Source: fixture.source, Projection: fixture.projection, SourceName: fixture.cfg.SourceName, Metrics: fixture.metrics}
	if err := bridge.Once(ctx); err != nil {
		t.Fatal(err)
	}
	var after int64
	fixture.projection.DB.QueryRowContext(ctx, "SELECT after_id FROM r5_scan_state").Scan(&after)
	if after != highID {
		t.Fatal("test did not advance beyond the uncommitted row")
	}
	if err := transaction.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := bridge.Once(ctx); err != nil {
		t.Fatal(err)
	}
	if err := bridge.Once(ctx); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := fixture.projection.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM r5_deliveries WHERE message_id=?", "outbox:"+fixture.cfg.SourceName+":"+fmt.Sprint(lowID)).Scan(&count); err != nil || count != 1 {
		t.Fatal("late committed lower sequence was lost")
	}
}
