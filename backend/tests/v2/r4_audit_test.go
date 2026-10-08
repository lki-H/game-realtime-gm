package v2_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"game-realtime-gm/backend/internal/auth"
	"game-realtime-gm/backend/internal/database"
	"game-realtime-gm/backend/internal/leaderboard"
	"game-realtime-gm/backend/internal/observation"
	"game-realtime-gm/backend/internal/pve/run"
	"game-realtime-gm/backend/internal/pve/store"
)

func TestR4AuditLegacyHistoryKeepsOriginalTimezone(t *testing.T) {
	fixture := setup(t)
	player := fixture.players[0]
	result, err := fixture.db.Exec("INSERT INTO mission_records(mission_instance_id,mission_id,squad_id,submitted_by_player_id,nonce,idempotency_key,completion_seconds,score,created_at) VALUES(?,'legacy_time','legacy_group',?,?,?,10,50,'2026-08-23 10:48:12.123')", store.ID("legacy"), player, store.ID("nonce"), store.ID("intent"))
	if err != nil {
		t.Fatal(err)
	}
	recordID, err := result.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.db.Exec("INSERT INTO reward_records(mission_record_id,mission_instance_id,player_id,reward_type,amount) SELECT id,mission_instance_id,?,'soft_currency',100 FROM mission_records WHERE id=?", player, recordID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = fixture.db.Exec("DELETE FROM reward_records WHERE mission_record_id=?", recordID)
		_, _ = fixture.db.Exec("DELETE FROM mission_records WHERE id=?", recordID)
	})
	history, err := leaderboard.NewService(fixture.db, fixture.cache).ListHistory(fixture.ctx, player, 1, 10)
	if err != nil || len(history.Items) != 1 {
		t.Fatalf("legacy history unavailable: %+v %v", history, err)
	}
	expected := time.Date(2026, 8, 23, 2, 48, 12, 123000000, time.UTC)
	if !history.Items[0].SettledAt.Equal(expected) {
		t.Fatalf("legacy time shifted: %s", history.Items[0].SettledAt.Format(time.RFC3339Nano))
	}
	page, err := observation.NewService(fixture.db, nil, nil, nil, nil).ListSettlements(fixture.ctx, 1, 10, "legacy_time", player)
	if err != nil || len(page.Items) != 1 || !page.Items[0].CreatedAt.Equal(expected) {
		t.Fatalf("GM legacy time shifted: %+v %v", page, err)
	}
}

func TestR4AuditRetryRechecksRoleAfterPendingLock(t *testing.T) {
	fixture := setup(t)
	adminID, token := r4Operator(t, fixture)
	id, _ := fixture.assigned(t)
	if err := fixture.app.Recover(fixture.ctx); err != nil {
		t.Fatal(err)
	}
	operationID := "settle:" + id
	if _, err := fixture.db.Exec("UPDATE pve_pending_operations SET status='needs_repair',attempts=5 WHERE operation_id=?", operationID); err != nil {
		t.Fatal(err)
	}
	blocking, err := fixture.db.BeginTx(fixture.ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer blocking.Rollback()
	if _, err := blocking.Exec("UPDATE pve_pending_operations SET attempts=5 WHERE operation_id=?", operationID); err != nil {
		t.Fatal(err)
	}
	statuses := make(chan int, 1)
	requestErrors := make(chan error, 1)
	input := store.JSON(map[string]any{"operation_id": store.ID("repair"), "expected_attempts": 5, "reason": "audit role change during request"})
	go func() {
		request, err := http.NewRequestWithContext(fixture.ctx, "POST", fixture.server.URL+"/api/admin/v2/operations/"+operationID+"/retry", bytes.NewReader(input))
		if err != nil {
			requestErrors <- err
			return
		}
		request.Header.Set("Authorization", "Bearer "+token)
		request.Header.Set("Content-Type", "application/json")
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			requestErrors <- err
			return
		}
		response.Body.Close()
		statuses <- response.StatusCode
	}()
	deadline := time.Now().Add(3 * time.Second)
	waiting := false
	for time.Now().Before(deadline) {
		var count int
		if err := fixture.db.QueryRow("SELECT COUNT(*) FROM information_schema.processlist WHERE INFO LIKE 'SELECT aggregate_id,operation_type,status,attempts FROM pve_pending_operations%' AND ID<>CONNECTION_ID()").Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count > 0 {
			waiting = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !waiting {
		t.Fatal("repair never reached the blocked pending row")
	}
	if _, err := fixture.db.Exec("UPDATE admins SET role='gm' WHERE id=?", adminID); err != nil {
		t.Fatal(err)
	}
	if err := blocking.Commit(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-requestErrors:
		t.Fatal(err)
	case status := <-statuses:
		if status != http.StatusForbidden {
			t.Fatalf("revoked role still repaired: status=%d", status)
		}
	case <-fixture.ctx.Done():
		t.Fatal("repair request did not finish")
	}
	var state string
	if err := fixture.db.QueryRow("SELECT status FROM pve_pending_operations WHERE operation_id=?", operationID).Scan(&state); err != nil || state != "needs_repair" {
		t.Fatalf("unauthorized repair altered work: state=%s err=%v", state, err)
	}
}

func TestR4AuditProtocolAndHistoryRoutes(t *testing.T) {
	fixture := setup(t)
	_, adminToken := r4Operator(t, fixture)
	playerToken, err := auth.GenerateToken(fixture.secret, fixture.players[0], "audit")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/admin/realtime/summary", "/api/admin/realtime/players/1"} {
		request, _ := http.NewRequest("GET", fixture.server.URL+path, nil)
		request.Header.Set("Authorization", "Bearer "+adminToken)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusNotFound {
			t.Fatalf("legacy live route %s should be retired: %d", path, response.StatusCode)
		}
	}
	request, _ := http.NewRequest("GET", fixture.server.URL+"/api/me/mission-records", nil)
	request.Header.Set("Authorization", "Bearer "+playerToken)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var envelope struct {
		Code int `json:"code"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil || response.StatusCode != 200 || envelope.Code != 0 {
		t.Fatal("legacy history retired with live routes", err)
	}
}

func TestR4AuditLostDatabaseOwnershipStopsWatcher(t *testing.T) {
	fixture := setup(t)
	owner, err := fixture.db.Conn(fixture.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	var acquired int
	if err := owner.QueryRowContext(fixture.ctx, "SELECT GET_LOCK(CONCAT('gm-gameplay:',DATABASE()),0)").Scan(&acquired); err != nil || acquired != 1 {
		t.Fatalf("test ownership unavailable: %d %v", acquired, err)
	}
	defer func() {
		_, _ = owner.ExecContext(context.Background(), "SELECT RELEASE_LOCK(CONCAT('gm-gameplay:',DATABASE()))")
	}()
	ctx, cancel := context.WithCancel(fixture.ctx)
	defer cancel()
	lost := make(chan struct{})
	exited := make(chan struct{})
	go func() {
		defer close(exited)
		database.WatchGameplayOwner(ctx, owner, 20*time.Millisecond, func() { close(lost) })
	}()
	if _, err := owner.ExecContext(fixture.ctx, "SELECT RELEASE_LOCK(CONCAT('gm-gameplay:',DATABASE()))"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-lost:
	case <-time.After(3 * time.Second):
		t.Fatal("ownership loss was not detected")
	}
	select {
	case <-exited:
	case <-time.After(time.Second):
		t.Fatal("watcher did not stop after loss")
	}
}

func TestR4AuditConcurrentRepairRetriesStayIdempotent(t *testing.T) {
	fixture := setup(t)
	adminID, token := r4Operator(t, fixture)
	id, _ := fixture.assigned(t)
	if err := fixture.app.Recover(fixture.ctx); err != nil {
		t.Fatal(err)
	}
	target := "settle:" + id
	if _, err := fixture.db.Exec("UPDATE pve_pending_operations SET status='needs_repair',attempts=5 WHERE operation_id=?", target); err != nil {
		t.Fatal(err)
	}
	blocking, err := fixture.db.BeginTx(fixture.ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer blocking.Rollback()
	if _, err := blocking.Exec("UPDATE pve_pending_operations SET attempts=5 WHERE operation_id=?", target); err != nil {
		t.Fatal(err)
	}
	input := store.JSON(map[string]any{"operation_id": store.ID("repair"), "expected_attempts": 5, "reason": "concurrent retry"})
	results := make(chan int, 2)
	for attempt := 0; attempt < 2; attempt++ {
		go func() {
			request, _ := http.NewRequestWithContext(fixture.ctx, "POST", fixture.server.URL+"/api/admin/v2/operations/"+target+"/retry", bytes.NewReader(input))
			request.Header.Set("Authorization", "Bearer "+token)
			request.Header.Set("Content-Type", "application/json")
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				results <- 0
				return
			}
			response.Body.Close()
			results <- response.StatusCode
		}()
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		var count int
		if err := fixture.db.QueryRow("SELECT COUNT(*) FROM information_schema.processlist WHERE INFO LIKE 'SELECT aggregate_id,operation_type,status,attempts FROM pve_pending_operations%' AND ID<>CONNECTION_ID()").Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("concurrent repair requests did not wait on target lock")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := blocking.Commit(); err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		select {
		case status := <-results:
			if status != 200 {
				t.Fatalf("concurrent retry status=%d", status)
			}
		case <-fixture.ctx.Done():
			t.Fatal("concurrent retry timed out")
		}
	}
	var audits int
	if err := fixture.db.QueryRow("SELECT COUNT(*) FROM admin_operation_logs WHERE admin_id=? AND action='admin.v2.settlement.retry'", adminID).Scan(&audits); err != nil || audits != 1 {
		t.Fatalf("duplicate repair audits=%d err=%v", audits, err)
	}
}

func TestR4AuditRunMaintenanceDoesNotStarveLaterRuns(t *testing.T) {
	fixture := setup(t)
	var lastID string
	for index := 0; index < 101; index++ {
		state, err := fixture.app.Runs.Create(fixture.ctx, run.CreateRequest{OperationName: "training_ground", Difficulty: "normal", PlayerIDs: fixture.players[:1]})
		if err != nil {
			t.Fatal(err)
		}
		state.Status = "running"
		state.Participants[0].Status = "participating"
		state.DeadlineAt = time.Now().Add(time.Hour)
		if _, err := fixture.db.Exec("UPDATE pve_runs SET status='running',state=?,created_at=?,updated_at='2026-01-01 00:00:00' WHERE id=?", store.JSON(state), time.Date(2026, 1, 1, 0, 0, index, 0, time.UTC), state.ID); err != nil {
			t.Fatal(err)
		}
		lastID = state.ID
	}
	for iteration := 0; iteration < 2; iteration++ {
		if err := fixture.app.Tick(fixture.ctx); err != nil {
			t.Fatal(err)
		}
	}
	var updated time.Time
	if err := fixture.db.QueryRow("SELECT updated_at FROM pve_runs WHERE id=?", lastID).Scan(&updated); err != nil || updated.Year() == 2026 && updated.Month() == time.January {
		t.Fatalf("later Run never received maintenance: updated=%s err=%v", updated, err)
	}
}
