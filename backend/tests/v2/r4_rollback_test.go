package v2_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"game-realtime-gm/backend/internal/auth"
	"game-realtime-gm/backend/internal/pve/control"
	"game-realtime-gm/backend/internal/pve/run"
	"game-realtime-gm/backend/internal/pve/store"
	"github.com/gorilla/websocket"
)

func TestR4LegacyCacheCleanupAndIdleGuard(t *testing.T) {
	fixture := setup(t)
	id, _ := fixture.assigned(t)
	if err := control.RequireLegacyIdle(fixture.ctx, fixture.db); err == nil {
		t.Fatal("legacy startup allowed during V2 loading")
	}
	for index, player := range fixture.players {
		fixture.event(t, id, int64(index+1), "loaded", player, "")
	}
	if err := fixture.app.Recover(fixture.ctx); err != nil {
		t.Fatal(err)
	}
	if err := fixture.app.Worker.Once(fixture.ctx); err != nil {
		t.Fatal(err)
	}
	if err := control.RequireLegacyIdle(fixture.ctx, fixture.db); err != nil {
		t.Fatal(err)
	}
	oldKeys := []string{"matchmaking:queue:r4_test", "matchmaking:ticket:r4_test", "matchmaking:player:r4_test", "matchmaking:timeouts"}
	preserved := []string{"v2:r4_preserved", "online:r4_preserved", "leaderboard:{r4}:scores", "matchmaking:other:r4_test"}
	for _, key := range append(oldKeys, preserved...) {
		if err := fixture.cache.Set(fixture.ctx, key, "fixture", time.Minute).Err(); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { _ = fixture.cache.Del(context.Background(), append(oldKeys, preserved...)...).Err() })
	keys, err := control.LegacyCacheKeys(fixture.ctx, fixture.cache)
	if err != nil || len(keys) != len(oldKeys) {
		t.Fatalf("legacy scan keys=%v err=%v", keys, err)
	}
	removed, err := control.RemoveLegacyCache(fixture.ctx, fixture.cache, keys)
	if err != nil || removed != int64(len(oldKeys)) {
		t.Fatalf("legacy removal=%d err=%v", removed, err)
	}
	for _, key := range preserved {
		if count, err := fixture.cache.Exists(fixture.ctx, key).Result(); err != nil || count != 1 {
			t.Fatalf("shared key lost: %s %v", key, err)
		}
	}
	if _, err := control.RemoveLegacyCache(fixture.ctx, fixture.cache, []string{preserved[0]}); err == nil {
		t.Fatal("cleanup accepted a V2 key")
	}
}

func TestR4ActualBinarySwitchAndRollback(t *testing.T) {
	currentBinary, baselineBinary := os.Getenv("PVE_R4_SERVER_BINARY"), os.Getenv("PVE_R3_SERVER_BINARY")
	if currentBinary == "" || baselineBinary == "" {
		t.Skip("set both server binary paths for an actual R4/R3 rollback rehearsal")
	}
	fixture := setup(t)
	_, operatorToken := r4Operator(t, fixture)
	id, _ := fixture.assigned(t)
	for index, player := range fixture.players {
		fixture.event(t, id, int64(index+1), "loaded", player, "")
	}
	for index := 0; index < 3; index++ {
		fixture.event(t, id, int64(index+5), "kill", fixture.players[index], store.ID("enemy"))
	}
	fixture.event(t, id, 8, "interact", fixture.players[1], "terminal")
	fixture.event(t, id, 9, "reach", fixture.players[2], "exit")
	if err := fixture.app.Worker.Once(fixture.ctx); err != nil {
		t.Fatal(err)
	}
	r4Set(t, fixture, operatorToken, "draining", 1)
	r4Set(t, fixture, operatorToken, "closed", 2)
	if _, err := fixture.db.Exec("UPDATE pve_rule_versions SET status='disabled'"); err != nil {
		t.Fatal(err)
	}
	var amountBefore, grantsBefore int64
	if err := fixture.db.QueryRow("SELECT SUM(amount),COUNT(*) FROM pve_reward_grants WHERE run_id=?", id).Scan(&amountBefore, &grantsBefore); err != nil || amountBefore <= 0 {
		t.Fatal("rollback requires nonempty rewarded facts", err)
	}
	var eventData []byte
	if err := fixture.db.QueryRow("SELECT payload FROM pve_run_events WHERE run_id=? AND sequence_no=1", id).Scan(&eventData); err != nil {
		t.Fatal(err)
	}
	var event run.Event
	if err := json.Unmarshal(eventData, &event); err != nil {
		t.Fatal(err)
	}
	fixture.server.Close()
	for _, binary := range []string{currentBinary, baselineBinary} {
		if _, err := fixture.db.Exec("UPDATE pve_pending_operations SET status='pending',attempts=0,next_attempt_at=UTC_TIMESTAMP(3) WHERE operation_id=?", "settle:"+id); err != nil {
			t.Fatal(err)
		}
		address, internalAddress, eventToken, stop := r4StartServer(t, fixture, binary, binary == currentBinary)
		playerToken, err := auth.GenerateToken(fixture.secret, fixture.players[0], "rollback-player")
		if err != nil {
			stop()
			t.Fatal(err)
		}
		for _, path := range []string{"/api/v2/runs/" + id + "/results", "/api/me/mission-records", "/api/v2/me/activity"} {
			request, _ := http.NewRequest("GET", address+path, nil)
			request.Header.Set("Authorization", "Bearer "+playerToken)
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				stop()
				t.Fatal(err)
			}
			response.Body.Close()
			if response.StatusCode != http.StatusOK {
				stop()
				t.Fatalf("rollback query %s status=%d", path, response.StatusCode)
			}
		}
		connection, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(address, "http")+"/ws", http.Header{"Authorization": []string{"Bearer " + playerToken}})
		if err != nil {
			stop()
			t.Fatal(err)
		}
		_ = connection.SetReadDeadline(time.Now().Add(5 * time.Second))
		var welcome map[string]any
		if err := connection.ReadJSON(&welcome); err != nil || welcome["schema_version"] != float64(2) {
			connection.Close()
			stop()
			t.Fatal("binary did not use v2", err)
		}
		if err := connection.WriteJSON(map[string]any{"type": "mission.finish", "data": map[string]any{}}); err != nil {
			t.Fatal(err)
		}
		var rejection map[string]any
		if err := connection.ReadJSON(&rejection); err != nil || rejection["code"] != float64(40971) {
			t.Fatal("legacy finish accepted by rollback", err)
		}
		connection.Close()
		request, err := http.NewRequest("POST", internalAddress+"/internal/v2/runs/"+id+"/events", bytes.NewReader(store.JSON(event)))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-PVE-Test-Token", eventToken)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		var duplicate struct {
			Data run.EventResult `json:"data"`
		}
		decodeErr := json.NewDecoder(response.Body).Decode(&duplicate)
		response.Body.Close()
		if decodeErr != nil || response.StatusCode != 200 || !duplicate.Data.Duplicate {
			t.Fatalf("actual rollback process lost event dedup: status=%d err=%v", response.StatusCode, decodeErr)
		}
		deadline := time.Now().Add(5 * time.Second)
		for {
			var pendingStatus string
			if err := fixture.db.QueryRow("SELECT status FROM pve_pending_operations WHERE operation_id=?", "settle:"+id).Scan(&pendingStatus); err != nil {
				t.Fatal(err)
			}
			if pendingStatus == "done" {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("actual rollback Worker did not recover pending settlement")
			}
			time.Sleep(20 * time.Millisecond)
		}
		stop()
		var amountAfter, grantsAfter, mismatches int64
		if err := fixture.db.QueryRow("SELECT SUM(amount),COUNT(*) FROM pve_reward_grants WHERE run_id=?", id).Scan(&amountAfter, &grantsAfter); err != nil || amountBefore != amountAfter || grantsBefore != grantsAfter {
			t.Fatalf("rollback changed rewards: amount=%d grants=%d err=%v", amountAfter, grantsAfter, err)
		}
		if err := fixture.db.QueryRow("SELECT COUNT(*) FROM asset_ledger WHERE balance_after<>balance_before+delta").Scan(&mismatches); err != nil || mismatches != 0 {
			t.Fatal("rollback ledger mismatch", err)
		}
	}
	state, err := control.Observe(fixture.ctx, fixture.db)
	if err != nil || state.AdmissionState != "closed" {
		t.Fatal("rollback removed persistent pause", err)
	}
	if _, err := fixture.db.Exec("UPDATE pve_rule_versions SET status='published'"); err != nil {
		t.Fatal(err)
	}
	t.Logf("R4 default and actual R3 binary rollback: amount=%d grants=%d ledger_errors=0", amountBefore, grantsBefore)
}

func r4StartServer(t *testing.T, fixture *fixture, binary string, useDefault bool) (string, string, string, func()) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := strconv.Itoa(listener.Addr().(*net.TCPAddr).Port)
	listener.Close()
	internalListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	internalEndpoint := internalListener.Addr().String()
	internalListener.Close()
	eventToken := store.ID("r4_events")
	environment := map[string]string{"APP_PORT": port, "GAMEPLAY_MODE": "v2", "DB_HOST": "127.0.0.1", "DB_PORT": "23306", "DB_USER": "pve_test", "DB_PASSWORD": os.Getenv("PVE_TEST_PASSWORD"), "DB_NAME": "game_realtime_v2_test", "REDIS_ADDR": "127.0.0.1:26379", "REDIS_DB": "14", "REDIS_PASSWORD": "", "JWT_SECRET": fixture.secret, "PVE_TEST_EVENTS_ENABLED": "true", "PVE_TEST_EVENTS_ADDR": internalEndpoint, "PVE_TEST_EVENTS_TOKEN": eventToken, "PVE_METRICS_ENABLED": "false", "PVE_ARCHIVE_ENABLED": "false", "PPROF_ENABLED": "false", "PVE_RULES_PATH": ""}
	if useDefault {
		environment["GAMEPLAY_MODE"] = ""
	}
	command := exec.Command(binary)
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if _, exists := environment[key]; !exists {
			command.Env = append(command.Env, entry)
		}
	}
	for key, value := range environment {
		command.Env = append(command.Env, key+"="+value)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	stopped := false
	stop := func() {
		if !stopped {
			stopped = true
			_ = command.Process.Kill()
			_ = command.Wait()
		}
	}
	t.Cleanup(stop)
	address := "http://127.0.0.1:" + port
	client := &http.Client{Timeout: time.Second}
	for attempt := 0; attempt < 50; attempt++ {
		response, err := client.Get(address + "/health")
		if err == nil {
			response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return address, "http://" + internalEndpoint, eventToken, stop
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	stop()
	t.Fatal("rollback server failed to become healthy")
	return "", "", "", stop
}
