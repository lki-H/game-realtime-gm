package v2_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"game-realtime-gm/backend/internal/pve/store"
	"github.com/gorilla/websocket"
	"golang.org/x/crypto/bcrypt"
)

func TestM7HTTPIdentityAndConcurrentAdminTransitions(t *testing.T) {
	fixture := setup(t)
	password := store.ID("password")
	username := store.ID("m7_player")
	type envelope struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	type result struct {
		status int
		body   envelope
		err    error
	}
	client := &http.Client{Timeout: 5 * time.Second}
	send := func(method, path, token string, input any) result {
		request, err := http.NewRequest(method, fixture.server.URL+path, bytes.NewReader(store.JSON(input)))
		if err != nil {
			return result{err: err}
		}
		request.Header.Set("Content-Type", "application/json")
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		response, err := client.Do(request)
		if err != nil {
			return result{err: err}
		}
		defer response.Body.Close()
		value := result{status: response.StatusCode}
		value.err = json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&value.body)
		return value
	}
	require := func(value result, status, code int) json.RawMessage {
		t.Helper()
		if value.err != nil || value.status != status || value.body.Code != code {
			t.Fatalf("HTTP status=%d code=%d expected=%d/%d transport=%T", value.status, value.body.Code, status, code, value.err)
		}
		return value.body.Data
	}
	registration := map[string]any{"username": username, "password": password, "nickname": "M7 synthetic"}
	var player struct{ ID int64 }
	if err := json.Unmarshal(require(send("POST", "/api/register", "", registration), 201, 0), &player); err != nil || player.ID <= 0 {
		t.Fatal("real HTTP registration did not create a player")
	}
	adminIDs := []int64{}
	t.Cleanup(func() {
		_, _ = fixture.db.Exec("DELETE FROM admin_operation_logs WHERE target_type='player' AND target_id=?", player.ID)
		_, _ = fixture.db.Exec("DELETE FROM pve_session_revocations WHERE player_id=?", player.ID)
		_, _ = fixture.db.Exec("DELETE FROM player_assets WHERE player_id=?", player.ID)
		_, _ = fixture.db.Exec("DELETE FROM players WHERE id=?", player.ID)
		for _, adminID := range adminIDs {
			_, _ = fixture.db.Exec("DELETE FROM admin_operation_logs WHERE admin_id=?", adminID)
			_, _ = fixture.db.Exec("DELETE FROM admins WHERE id=?", adminID)
		}
	})
	var balance int64
	if err := fixture.db.QueryRow("SELECT soft_currency FROM player_assets WHERE player_id=?", player.ID).Scan(&balance); err != nil || balance != 0 {
		t.Fatal("registration asset transaction missing")
	}
	require(send("POST", "/api/register", "", registration), 409, 40901)
	require(send("POST", "/api/login", "", map[string]any{"username": username, "password": store.ID("wrong")}), 401, 40101)
	login := func() string {
		t.Helper()
		var identity struct{ Token string }
		if err := json.Unmarshal(require(send("POST", "/api/login", "", registration), 200, 0), &identity); err != nil || identity.Token == "" {
			t.Fatal("real HTTP login failed")
		}
		return identity.Token
	}
	playerToken := login()
	require(send("GET", "/api/me", playerToken, nil), 200, 0)
	if unauthorized := send("GET", "/api/admin/me", playerToken, nil); unauthorized.err != nil || (unauthorized.status != 401 && unauthorized.status != 403) {
		t.Fatal("player acquired administrator permission")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	adminTokens := []string{}
	for index := 0; index < 2; index++ {
		name := store.ID(fmt.Sprintf("m7_gm%d", index))
		inserted, err := fixture.db.Exec("INSERT INTO admins(username,password_hash,display_name,role) VALUES(?,?,'M7 synthetic','gm')", name, string(hash))
		if err != nil {
			t.Fatal(err)
		}
		adminID, _ := inserted.LastInsertId()
		adminIDs = append(adminIDs, adminID)
		var identity struct{ Token string }
		if err := json.Unmarshal(require(send("POST", "/api/admin/login", "", map[string]any{"username": name, "password": password}), 200, 0), &identity); err != nil || identity.Token == "" {
			t.Fatal("real administrator login failed")
		}
		adminTokens = append(adminTokens, identity.Token)
	}
	connection, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(fixture.server.URL, "http")+"/ws?token="+playerToken, nil)
	if err != nil {
		t.Fatal("player WebSocket handshake failed")
	}
	defer connection.Close()
	path := "/api/admin/players/" + strconv.FormatInt(player.ID, 10)
	concurrent := func(action string, input any, conflictCode int) {
		t.Helper()
		gate := make(chan struct{})
		results := make(chan result, 2)
		for _, token := range adminTokens {
			go func(token string) {
				<-gate
				results <- send("POST", path+"/"+action, token, input)
			}(token)
		}
		close(gate)
		successes, conflicts := 0, 0
		for index := 0; index < 2; index++ {
			value := <-results
			if value.err != nil {
				t.Fatal("concurrent administrator HTTP transport failed")
			}
			if value.status == 200 && value.body.Code == 0 {
				successes++
			} else if value.status == 409 && value.body.Code == conflictCode {
				conflicts++
			} else {
				t.Fatalf("unexpected concurrent status=%d code=%d", value.status, value.body.Code)
			}
		}
		if successes != 1 || conflicts != 1 {
			t.Fatal("concurrent state transition was not serialized")
		}
		var audits int
		if err := fixture.db.QueryRow("SELECT COUNT(*) FROM admin_operation_logs WHERE action=? AND target_id=?", "admin.players."+action, player.ID).Scan(&audits); err != nil || audits != 1 {
			t.Fatalf("successful transition audit count=%d", audits)
		}
	}
	concurrent("ban", map[string]any{"reason": "M7 isolated transition"}, 40951)
	require(send("GET", "/api/me", playerToken, nil), 403, 40321)
	require(send("POST", "/api/login", "", registration), 403, 40321)
	_ = connection.SetReadDeadline(time.Now().Add(4 * time.Second))
	for {
		if _, _, err := connection.ReadMessage(); err != nil {
			if timeout, ok := err.(interface{ Timeout() bool }); ok && timeout.Timeout() {
				t.Fatal("banned existing WebSocket was not closed")
			}
			break
		}
	}
	concurrent("unban", map[string]any{"reason": "M7 isolated restore"}, 40961)
	if stale := send("GET", "/api/me", playerToken, nil); stale.err != nil || stale.status != 403 {
		t.Fatal("unban revived a revoked token")
	}
	oldConnection, response, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(fixture.server.URL, "http")+"/ws?token="+playerToken, nil)
	if oldConnection != nil {
		oldConnection.Close()
	}
	if response != nil {
		response.Body.Close()
	}
	if err == nil || response == nil || response.StatusCode != 403 {
		t.Fatal("unban revived a revoked WebSocket identity")
	}
	require(send("GET", "/api/me", login(), nil), 200, 0)
}
