package v2_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"game-realtime-gm/backend/internal/auth"
	"game-realtime-gm/backend/internal/config"
	"game-realtime-gm/backend/internal/pve/run"
	"game-realtime-gm/backend/internal/pve/store"
	"game-realtime-gm/backend/internal/router"
	"github.com/goccy/go-yaml"
	"github.com/gorilla/websocket"
)

func requestWS(t *testing.T, connection *websocket.Conn, kind string, input any) json.RawMessage {
	t.Helper()
	request := store.ID("ws")
	if err := connection.WriteJSON(map[string]any{"type": kind, "schema_version": 2, "operation_id": store.ID("operation"), "request_id": request, "data": input}); err != nil {
		t.Fatal(err)
	}
	_ = connection.SetReadDeadline(time.Now().Add(5 * time.Second))
	for {
		var response struct {
			Request string          `json:"request_id"`
			Type    string          `json:"type"`
			Code    int             `json:"code"`
			Message string          `json:"message"`
			Data    json.RawMessage `json:"data"`
		}
		if err := connection.ReadJSON(&response); err != nil {
			t.Fatal(err)
		}
		if response.Request != request {
			continue
		}
		if response.Code != 0 {
			t.Fatalf("%s code=%d %s", kind, response.Code, response.Message)
		}
		return response.Data
	}
}

func TestFourWebSocketsWithInternalHTTPAndResultRecovery(t *testing.T) {
	fixture := setup(t)
	connections := []*websocket.Conn{}
	tokens := []string{}
	for _, player := range fixture.players {
		token, err := auth.GenerateToken(fixture.secret, player, "ws-test")
		if err != nil {
			t.Fatal(err)
		}
		tokens = append(tokens, token)
		connection, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(fixture.server.URL, "http")+"/ws?token="+token, nil)
		if err != nil {
			t.Fatal(err)
		}
		connections = append(connections, connection)
		t.Cleanup(func() { _ = connection.Close() })
		var welcome any
		if err := connection.ReadJSON(&welcome); err != nil {
			t.Fatal(err)
		}
	}
	for index, connection := range connections {
		taskKey := ""
		if index == 0 {
			taskKey = "hunter"
		}
		requestWS(t, connection, "v2.match.enqueue", map[string]any{"plan": map[string]any{"operation": "training_ground", "difficulty": "normal", "fill_policy": "public", "allow_partial": true}, "task": map[string]any{"task_key": taskKey, "task_version": "training_ground.v1"}})
	}
	if err := fixture.app.Match.Match(fixture.ctx); err != nil {
		t.Fatal(err)
	}
	var proposal string
	if err := fixture.db.QueryRowContext(fixture.ctx, "SELECT id FROM pve_match_proposals WHERE status='pending'").Scan(&proposal); err != nil {
		t.Fatal(err)
	}
	var assigned struct {
		RunID string `json:"run_id"`
	}
	for _, connection := range connections {
		data := requestWS(t, connection, "v2.match.proposal_confirm", map[string]any{"proposal_id": proposal, "revision": 1})
		if err := json.Unmarshal(data, &assigned); err != nil {
			t.Fatal(err)
		}
	}
	cfg := config.Load()
	cfg.PVE.TestEventsEnabled = true
	cfg.PVE.TestEventsToken = store.ID("trusted-source")
	internal := httptest.NewServer(router.NewPVEInternalHandler(fixture.db, cfg, fixture.app))
	defer internal.Close()
	sequence := int64(1)
	send := func(kind string, player int64, target string) {
		t.Helper()
		event := run.Event{EventID: store.ID("event"), Source: "pve_event_bot", SourceGeneration: 1, Sequence: sequence, RunID: assigned.RunID, SchemaVersion: 2, EventType: kind, ActorPlayerID: &player, Contributors: fixture.players, TargetID: target, OccurredAt: time.Now().UTC()}
		sequence++
		request, err := http.NewRequest("POST", internal.URL+"/internal/v2/runs/"+assigned.RunID+"/events", bytes.NewReader(store.JSON(event)))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-PVE-Test-Token", cfg.PVE.TestEventsToken)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		if response.StatusCode != 200 {
			body, _ := io.ReadAll(io.LimitReader(response.Body, 2000))
			t.Fatalf("HTTP event %s status=%d body=%s", kind, response.StatusCode, string(body))
		}
	}
	for _, player := range fixture.players {
		send("loaded", player, "")
	}
	for index := 0; index < 3; index++ {
		send("kill", fixture.players[0], store.ID("enemy"))
	}
	send("interact", fixture.players[1], "terminal")
	send("reach", fixture.players[2], "exit")
	if err := fixture.app.Worker.Once(fixture.ctx); err != nil {
		t.Fatal(err)
	}
	data := requestWS(t, connections[0], "v2.run.result", map[string]any{"run_id": assigned.RunID})
	var state run.State
	if err := json.Unmarshal(data, &state); err != nil || !state.Settled {
		t.Fatal("WS settled result missing")
	}
	request, _ := http.NewRequest("GET", fixture.server.URL+"/api/v2/runs/"+assigned.RunID+"/results", nil)
	request.Header.Set("Authorization", "Bearer "+tokens[0])
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatal("HTTP recovery failed")
	}
	adminResult, err := fixture.db.ExecContext(fixture.ctx, "INSERT INTO admins(username,password_hash,display_name,role) VALUES(?,'synthetic','test gm','gm')", store.ID("gm"))
	if err != nil {
		t.Fatal(err)
	}
	adminID, _ := adminResult.LastInsertId()
	t.Cleanup(func() {
		_, _ = fixture.db.Exec("DELETE FROM admin_operation_logs WHERE admin_id=?", adminID)
		_, _ = fixture.db.Exec("DELETE FROM admins WHERE id=?", adminID)
	})
	adminToken, _ := auth.GenerateAdminToken(fixture.secret, adminID, "gm-test", "gm")
	for _, entity := range []string{"parties", "proposals", "runs", "tasks", "pending"} {
		request, _ = http.NewRequest("GET", fixture.server.URL+"/api/admin/v2/observations/"+entity, nil)
		request.Header.Set("Authorization", "Bearer "+adminToken)
		response, err = http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		var envelope struct {
			Code int              `json:"code"`
			Data []map[string]any `json:"data"`
		}
		decodeErr := json.NewDecoder(response.Body).Decode(&envelope)
		response.Body.Close()
		if response.StatusCode != 200 || decodeErr != nil || envelope.Code != 0 {
			t.Fatalf("observation %s failed", entity)
		}
	}
	request, _ = http.NewRequest("POST", fixture.server.URL+"/api/admin/players/"+strconv.FormatInt(fixture.players[0], 10)+"/ban", strings.NewReader(`{"reason":"isolated test"}`))
	request.Header.Set("Authorization", "Bearer "+adminToken)
	request.Header.Set("Content-Type", "application/json")
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatal("ban failed")
	}
	_ = connections[0].SetReadDeadline(time.Now().Add(4 * time.Second))
	for {
		_, _, err = connections[0].ReadMessage()
		if err != nil {
			break
		}
	}
	if timeout, ok := err.(interface{ Timeout() bool }); ok && timeout.Timeout() {
		t.Fatal("banned connection not closed")
	}
	_, response, err = websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(fixture.server.URL, "http")+"/ws?token="+tokens[0], nil)
	if err == nil || response == nil || response.StatusCode != 403 {
		t.Fatal("banned player reconnected")
	}
	response.Body.Close()
	request, _ = http.NewRequest("GET", fixture.server.URL+"/api/admin/v2/metrics", nil)
	request.Header.Set("Authorization", "Bearer "+adminToken)
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	metrics, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != 200 || !strings.Contains(string(metrics), "pve_settlement_seconds") {
		t.Fatal("metrics unavailable")
	}
}

func TestOpenAPILocalReferencesAndV2Paths(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", "openapi.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := yaml.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	paths := document["paths"].(map[string]any)
	for _, path := range []string{"/api/v2/runs/{run_id}/snapshot", "/api/v2/me/activity", "/api/v2/friends", "/api/v2/operations/catalog", "/api/v2/operations/preview", "/api/v2/operations/recommendations", "/api/v2/recruitment/parties/{party_id}", "/api/v2/regroup/{proposal_id}", "/api/admin/v2/operations/{operation_id}/retry", "/api/admin/v2/control", "/api/admin/v2/observations/{entity}", "/api/admin/v2/metrics"} {
		if _, exists := paths[path]; !exists {
			t.Fatalf("missing %s", path)
		}
	}
	var visit func(any)
	visit = func(value any) {
		switch current := value.(type) {
		case map[string]any:
			if reference, ok := current["$ref"].(string); ok && strings.HasPrefix(reference, "#/") {
				var target any = document
				for _, segment := range strings.Split(strings.TrimPrefix(reference, "#/"), "/") {
					object, ok := target.(map[string]any)
					if !ok {
						t.Fatalf("invalid ref %s", reference)
					}
					var exists bool
					target, exists = object[segment]
					if !exists {
						t.Fatalf("unresolved ref %s", reference)
					}
				}
			}
			for _, child := range current {
				visit(child)
			}
		case []any:
			for _, child := range current {
				visit(child)
			}
		}
	}
	visit(document)
}
