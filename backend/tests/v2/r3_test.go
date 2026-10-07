package v2_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"game-realtime-gm/backend/internal/auth"
	"game-realtime-gm/backend/internal/config"
	"game-realtime-gm/backend/internal/pve/run"
	"game-realtime-gm/backend/internal/pve/store"
	"game-realtime-gm/backend/internal/router"
	"github.com/gorilla/websocket"
)

func TestR3ArchivePreservesDedupAndHistoricalRecovery(t *testing.T) {
	fixture := setup(t)
	runID, _ := fixture.assigned(t)
	for index, player := range fixture.players {
		fixture.event(t, runID, int64(index+1), "loaded", player, "")
	}
	for index := 0; index < 3; index++ {
		fixture.event(t, runID, int64(index+5), "kill", fixture.players[index], store.ID("enemy"))
	}
	fixture.event(t, runID, 8, "interact", fixture.players[1], "terminal")
	fixture.event(t, runID, 9, "reach", fixture.players[2], "exit")
	if err := fixture.app.Worker.Once(fixture.ctx); err != nil {
		t.Fatal(err)
	}
	var payload []byte
	if err := fixture.db.QueryRow("SELECT payload FROM pve_run_events WHERE run_id=? AND sequence_no=1", runID).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	var event run.Event
	if err := json.Unmarshal(payload, &event); err != nil {
		t.Fatal(err)
	}
	operation := store.ID("archive_command")
	input := store.JSON(map[string]any{"player_id": fixture.players[3]})
	if _, err := fixture.app.Command(fixture.ctx, fixture.players[2], operation, "v2.social.friend_request", input); err != nil {
		t.Fatal(err)
	}
	var balanceBefore int64
	if err := fixture.db.QueryRow("SELECT SUM(soft_currency) FROM player_assets WHERE player_id IN (?,?,?,?)", fixture.players[0], fixture.players[1], fixture.players[2], fixture.players[3]).Scan(&balanceBefore); err != nil {
		t.Fatal(err)
	}
	fixture.app.Retention.Enabled = true
	fixture.app.Retention.Now = func() time.Time { return time.Now().AddDate(0, 0, 91) }
	if err := fixture.app.Retention.Compact(fixture.ctx); err != nil {
		t.Fatal(err)
	}
	if err := fixture.app.Retention.Compact(fixture.ctx); err != nil {
		t.Fatal(err)
	}
	var archiveCount, compactedEvents int
	if err := fixture.db.QueryRow("SELECT COUNT(*) FROM pve_run_archives WHERE run_id=?", runID).Scan(&archiveCount); err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.QueryRow("SELECT COUNT(*) FROM pve_run_events WHERE run_id=? AND JSON_EXTRACT(payload,'$._pve_archived')=true", runID).Scan(&compactedEvents); err != nil {
		t.Fatal(err)
	}
	if archiveCount != 1 || compactedEvents != 9 {
		t.Fatalf("archives=%d compacted=%d", archiveCount, compactedEvents)
	}
	duplicate, err := fixture.app.Runs.ApplyEvent(fixture.ctx, event)
	if err != nil || !duplicate.Duplicate {
		t.Fatal("compacted event lost deduplication", err)
	}
	event.TargetID = "changed"
	if _, err := fixture.app.Runs.ApplyEvent(fixture.ctx, event); !errors.Is(err, store.Conflict) {
		t.Fatal("compacted event accepted changed content", err)
	}
	if _, err := fixture.app.Command(fixture.ctx, fixture.players[2], operation, "v2.social.friend_request", input); !errors.Is(err, store.Archived) {
		t.Fatal("old command did not require recovery", err)
	}
	if _, err := fixture.app.Command(fixture.ctx, fixture.players[2], operation, "v2.social.friend_request", store.JSON(map[string]any{"player_id": fixture.players[0]})); !errors.Is(err, store.Conflict) {
		t.Fatal("archived command accepted different content", err)
	}
	if err := fixture.app.Settlement.Settle(fixture.ctx, runID); err != nil {
		t.Fatal(err)
	}
	state, err := fixture.app.Runs.Snapshot(fixture.ctx, runID)
	if err != nil || !state.Settled || len(state.Participants) != 4 {
		t.Fatal("historical snapshot lost", err)
	}
	var balanceAfter int64
	if err := fixture.db.QueryRow("SELECT SUM(soft_currency) FROM player_assets WHERE player_id IN (?,?,?,?)", fixture.players[0], fixture.players[1], fixture.players[2], fixture.players[3]).Scan(&balanceAfter); err != nil {
		t.Fatal(err)
	}
	if balanceBefore != balanceAfter {
		t.Fatal("archive replay changed assets")
	}
}

func TestR3ArchiveNeverCompactsRunningOrPendingRun(t *testing.T) {
	fixture := setup(t)
	runID, _ := fixture.assigned(t)
	for index, player := range fixture.players {
		fixture.event(t, runID, int64(index+1), "loaded", player, "")
	}
	fixture.app.Retention.Enabled = true
	fixture.app.Retention.Now = func() time.Time { return time.Now().AddDate(0, 0, 91) }
	if err := fixture.app.Retention.Compact(fixture.ctx); err != nil {
		t.Fatal(err)
	}
	var archived int
	if err := fixture.db.QueryRow("SELECT COUNT(*) FROM pve_run_archives WHERE run_id=?", runID).Scan(&archived); err != nil {
		t.Fatal(err)
	}
	if archived != 0 {
		t.Fatal("running run archived")
	}
	if _, err := fixture.db.Exec("DELETE FROM player_assets WHERE player_id=?", fixture.players[3]); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 3; index++ {
		fixture.event(t, runID, int64(index+5), "kill", fixture.players[index], store.ID("enemy"))
	}
	fixture.event(t, runID, 8, "interact", fixture.players[1], "terminal")
	fixture.event(t, runID, 9, "reach", fixture.players[2], "exit")
	_ = fixture.app.Worker.Once(fixture.ctx)
	if err := fixture.app.Retention.Compact(fixture.ctx); err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.QueryRow("SELECT COUNT(*) FROM pve_run_archives WHERE run_id=?", runID).Scan(&archived); err != nil {
		t.Fatal(err)
	}
	if archived != 0 {
		t.Fatal("unsettled run archived")
	}
}

func TestR3ConnectionQuotaAllowsReplacement(t *testing.T) {
	for _, perIP := range []int{2, 1} {
		t.Run(string(rune('0'+perIP)), func(t *testing.T) {
			fixture := setup(t)
			cfg := config.Load()
			cfg.GameplayMode = "v2"
			cfg.JWTSecret = fixture.secret
			cfg.HTTP.WebSocketMaxConnections = 2
			cfg.HTTP.WebSocketMaxPerIP = perIP
			server := httptest.NewServer(router.New(fixture.ctx, fixture.db, fixture.cache, cfg, fixture.app))
			defer server.Close()
			connect := func(player int64) (*websocket.Conn, *http.Response, error) {
				token, err := auth.GenerateToken(fixture.secret, player, "quota-test")
				if err != nil {
					return nil, nil, err
				}
				return websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/ws", http.Header{"Authorization": []string{"Bearer " + token}})
			}
			first, _, err := connect(fixture.players[0])
			if err != nil {
				t.Fatal(err)
			}
			defer first.Close()
			var welcome any
			if err := first.ReadJSON(&welcome); err != nil {
				t.Fatal(err)
			}
			if perIP == 2 {
				second, _, err := connect(fixture.players[1])
				if err != nil {
					t.Fatal(err)
				}
				defer second.Close()
				if err := second.ReadJSON(&welcome); err != nil {
					t.Fatal(err)
				}
			}
			unexpected, response, err := connect(fixture.players[2])
			if unexpected != nil {
				unexpected.Close()
			}
			if err == nil || response == nil || response.StatusCode != 429 {
				t.Fatal("connection quota not enforced")
			}
			response.Body.Close()
			replacement, _, err := connect(fixture.players[0])
			if err != nil {
				t.Fatal("replacement blocked", err)
			}
			defer replacement.Close()
			if err := replacement.ReadJSON(&welcome); err != nil {
				t.Fatal(err)
			}
			_ = first.SetReadDeadline(time.Now().Add(time.Second))
			if _, _, err := first.ReadMessage(); err == nil {
				t.Fatal("old connection survived replacement")
			}
		})
	}
}

func TestR3MetricsServiceIdentityAndObservationPagination(t *testing.T) {
	fixture := setup(t)
	secret := store.ID("metrics_only")
	server := httptest.NewServer(router.NewPVEMetricsHandler(fixture.db, secret))
	defer server.Close()
	for _, header := range []string{"", "Bearer " + fixture.secret, secret, "Bearer " + secret} {
		request, _ := http.NewRequest("GET", server.URL+"/metrics", nil)
		request.Header.Set("Authorization", header)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		expected := 401
		if header == "Bearer "+secret {
			expected = 200
		}
		if response.StatusCode != expected {
			t.Fatalf("metrics status=%d expected=%d", response.StatusCode, expected)
		}
	}
	result, err := fixture.db.Exec("INSERT INTO admins(username,password_hash,display_name,role) VALUES(?,'synthetic','pagination','gm')", store.ID("gm"))
	if err != nil {
		t.Fatal(err)
	}
	adminID, _ := result.LastInsertId()
	t.Cleanup(func() { _, _ = fixture.db.Exec("DELETE FROM admins WHERE id=?", adminID) })
	token, err := auth.GenerateAdminToken(fixture.secret, adminID, "pagination", "gm")
	if err != nil {
		t.Fatal(err)
	}
	fixture.command(t, fixture.players[0], "v2.party.create", map[string]any{})
	fixture.command(t, fixture.players[1], "v2.party.create", map[string]any{})
	for _, query := range []string{"?page=1&page_size=1", "?page=2&page_size=1", "?page=0", "?page_size=51"} {
		request, _ := http.NewRequest("GET", fixture.server.URL+"/api/admin/v2/observations/parties"+query, nil)
		request.Header.Set("Authorization", "Bearer "+token)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		if query == "?page=0" || query == "?page_size=51" {
			response.Body.Close()
			if response.StatusCode != 400 {
				t.Fatal("invalid pagination accepted")
			}
			continue
		}
		var envelope struct {
			Data       []map[string]any
			Pagination struct {
				Total    int
				Page     int
				PageSize int `json:"page_size"`
			}
		}
		err = json.NewDecoder(response.Body).Decode(&envelope)
		response.Body.Close()
		if err != nil || response.StatusCode != 200 || len(envelope.Data) != 1 || envelope.Pagination.Total != 2 {
			t.Fatal("pagination failed", err)
		}
	}
}
