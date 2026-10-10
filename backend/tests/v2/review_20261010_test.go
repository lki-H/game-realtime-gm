package v2_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"game-realtime-gm/backend/internal/auth"
	"game-realtime-gm/backend/internal/pve"
	"game-realtime-gm/backend/internal/pve/control"
	"game-realtime-gm/backend/internal/pve/party"
	"game-realtime-gm/backend/internal/pve/store"
	"game-realtime-gm/backend/internal/pve/worker"
	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
)

func TestReviewPlayerSessionRechecksJWTLifetime(t *testing.T) {
	fixture := setup(t)
	claims := &auth.Claims{SubjectType: auth.SubjectTypePlayer, PlayerID: fixture.players[0], RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Second))}}
	if err := pve.Authorize(fixture.ctx, fixture.db, claims); !errors.Is(err, store.Forbidden) {
		t.Fatal("expired claims retain authorization")
	}
	claims.ExpiresAt = nil
	if err := pve.Authorize(fixture.ctx, fixture.db, claims); !errors.Is(err, store.Forbidden) {
		t.Fatal("claims without expiration retain authorization")
	}
	claims.ExpiresAt = jwt.NewNumericDate(time.Now().Add(time.Hour))
	claims.NotBefore = jwt.NewNumericDate(time.Now().Add(time.Minute))
	if err := pve.Authorize(fixture.ctx, fixture.db, claims); !errors.Is(err, store.Forbidden) {
		t.Fatal("future claims retain authorization")
	}
}

func TestReviewLoadingFailureRequeuesFriendRoomState(t *testing.T) {
	fixture := setup(t)
	id, roomID := fixture.assigned(t)
	for index, player := range fixture.players[:3] {
		fixture.event(t, id, int64(index+1), "loaded", player, "")
	}
	fixture.app.Match.Now = func() time.Time { return time.Now().Add(time.Hour) }
	if err := fixture.app.Match.LoadingExpired(fixture.ctx, id); err != nil {
		t.Fatal(err)
	}
	room, err := fixture.app.Party.Snapshot(fixture.ctx, roomID, fixture.players[0])
	if err != nil || room.Status != "queued" {
		t.Fatalf("friend room kept obsolete Run state after requeue: %+v %v", room, err)
	}
	fixture.command(t, fixture.players[0], "v2.match.cancel", map[string]any{"party_id": roomID})
	room, err = fixture.app.Party.Snapshot(fixture.ctx, roomID, fixture.players[0])
	if err != nil || room.Status != "open" {
		t.Fatal("requeued friend room cannot cancel")
	}
}

func TestReviewBlockAppliesToEveryFriendRoomMember(t *testing.T) {
	fixture := setup(t)
	created := fixture.command(t, fixture.players[0], "v2.party.create", map[string]any{})
	var room party.Party
	if err := json.Unmarshal(created, &room); err != nil {
		t.Fatal(err)
	}
	invite := fixture.command(t, fixture.players[0], "v2.party.invite", map[string]any{"party_id": room.ID, "player_id": fixture.players[1]})
	var token struct {
		Token string `json:"token"`
	}
	json.Unmarshal(invite, &token)
	fixture.command(t, fixture.players[1], "v2.party.accept_invite", map[string]any{"token": token.Token})
	fixture.command(t, fixture.players[1], "v2.social.block", map[string]any{"player_id": fixture.players[2]})
	invite = fixture.command(t, fixture.players[0], "v2.party.invite", map[string]any{"party_id": room.ID, "player_id": fixture.players[2]})
	json.Unmarshal(invite, &token)
	if _, err := fixture.app.Command(fixture.ctx, fixture.players[2], store.ID("blocked_join"), "v2.party.accept_invite", store.JSON(map[string]any{"token": token.Token})); !errors.Is(err, store.Forbidden) {
		t.Fatal("invited player bypassed another member's block")
	}
}

func TestReviewEstablishedWebSocketRejectsExpiredJWT(t *testing.T) {
	fixture := setup(t)
	claims := auth.Claims{SubjectType: auth.SubjectTypePlayer, PlayerID: fixture.players[0], RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(2 * time.Second))}}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(fixture.secret))
	if err != nil {
		t.Fatal(err)
	}
	connection, _, err := websocket.DefaultDialer.Dial("ws"+fixture.server.URL[4:]+"/ws", http.Header{"Authorization": []string{"Bearer " + token}})
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	var message map[string]any
	if err := connection.ReadJSON(&message); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Until(claims.ExpiresAt.Time) + 100*time.Millisecond)
	if err := connection.WriteJSON(map[string]any{"schema_version": 2, "type": "v2.party.create", "operation_id": store.ID("expired_intent"), "request_id": "expired_request", "data": map[string]any{}}); err == nil {
		connection.SetReadDeadline(time.Now().Add(3 * time.Second))
		if err := connection.ReadJSON(&message); err == nil && message["code"] == float64(0) {
			t.Fatal("expired established connection executed a command")
		}
	}
	var count int
	if err := fixture.db.QueryRow("SELECT COUNT(*) FROM pve_parties WHERE owner_id=?", fixture.players[0]).Scan(&count); err != nil || count != 0 {
		t.Fatal("expired connection created a party")
	}
}
func TestReviewMalformedOutboxDoesNotStarveHealthyNotifications(t *testing.T) {
	fixture := setup(t)
	badID := store.ID("bad_outbox")
	if _, err := fixture.db.Exec("INSERT INTO pve_outbox_records(operation_id,event_type,aggregate_id,payload) VALUES(?,'v2.bad','notification',JSON_ARRAY(1))", badID); err != nil {
		t.Fatal(err)
	}
	if err := store.Transaction(fixture.ctx, fixture.db, func(transaction *sql.Tx) error {
		return store.Notify(fixture.ctx, transaction, []int64{fixture.players[0]}, "v2.review.changed", map[string]any{"ok": true})
	}); err != nil {
		t.Fatal(err)
	}
	notified := 0
	processor := worker.Worker{DB: fixture.db, Notify: func(players []int64, kind string, data json.RawMessage) { notified++ }}
	if err := processor.Once(fixture.ctx); err != nil {
		t.Fatalf("one malformed outbox blocks healthy records: %v", err)
	}
	if notified != 1 {
		t.Fatalf("healthy notifications delivered=%d", notified)
	}
	var state string
	if err := fixture.db.QueryRow("SELECT status FROM pve_outbox_records WHERE operation_id=?", badID).Scan(&state); err != nil || state != "needs_repair" {
		t.Fatal("bad outbox not durably isolated")
	}
	if _, err := fixture.db.Exec("UPDATE pve_service_control SET admission_state='draining' WHERE id='gameplay'"); err != nil {
		t.Fatal(err)
	}
	snapshot, err := control.Observe(context.Background(), fixture.db)
	if err != nil || snapshot.NeedsRepair != 1 || snapshot.ReadyToStop {
		t.Fatal("quarantined outbox silently ignored by drain readiness")
	}
}

func TestReviewIncompatibleTaskAttemptClosesAfterRun(t *testing.T) {
	fixture := setup(t)
	id := productRun(t, fixture, []string{"other_map", ""})
	if err := fixture.app.Recover(fixture.ctx); err != nil {
		t.Fatal(err)
	}
	if err := fixture.app.Worker.Once(fixture.ctx); err != nil {
		t.Fatal(err)
	}
	var status string
	if err := fixture.db.QueryRow("SELECT status FROM pve_player_task_attempts WHERE run_id=?", id).Scan(&status); err != nil || status != "closed" {
		t.Fatalf("finished incompatible task attempt remains %s: %v", status, err)
	}
}

func TestReviewPermanentlyLeftTaskAttemptCloses(t *testing.T) {
	fixture := setup(t)
	id := productRun(t, fixture, []string{"medic", ""})
	if err := fixture.app.Runs.Leave(fixture.ctx, fixture.players[0], id); err != nil {
		t.Fatal(err)
	}
	var status string
	if err := fixture.db.QueryRow("SELECT status FROM pve_player_task_attempts WHERE run_id=?", id).Scan(&status); err != nil || status != "closed" {
		t.Fatalf("left player's task attempt remains %s: %v", status, err)
	}
	state, err := fixture.app.Runs.Snapshot(fixture.ctx, id)
	if err != nil || state.Status != "running" {
		t.Fatal("closing personal attempt stopped teammates")
	}
	if state.Member(&fixture.players[0]).Status != "left" {
		t.Fatal("permanent exit status missing")
	}
}
