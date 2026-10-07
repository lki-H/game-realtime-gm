package v2_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"game-realtime-gm/backend/internal/auth"
	"game-realtime-gm/backend/internal/database"
	"game-realtime-gm/backend/internal/pve/run"
	"game-realtime-gm/backend/internal/pve/store"
	"game-realtime-gm/backend/internal/pve/worker"
	"github.com/gorilla/websocket"
)

func TestAuditDeletedAdminCannotAccessV2(t *testing.T) {
	fixture := setup(t)
	result, err := fixture.db.Exec("INSERT INTO admins(username,password_hash,display_name,role) VALUES(?,'synthetic','audit','gm')", store.ID("admin_audit"))
	if err != nil {
		t.Fatal(err)
	}
	id, _ := result.LastInsertId()
	token, err := auth.GenerateAdminToken(fixture.secret, id, "audit", "gm")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.db.Exec("DELETE FROM admins WHERE id=?", id); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/admin/v2/observations/runs", "/api/admin/v2/metrics", "/api/admin/players"} {
		request, _ := http.NewRequest("GET", fixture.server.URL+path, nil)
		request.Header.Set("Authorization", "Bearer "+token)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusForbidden {
			t.Fatalf("deleted admin access %s status=%d", path, response.StatusCode)
		}
	}
}

func TestAuditWorkerReportsDurableAcknowledgementFailure(t *testing.T) {
	fixture := setup(t)
	if _, err := fixture.db.Exec("INSERT INTO pve_pending_operations(operation_id,operation_type,aggregate_id,payload,next_attempt_at) VALUES('audit_pending','settlement','audit_run',JSON_OBJECT(),UTC_TIMESTAMP(3))"); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = fixture.db.Exec("RENAME TABLE pve_pending_operations_audit TO pve_pending_operations") }()
	processor := worker.Worker{DB: fixture.db, Settle: func(ctx context.Context, runID string) error {
		_, err := fixture.db.ExecContext(ctx, "RENAME TABLE pve_pending_operations TO pve_pending_operations_audit")
		return err
	}}
	if err := processor.Once(fixture.ctx); err == nil {
		t.Fatal("worker reported success without persisting done state")
	}
	if _, err := fixture.db.Exec("RENAME TABLE pve_pending_operations_audit TO pve_pending_operations"); err != nil {
		t.Fatal(err)
	}
	processor.Settle = func(context.Context, string) error { return nil }
	if err := processor.Once(fixture.ctx); err != nil {
		t.Fatal(err)
	}
	var status string
	if err := fixture.db.QueryRow("SELECT status FROM pve_pending_operations WHERE operation_id='audit_pending'").Scan(&status); err != nil || status != "done" {
		t.Fatalf("worker recovery status=%s err=%v", status, err)
	}
}

func TestAuditSessionOutageDoesNotImpersonateRevocation(t *testing.T) {
	fixture := setup(t)
	token, err := auth.GenerateToken(fixture.secret, fixture.players[0], "audit-player")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.db.Exec("RENAME TABLE players TO players_audit"); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = fixture.db.Exec("RENAME TABLE players_audit TO players") }()
	request, _ := http.NewRequest("GET", fixture.server.URL+"/api/v2/me/activity", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("session storage outage status=%d", response.StatusCode)
	}
}

func TestAuditMigrationRejectsColumnDrift(t *testing.T) {
	fixture := setup(t)
	for _, column := range []string{"VARCHAR(64) NOT NULL", "BIGINT NULL"} {
		if _, err := fixture.db.Exec("ALTER TABLE pve_run_archives MODIFY final_sequence " + column); err != nil {
			t.Fatal(err)
		}
		_, inspectionError := database.ApplyV2Engineering(fixture.ctx, fixture.db, "../../internal/database/migrations/day39_v2_engineering.sql")
		if _, err := fixture.db.Exec("ALTER TABLE pve_run_archives MODIFY final_sequence BIGINT NOT NULL"); err != nil {
			t.Fatal(err)
		}
		if inspectionError == nil {
			t.Fatalf("migration accepted column drift: %s", column)
		}
	}
}

func TestAuditReconnectWaitsForPriorReadinessCleanup(t *testing.T) {
	fixture := setup(t)
	player := fixture.players[0]
	token, err := auth.GenerateToken(fixture.secret, player, "audit-ws")
	if err != nil {
		t.Fatal(err)
	}
	address := "ws" + strings.TrimPrefix(fixture.server.URL, "http") + "/ws?token=" + token
	previous, _, err := websocket.DefaultDialer.Dial(address, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer previous.Close()
	var welcome any
	if err := previous.ReadJSON(&welcome); err != nil {
		t.Fatal(err)
	}
	data := fixture.command(t, player, "v2.party.create", map[string]any{})
	var group struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(data, &group); err != nil {
		t.Fatal(err)
	}
	partyID := group.ID
	blocking, err := fixture.db.BeginTx(fixture.ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer blocking.Rollback()
	if _, err := blocking.Exec("UPDATE pve_party_members SET ready=1 WHERE party_id=?", partyID); err != nil {
		t.Fatal(err)
	}
	previous.Close()
	deadline := time.Now().Add(3 * time.Second)
	waiting := false
	for time.Now().Before(deadline) {
		var count int
		if err := fixture.db.QueryRow("SELECT COUNT(*) FROM information_schema.processlist WHERE INFO LIKE 'UPDATE pve_party_members SET ready=0 WHERE player_id=%'").Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count > 0 {
			waiting = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !waiting {
		t.Fatal("prior disconnect did not reach cleanup")
	}
	replacement, _, err := websocket.DefaultDialer.Dial(address, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer replacement.Close()
	welcomed := make(chan error, 1)
	go func() { var response any; welcomed <- replacement.ReadJSON(&response) }()
	select {
	case err := <-welcomed:
		t.Fatalf("new connection acknowledged before old cleanup finished: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	if err := blocking.Commit(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-welcomed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("replacement never acknowledged")
	}
	requestWS(t, replacement, "v2.party.ready", map[string]any{"party_id": partyID, "ready": true, "roster_version": 1, "plan_version": 1, "selection_version": 1})
	var ready bool
	if err := fixture.db.QueryRow("SELECT ready FROM pve_party_members WHERE party_id=? AND player_id=?", partyID, player).Scan(&ready); err != nil || !ready {
		t.Fatalf("replacement readiness lost: %v", err)
	}
}
