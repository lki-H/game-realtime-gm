package v2_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"game-realtime-gm/backend/internal/auth"
	"game-realtime-gm/backend/internal/config"
	"game-realtime-gm/backend/internal/database"
	"game-realtime-gm/backend/internal/middleware"
	"game-realtime-gm/backend/internal/pve/party"
	"game-realtime-gm/backend/internal/pve/run"
	"game-realtime-gm/backend/internal/pve/settlement"
	"game-realtime-gm/backend/internal/pve/store"
	"game-realtime-gm/backend/internal/router"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"strings"
)

func TestIndividualSettlementCanRetryWithoutRepeatingOtherRewards(t *testing.T) {
	fixture := setup(t)
	id, _ := fixture.assigned(t)
	for index, player := range fixture.players {
		fixture.event(t, id, int64(index+1), "loaded", player, "")
	}
	for index := 0; index < 3; index++ {
		fixture.event(t, id, int64(index+5), "kill", fixture.players[0], store.ID("enemy"))
	}
	fixture.event(t, id, 8, "interact", fixture.players[1], "terminal")
	fixture.event(t, id, 9, "reach", fixture.players[2], "exit")
	if _, err := fixture.db.ExecContext(fixture.ctx, "DELETE FROM player_assets WHERE player_id=?", fixture.players[0]); err != nil {
		t.Fatal(err)
	}
	if err := fixture.app.Settlement.Settle(fixture.ctx, id); !errors.Is(err, settlement.ErrSettlementPending) {
		t.Fatalf("partial settlement: %v", err)
	}
	if err := fixture.app.Settlement.FinalizeRun(fixture.ctx, id); !errors.Is(err, settlement.ErrSettlementPending) {
		t.Fatalf("premature finalization: %v", err)
	}
	for index, player := range fixture.players[1:] {
		var balance int64
		if err := fixture.db.QueryRowContext(fixture.ctx, "SELECT soft_currency FROM player_assets WHERE player_id=?", player).Scan(&balance); err != nil {
			t.Fatal(err)
		}
		if balance != []int64{140, 125, 100}[index] {
			t.Fatalf("healthy participant blocked: balance=%d", balance)
		}
	}
	var locks int
	if err := fixture.db.QueryRowContext(fixture.ctx, "SELECT COUNT(*) FROM pve_player_activity_locks WHERE activity_id=?", id).Scan(&locks); err != nil || locks != 1 {
		t.Fatalf("unsettled participant lock count=%d err=%v", locks, err)
	}
	if _, err := fixture.db.ExecContext(fixture.ctx, "INSERT INTO player_assets(player_id,soft_currency) VALUES(?,0)", fixture.players[0]); err != nil {
		t.Fatal(err)
	}
	if err := fixture.app.Settlement.SettleParticipant(fixture.ctx, id, fixture.players[0]); err != nil {
		t.Fatal(err)
	}
	for _, player := range fixture.players[2:] {
		if err := fixture.app.Settlement.SettleParticipant(fixture.ctx, id, player); err != nil {
			t.Fatal(err)
		}
	}
	if err := fixture.app.Settlement.FinalizeRun(fixture.ctx, id); err != nil {
		t.Fatal(err)
	}
	if err := fixture.app.Settlement.Settle(fixture.ctx, id); err != nil {
		t.Fatal(err)
	}
	state, err := fixture.app.Runs.Snapshot(fixture.ctx, id)
	if err != nil || !state.Settled || state.Status != "closed" {
		t.Fatalf("final state=%+v err=%v", state, err)
	}
	var grants int
	if err := fixture.db.QueryRowContext(fixture.ctx, "SELECT COUNT(*) FROM pve_reward_grants WHERE run_id=?", id).Scan(&grants); err != nil {
		t.Fatal(err)
	}
	if grants != 7 {
		t.Fatalf("expected 7 grants got %d", grants)
	}
}

func activityView(t *testing.T, fixture *fixture, player int64) map[string]any {
	t.Helper()
	token, err := auth.GenerateToken(fixture.secret, player, "snapshot")
	if err != nil {
		t.Fatal(err)
	}
	request, _ := http.NewRequest("GET", fixture.server.URL+"/api/v2/me/activity", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var envelope struct {
		Code int            `json:"code"`
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil || response.StatusCode != 200 || envelope.Code != 0 {
		t.Fatalf("activity status=%d err=%v", response.StatusCode, err)
	}
	return envelope.Data
}

func TestActivityPartyWithoutLockAndQueuedTicket(t *testing.T) {
	fixture := setup(t)
	created := fixture.command(t, fixture.players[0], "v2.party.create", map[string]any{})
	var group party.Party
	if err := json.Unmarshal(created, &group); err != nil {
		t.Fatal(err)
	}
	view := activityView(t, fixture, fixture.players[0])
	if view["party"] == nil || view["activity_lock"] != nil {
		t.Fatalf("idle party lost: %+v", view)
	}
	fixture.command(t, fixture.players[0], "v2.party.ready", map[string]any{"party_id": group.ID, "ready": true, "roster_version": group.RosterVersion, "plan_version": group.PlanVersion, "selection_version": 1})
	fixture.command(t, fixture.players[0], "v2.match.enqueue", map[string]any{"party_id": group.ID})
	view = activityView(t, fixture, fixture.players[0])
	if view["ticket"] == nil || view["activity_lock"] == nil || view["party"] == nil {
		t.Fatalf("queued activity incomplete: %+v", view)
	}
}

func TestRepairRequiresOperatorAndResolvedFailure(t *testing.T) {
	fixture := setup(t)
	id, _ := fixture.assigned(t)
	for index, player := range fixture.players {
		fixture.event(t, id, int64(index+1), "loaded", player, "")
	}
	fixture.event(t, id, 5, "kill", fixture.players[0], "enemy")
	fixture.event(t, id, 6, "system_abort", fixture.players[0], "")
	if _, err := fixture.db.ExecContext(fixture.ctx, "DELETE FROM player_assets WHERE player_id=?", fixture.players[0]); err != nil {
		t.Fatal(err)
	}
	fixture.app.Worker.Tick = nil
	fixture.app.Worker.Settle = func(ctx context.Context, runID string) error {
		if runID == id {
			return errors.New("injected storage failure")
		}
		return fixture.app.Settlement.Settle(ctx, runID)
	}
	for attempt := 0; attempt < 5; attempt++ {
		if _, err := fixture.db.ExecContext(fixture.ctx, "UPDATE pve_pending_operations SET next_attempt_at=UTC_TIMESTAMP(3) WHERE aggregate_id=?", id); err != nil {
			t.Fatal(err)
		}
		if err := fixture.app.Worker.Once(fixture.ctx); err != nil {
			t.Fatal(err)
		}
	}
	var operationID string
	if err := fixture.db.QueryRowContext(fixture.ctx, "SELECT operation_id FROM pve_pending_operations WHERE aggregate_id=? AND status='needs_repair'", id).Scan(&operationID); err != nil {
		t.Fatal(err)
	}
	adminResult, err := fixture.db.ExecContext(fixture.ctx, "INSERT INTO admins(username,password_hash,display_name,role) VALUES(?,'synthetic','operator','operator')", store.ID("operator"))
	if err != nil {
		t.Fatal(err)
	}
	adminID, _ := adminResult.LastInsertId()
	t.Cleanup(func() {
		fixture.db.Exec("DELETE FROM admin_operation_logs WHERE admin_id=?", adminID)
		fixture.db.Exec("DELETE FROM admins WHERE id=?", adminID)
	})
	operatorToken, _ := auth.GenerateAdminToken(fixture.secret, adminID, "operator", "operator")
	playerToken, _ := auth.GenerateToken(fixture.secret, fixture.players[0], "player")
	input := map[string]any{"operation_id": store.ID("repair"), "expected_attempts": 5, "reason": "storage restored and verified"}
	send := func(token string) int {
		request, _ := http.NewRequest("POST", fixture.server.URL+"/api/admin/v2/operations/"+operationID+"/retry", bytes.NewReader(store.JSON(input)))
		request.Header.Set("Authorization", "Bearer "+token)
		request.Header.Set("Content-Type", "application/json")
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		return response.StatusCode
	}
	if status := send(playerToken); status != 403 {
		t.Fatalf("player repair status=%d", status)
	}
	fixture.db.ExecContext(fixture.ctx, "UPDATE admins SET role='gm' WHERE id=?", adminID)
	if status := send(operatorToken); status != 403 {
		t.Fatalf("old elevated token status=%d", status)
	}
	fixture.db.ExecContext(fixture.ctx, "UPDATE admins SET role='operator' WHERE id=?", adminID)
	if status := send(operatorToken); status != 409 {
		t.Fatalf("unresolved repair status=%d", status)
	}
	if _, err := fixture.db.ExecContext(fixture.ctx, "INSERT INTO player_assets(player_id) VALUES(?)", fixture.players[0]); err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		if status := send(operatorToken); status != 200 {
			t.Fatalf("resolved repair status=%d", status)
		}
	}
	fixture.app.Worker.Settle = fixture.app.Settlement.Settle
	if err := fixture.app.Worker.Once(fixture.ctx); err != nil {
		t.Fatal(err)
	}
	state, err := fixture.app.Runs.Snapshot(fixture.ctx, id)
	if err != nil || !state.Settled {
		t.Fatalf("repair did not converge: %v", err)
	}
	var audits int
	if err := fixture.db.QueryRowContext(fixture.ctx, "SELECT COUNT(*) FROM admin_operation_logs WHERE admin_id=? AND action='admin.v2.settlement.retry'", adminID).Scan(&audits); err != nil || audits != 1 {
		t.Fatalf("audit count=%d err=%v", audits, err)
	}
}

func TestDisabledRuleBlocksAdmissionButPreservesRunSnapshot(t *testing.T) {
	fixture := setup(t)
	id, _ := fixture.assigned(t)
	if _, err := fixture.db.ExecContext(fixture.ctx, "UPDATE pve_rule_versions SET status='disabled'"); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.app.Runs.Create(fixture.ctx, run.CreateRequest{OperationName: "training_ground", Difficulty: "normal", PlayerIDs: []int64{fixture.players[0]}}); !errors.Is(err, store.Conflict) {
		t.Fatalf("disabled admission err=%v", err)
	}
	for index, player := range fixture.players {
		fixture.event(t, id, int64(index+1), "loaded", player, "")
	}
	state, err := fixture.app.Runs.Snapshot(fixture.ctx, id)
	if err != nil || state.Status != "running" {
		t.Fatalf("disabled existing run status=%+v err=%v", state, err)
	}
	if err := fixture.app.Publish(fixture.ctx); err != nil {
		t.Fatal(err)
	}
	var status string
	fixture.db.QueryRowContext(fixture.ctx, "SELECT status FROM pve_rule_versions").Scan(&status)
	if status != "disabled" {
		t.Fatal("restart reenabled disabled rule")
	}
	fixture.app.Runs.Rules.SuccessReward++
	if err := fixture.app.Publish(fixture.ctx); !errors.Is(err, store.Conflict) {
		t.Fatalf("published content mutation err=%v", err)
	}
}

func TestWebSocketCommandFloodAndHandshakeLimit(t *testing.T) {
	fixture := setup(t)
	cfg := config.Load()
	cfg.GameplayMode = "v2"
	cfg.JWTSecret = fixture.secret
	cfg.HTTP.WebSocketRateLimit = 1
	cfg.HTTP.WebSocketCommandRateLimit = 1
	server := httptest.NewServer(router.New(fixture.ctx, fixture.db, fixture.cache, cfg, fixture.app))
	defer server.Close()
	fixture.cache.Del(fixture.ctx, "ratelimit:ws:127.0.0.1")
	token, _ := auth.GenerateToken(fixture.secret, fixture.players[0], "ws-limit")
	connection, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/ws?token="+token, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	connection.SetReadDeadline(time.Now().Add(5 * time.Second))
	var response map[string]any
	if err := connection.ReadJSON(&response); err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		if err := connection.WriteJSON(map[string]any{"schema_version": 2, "type": "v2.party.create", "operation_id": store.ID("command"), "data": map[string]any{}}); err != nil {
			t.Fatal(err)
		}
		if err := connection.ReadJSON(&response); err != nil {
			t.Fatal(err)
		}
		if attempt == 1 && response["code"] != float64(42970) {
			t.Fatalf("command flood reply=%+v", response)
		}
	}
	second, handshake, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/ws?token="+token, nil)
	if second != nil {
		second.Close()
	}
	if handshake != nil {
		defer handshake.Body.Close()
	}
	if err == nil || handshake == nil || handshake.StatusCode != 429 {
		t.Fatalf("handshake flood status=%+v err=%v", handshake, err)
	}
}

func TestUnifiedActivitySnapshot(t *testing.T) {
	fixture := setup(t)
	id, _ := fixture.assigned(t)
	token, err := auth.GenerateToken(fixture.secret, fixture.players[0], "snapshot")
	if err != nil {
		t.Fatal(err)
	}
	request, _ := http.NewRequest("GET", fixture.server.URL+"/api/v2/me/activity", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatalf("activity status=%d", response.StatusCode)
	}
	var envelope struct {
		Code int            `json:"code"`
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Code != 0 || envelope.Data["activity_lock"] == nil || envelope.Data["run"] == nil {
		t.Fatalf("incomplete activity view: %+v", envelope.Data)
	}
	state := envelope.Data["run"].(map[string]any)
	if state["id"] != id {
		t.Fatal("different run recovered")
	}
	for _, item := range state["participants"].([]any) {
		member := item.(map[string]any)
		if member["player_id"] != float64(fixture.players[0]) && (member["task_key"] != nil || member["task_attempt_id"] != nil || member["task_progress"] != nil) {
			t.Fatal("teammate task leaked")
		}
	}
}

func TestMigrationLedgerCanBeReconciledTwice(t *testing.T) {
	fixture := setup(t)
	migrationPath := filepath.Join("..", "..", "internal", "database", "migrations", "day37_v2_pve_foundation.sql")
	var status string
	if err := fixture.db.QueryRowContext(fixture.ctx, "SELECT status FROM schema_migrations WHERE migration_id=?", "day37_v2_pve_foundation").Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "reconciled" && status != "applied" {
		t.Fatal(status)
	}
	repeated, err := database.ApplyV2Foundation(fixture.ctx, fixture.db, migrationPath)
	if err != nil || repeated != status {
		t.Fatalf("repeat migration status=%s err=%v", repeated, err)
	}
	tamperedPath := filepath.Join(t.TempDir(), "tampered.sql")
	migration, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tamperedPath, append(migration, []byte("\n-- checksum test\n")...), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ApplyV2Foundation(fixture.ctx, fixture.db, tamperedPath); err == nil {
		t.Fatal("tampered migration was accepted")
	}
}

func TestResourceLimitsRejectOversizedBodyAndRateFlood(t *testing.T) {
	fixture := setup(t)
	gin.SetMode(gin.TestMode)
	bodyRouter := gin.New()
	bodyRouter.Use(middleware.BodyLimit(4))
	bodyRouter.POST("/", func(c *gin.Context) {
		if _, err := io.ReadAll(c.Request.Body); err != nil {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"code": 41370})
			return
		}
		c.Status(http.StatusNoContent)
	})
	bodyRequest := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString("too-large"))
	bodyResponse := httptest.NewRecorder()
	bodyRouter.ServeHTTP(bodyResponse, bodyRequest)
	if bodyResponse.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized body status=%d", bodyResponse.Code)
	}

	rateRouter := gin.New()
	rateRouter.Use(middleware.RedisRateLimit(fixture.cache, "r1-rate-"+store.ID("key"), 1, time.Minute))
	rateRouter.POST("/", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	for index := 0; index < 2; index++ {
		rateRequest := httptest.NewRequest(http.MethodPost, "/", nil)
		rateRequest.RemoteAddr = "192.0.2.1:1234"
		rateResponse := httptest.NewRecorder()
		rateRouter.ServeHTTP(rateResponse, rateRequest)
		if index == 0 && rateResponse.Code != http.StatusNoContent {
			t.Fatalf("first rate limited request status=%d", rateResponse.Code)
		}
		if index == 1 && rateResponse.Code != http.StatusTooManyRequests {
			t.Fatalf("flooded request status=%d", rateResponse.Code)
		}
	}
}

func TestLoginLimitIgnoresUntrustedForwardedAddress(t *testing.T) {
	fixture := setup(t)
	cfg := config.Load()
	cfg.GameplayMode = "v2"
	cfg.JWTSecret = fixture.secret
	cfg.HTTP.AuthRateLimit = 1
	server := httptest.NewServer(router.New(fixture.ctx, fixture.db, fixture.cache, cfg, fixture.app))
	defer server.Close()
	fixture.cache.Del(fixture.ctx, "ratelimit:login:127.0.0.1")
	for attempt := 0; attempt < 2; attempt++ {
		request, _ := http.NewRequest("POST", server.URL+"/api/login", strings.NewReader("{}"))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Forwarded-For", []string{"192.0.2.1", "192.0.2.2"}[attempt])
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if attempt == 0 && response.StatusCode != 400 {
			t.Fatalf("first login status=%d", response.StatusCode)
		}
		if attempt == 1 && response.StatusCode != 429 {
			t.Fatalf("forwarded IP bypass status=%d", response.StatusCode)
		}
	}
	key := "v2:ratelimit:test:" + store.ID("key")
	if err := fixture.cache.Set(fixture.ctx, key, 1, 0).Err(); err != nil {
		t.Fatal(err)
	}
	if _, err := middleware.TakeRateLimit(fixture.ctx, fixture.cache, key, 5, time.Minute); err != nil {
		t.Fatal(err)
	}
	if ttl, err := fixture.cache.PTTL(fixture.ctx, key).Result(); err != nil || ttl <= 0 {
		t.Fatalf("missing rate limit expiry ttl=%s err=%v", ttl, err)
	}
}
