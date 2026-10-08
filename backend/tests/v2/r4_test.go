package v2_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"game-realtime-gm/backend/internal/auth"
	"game-realtime-gm/backend/internal/pve/control"
	"game-realtime-gm/backend/internal/pve/party"
	"game-realtime-gm/backend/internal/pve/run"
	"game-realtime-gm/backend/internal/pve/store"
)

func r4Operator(t *testing.T, fixture *fixture) (int64, string) {
	t.Helper()
	result, err := fixture.db.ExecContext(fixture.ctx, "INSERT INTO admins(username,password_hash,display_name,role) VALUES(?,'synthetic','R4 operator','operator')", store.ID("operator"))
	if err != nil {
		t.Fatal(err)
	}
	adminID, err := result.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	token, err := auth.GenerateAdminToken(fixture.secret, adminID, "operator", "operator")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = fixture.db.Exec("DELETE FROM admin_operation_logs WHERE admin_id=?", adminID)
		_, _ = fixture.db.Exec("DELETE FROM admins WHERE id=?", adminID)
	})
	return adminID, token
}

func r4ControlRequest(fixture *fixture, token string, input any) (int, error) {
	request, err := http.NewRequest("POST", fixture.server.URL+"/api/admin/v2/control", bytes.NewReader(store.JSON(input)))
	if err != nil {
		return 0, err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	return response.StatusCode, nil
}

func r4Set(t *testing.T, fixture *fixture, token, state string, version int64) {
	t.Helper()
	status, err := r4ControlRequest(fixture, token, map[string]any{"operation_id": store.ID("control"), "admission_state": state, "expected_version": version, "reason": "isolated R4 verification"})
	if err != nil || status != http.StatusOK {
		t.Fatalf("control %s status=%d err=%v", state, status, err)
	}
}

func TestR4DrainClearsProposalsAndRejectsAdmission(t *testing.T) {
	fixture := setup(t)
	_, token := r4Operator(t, fixture)
	plan := party.Plan{Operation: "training_ground", Difficulty: "normal", FillPolicy: "public", AllowPartial: true}
	for _, player := range fixture.players {
		fixture.command(t, player, "v2.match.enqueue", map[string]any{"plan": plan})
	}
	if err := fixture.app.Match.Match(fixture.ctx); err != nil {
		t.Fatal(err)
	}
	var proposal string
	if err := fixture.db.QueryRow("SELECT id FROM pve_match_proposals WHERE status='pending'").Scan(&proposal); err != nil {
		t.Fatal(err)
	}
	r4Set(t, fixture, token, "draining", 1)
	snapshot, err := control.Observe(fixture.ctx, fixture.db)
	if err != nil || snapshot.QueuedTickets != 0 || snapshot.ProposedTickets != 0 || snapshot.PendingProposals != 0 || snapshot.ActivityLocks != 0 || snapshot.ReadyToStop {
		t.Fatalf("drain snapshot=%+v err=%v", snapshot, err)
	}
	if _, err := fixture.app.Command(fixture.ctx, fixture.players[0], store.ID("enqueue"), "v2.match.enqueue", store.JSON(map[string]any{"plan": plan})); !errors.Is(err, store.Maintenance) {
		t.Fatalf("admission while draining err=%v", err)
	}
	if _, err := fixture.app.Runs.Create(fixture.ctx, run.CreateRequest{OperationName: "training_ground", Difficulty: "normal", PlayerIDs: fixture.players[:1]}); !errors.Is(err, store.Maintenance) {
		t.Fatalf("direct run admission err=%v", err)
	}
	if _, err := fixture.app.Command(fixture.ctx, fixture.players[0], store.ID("confirm"), "v2.match.proposal_confirm", store.JSON(map[string]any{"proposal_id": proposal, "revision": 1})); !errors.Is(err, store.Conflict) {
		t.Fatalf("old confirmation revived proposal: %v", err)
	}
	if err := fixture.app.Worker.Once(fixture.ctx); err != nil {
		t.Fatal(err)
	}
	snapshot, err = control.Observe(fixture.ctx, fixture.db)
	if err != nil || !snapshot.ReadyToStop {
		t.Fatalf("drain did not converge: %+v %v", snapshot, err)
	}
	r4Set(t, fixture, token, "closed", 2)
	if err := fixture.app.Recover(fixture.ctx); err != nil {
		t.Fatal(err)
	}
	snapshot, err = control.Observe(fixture.ctx, fixture.db)
	if err != nil || snapshot.AdmissionState != "closed" || snapshot.Version != 3 {
		t.Fatalf("restart reopened admission: %+v %v", snapshot, err)
	}
	r4Set(t, fixture, token, "open", 3)
	fixture.command(t, fixture.players[0], "v2.match.enqueue", map[string]any{"plan": plan})
}

func TestR4DrainPreservesRunAndWorkerSettlement(t *testing.T) {
	fixture := setup(t)
	_, token := r4Operator(t, fixture)
	id, _ := fixture.assigned(t)
	for index, player := range fixture.players {
		fixture.event(t, id, int64(index+1), "loaded", player, "")
	}
	r4Set(t, fixture, token, "draining", 1)
	if status, err := r4ControlRequest(fixture, token, map[string]any{"operation_id": store.ID("close"), "admission_state": "closed", "expected_version": 2, "reason": "must wait for active run"}); err != nil || status != http.StatusConflict {
		t.Fatalf("premature stop status=%d err=%v", status, err)
	}
	for index := 0; index < 3; index++ {
		fixture.event(t, id, int64(index+5), "kill", fixture.players[index], store.ID("enemy"))
	}
	fixture.event(t, id, 8, "interact", fixture.players[1], "terminal")
	fixture.event(t, id, 9, "reach", fixture.players[2], "exit")
	if err := fixture.app.Worker.Once(fixture.ctx); err != nil {
		t.Fatal(err)
	}
	state, err := fixture.app.Runs.Snapshot(fixture.ctx, id)
	if err != nil || state.Status != "closed" || !state.Settled || state.EndReason != "success" {
		t.Fatalf("drain stopped existing run: %+v %v", state, err)
	}
	snapshot, err := control.Observe(fixture.ctx, fixture.db)
	if err != nil || !snapshot.ReadyToStop {
		t.Fatalf("settlement did not drain: %+v %v", snapshot, err)
	}
	var before int64
	if err := fixture.db.QueryRow("SELECT SUM(soft_currency) FROM player_assets").Scan(&before); err != nil {
		t.Fatal(err)
	}
	if err := fixture.app.Recover(fixture.ctx); err != nil {
		t.Fatal(err)
	}
	if err := fixture.app.Settlement.Settle(fixture.ctx, id); err != nil {
		t.Fatal(err)
	}
	var after int64
	if err := fixture.db.QueryRow("SELECT SUM(soft_currency) FROM player_assets").Scan(&after); err != nil || before != after {
		t.Fatalf("restart repeated assets: before=%d after=%d err=%v", before, after, err)
	}
}

func TestR4ControlPermissionsVersionsAndConcurrentRetries(t *testing.T) {
	fixture := setup(t)
	adminID, token := r4Operator(t, fixture)
	playerToken, err := auth.GenerateToken(fixture.secret, fixture.players[0], "player")
	if err != nil {
		t.Fatal(err)
	}
	operation := store.ID("drain")
	input := map[string]any{"operation_id": operation, "admission_state": "draining", "expected_version": 1, "reason": "concurrent idempotent maintenance"}
	if status, err := r4ControlRequest(fixture, playerToken, input); err != nil || status != http.StatusForbidden {
		t.Fatalf("player control status=%d err=%v", status, err)
	}
	if _, err := fixture.db.Exec("UPDATE admins SET role='gm' WHERE id=?", adminID); err != nil {
		t.Fatal(err)
	}
	if status, err := r4ControlRequest(fixture, token, input); err != nil || status != http.StatusForbidden {
		t.Fatalf("revoked operator status=%d err=%v", status, err)
	}
	if _, err := fixture.db.Exec("UPDATE admins SET role='operator' WHERE id=?", adminID); err != nil {
		t.Fatal(err)
	}
	var wait sync.WaitGroup
	results := make(chan int, 2)
	for attempt := 0; attempt < 2; attempt++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			status, err := r4ControlRequest(fixture, token, input)
			if err != nil {
				results <- 0
				return
			}
			results <- status
		}()
	}
	wait.Wait()
	close(results)
	for status := range results {
		if status != http.StatusOK {
			t.Fatalf("concurrent retry status=%d", status)
		}
	}
	var audits int
	if err := fixture.db.QueryRow("SELECT COUNT(*) FROM admin_operation_logs WHERE admin_id=? AND action='admin.v2.control.draining'", adminID).Scan(&audits); err != nil || audits != 1 {
		t.Fatalf("audit count=%d err=%v", audits, err)
	}
	input["reason"] = "different intent"
	if status, err := r4ControlRequest(fixture, token, input); err != nil || status != http.StatusConflict {
		t.Fatalf("reused operation status=%d err=%v", status, err)
	}
	input["operation_id"] = store.ID("stale")
	if status, err := r4ControlRequest(fixture, token, input); err != nil || status != http.StatusConflict {
		t.Fatalf("stale version status=%d err=%v", status, err)
	}
	request, _ := http.NewRequest("GET", fixture.server.URL+"/api/admin/v2/control", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var envelope struct {
		Data control.Snapshot `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil || envelope.Data.Version != 2 {
		t.Fatalf("state query=%+v err=%v", envelope, err)
	}
}

func TestR4LoadingFailureDuringDrainDoesNotRequeue(t *testing.T) {
	fixture := setup(t)
	_, token := r4Operator(t, fixture)
	id, _ := fixture.assigned(t)
	fixture.event(t, id, 1, "loaded", fixture.players[0], "")
	r4Set(t, fixture, token, "draining", 1)
	fixture.app.Match.Now = func() time.Time { return time.Now().Add(time.Hour) }
	if err := fixture.app.Match.LoadingExpired(fixture.ctx, id); err != nil {
		t.Fatal(err)
	}
	if err := fixture.app.Worker.Once(fixture.ctx); err != nil {
		t.Fatal(err)
	}
	snapshot, err := control.Observe(fixture.ctx, fixture.db)
	if err != nil || !snapshot.ReadyToStop || snapshot.QueuedTickets != 0 {
		t.Fatalf("loading failure escaped drain: %+v %v", snapshot, err)
	}
}

func TestR4AuditFailureRollsBackDrainAndNeedsRepairBlocksClose(t *testing.T) {
	fixture := setup(t)
	_, token := r4Operator(t, fixture)
	plan := party.Plan{Operation: "training_ground", Difficulty: "normal", FillPolicy: "public", AllowPartial: true}
	fixture.command(t, fixture.players[0], "v2.match.enqueue", map[string]any{"plan": plan})
	if _, err := fixture.db.Exec("RENAME TABLE admin_operation_logs TO admin_operation_logs_r4_fault"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = fixture.db.Exec("RENAME TABLE admin_operation_logs_r4_fault TO admin_operation_logs") })
	status, requestErr := r4ControlRequest(fixture, token, map[string]any{"operation_id": store.ID("audit_fault"), "admission_state": "draining", "expected_version": 1, "reason": "test audit atomicity"})
	if _, err := fixture.db.Exec("RENAME TABLE admin_operation_logs_r4_fault TO admin_operation_logs"); err != nil {
		t.Fatal(err)
	}
	if requestErr != nil || status != http.StatusServiceUnavailable {
		t.Fatalf("audit fault response=%d err=%v", status, requestErr)
	}
	snapshot, err := control.Observe(fixture.ctx, fixture.db)
	if err != nil || snapshot.AdmissionState != "open" || snapshot.Version != 1 || snapshot.QueuedTickets != 1 || snapshot.ActivityLocks != 1 {
		t.Fatalf("audit failure committed drain: %+v %v", snapshot, err)
	}
	r4Set(t, fixture, token, "draining", 1)
	if err := fixture.app.Worker.Once(fixture.ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.db.Exec("INSERT INTO pve_pending_operations(operation_id,operation_type,aggregate_id,payload,status,next_attempt_at) VALUES(?,'settlement','r4_missing_run',JSON_OBJECT(),'needs_repair',UTC_TIMESTAMP(3))", store.ID("repair")); err != nil {
		t.Fatal(err)
	}
	snapshot, err = control.Observe(fixture.ctx, fixture.db)
	if err != nil || snapshot.ReadyToStop || snapshot.NeedsRepair != 1 {
		t.Fatalf("needs_repair ignored: %+v %v", snapshot, err)
	}
	status, err = r4ControlRequest(fixture, token, map[string]any{"operation_id": store.ID("close"), "admission_state": "closed", "expected_version": 2, "reason": "must repair first"})
	if err != nil || status != http.StatusConflict {
		t.Fatalf("closed with repair outstanding: status=%d err=%v", status, err)
	}
}
