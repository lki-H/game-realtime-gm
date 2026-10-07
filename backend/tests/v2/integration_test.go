package v2_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"game-realtime-gm/backend/internal/auth"
	"game-realtime-gm/backend/internal/config"
	"game-realtime-gm/backend/internal/database"
	"game-realtime-gm/backend/internal/pve"
	"game-realtime-gm/backend/internal/pve/party"
	"game-realtime-gm/backend/internal/pve/run"
	"game-realtime-gm/backend/internal/pve/store"
	"game-realtime-gm/backend/internal/pve/task"
	"game-realtime-gm/backend/internal/router"
	"github.com/go-sql-driver/mysql"
	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
)

type fixture struct {
	db           *sql.DB
	app          *pve.App
	cache        *redis.Client
	ctx          context.Context
	players      []int64
	contributors []int64
	server       *httptest.Server
	secret       string
}

func TestSocialChatRecruitmentAndBlocking(t *testing.T) {
	fixture := setup(t)
	first, second := fixture.players[0], fixture.players[1]
	request := fixture.command(t, first, "v2.social.friend_request", map[string]any{"player_id": second})
	var requested struct {
		ID int64 `json:"id"`
	}
	_ = json.Unmarshal(request, &requested)
	fixture.command(t, second, "v2.social.friend_response", map[string]any{"request_id": requested.ID, "accept": true})
	fixture.command(t, first, "v2.social.note", map[string]any{"player_id": second, "note": "healer"})
	sent := fixture.command(t, first, "v2.social.message.send", map[string]any{"player_id": second, "body": "hello"})
	var message struct {
		ID int64 `json:"id"`
	}
	_ = json.Unmarshal(sent, &message)
	unread, err := fixture.app.Social.Query(fixture.ctx, second, "unread", 0)
	if err != nil || len(unread.([]map[string]any)) != 1 {
		t.Fatal("unread lost")
	}
	fixture.command(t, second, "v2.social.message.read", map[string]any{"player_id": first, "message_id": message.ID})
	fixture.command(t, second, "v2.social.block", map[string]any{"player_id": first})
	if _, err := fixture.app.Command(fixture.ctx, first, store.ID("cmd"), "v2.social.message.send", store.JSON(map[string]any{"player_id": second, "body": "blocked"})); err == nil {
		t.Fatal("blocked message allowed")
	}
	fixture.command(t, second, "v2.social.unblock", map[string]any{"player_id": first})
	created := fixture.command(t, first, "v2.party.create", map[string]any{})
	var group party.Party
	_ = json.Unmarshal(created, &group)
	posted := fixture.command(t, first, "v2.recruitment.publish", map[string]any{"party_id": group.ID})
	var post struct {
		PostID string `json:"post_id"`
	}
	_ = json.Unmarshal(posted, &post)
	applied := fixture.command(t, second, "v2.recruitment.apply", map[string]any{"post_id": post.PostID})
	var application struct {
		ID int64 `json:"application_id"`
	}
	_ = json.Unmarshal(applied, &application)
	fixture.command(t, first, "v2.recruitment.respond", map[string]any{"application_id": application.ID, "accept": true})
	snapshot, err := fixture.app.Party.Snapshot(fixture.ctx, group.ID, first)
	if err != nil || len(snapshot.Members) != 1 {
		t.Fatal("recruited guest joined friend room")
	}
}

func TestProposalRejectionPreservesOtherWaitingTime(t *testing.T) {
	fixture := setup(t)
	for _, player := range fixture.players {
		fixture.command(t, player, "v2.match.enqueue", map[string]any{"plan": party.Plan{Operation: "training_ground", Difficulty: "normal", FillPolicy: "public", AllowPartial: true}})
	}
	if err := fixture.app.Match.Match(fixture.ctx); err != nil {
		t.Fatal(err)
	}
	var proposal string
	_ = fixture.db.QueryRowContext(fixture.ctx, "SELECT id FROM pve_match_proposals WHERE status='pending'").Scan(&proposal)
	var since time.Time
	_ = fixture.db.QueryRowContext(fixture.ctx, "SELECT queue_priority_since FROM pve_match_tickets WHERE id=(SELECT activity_id FROM pve_player_activity_locks WHERE player_id=?)", fixture.players[1]).Scan(&since)
	fixture.command(t, fixture.players[0], "v2.match.proposal_reject", map[string]any{"proposal_id": proposal, "revision": 1})
	var after time.Time
	var status string
	if err := fixture.db.QueryRowContext(fixture.ctx, "SELECT queue_priority_since,status FROM pve_match_tickets WHERE id=(SELECT activity_id FROM pve_player_activity_locks WHERE player_id=?)", fixture.players[1]).Scan(&after, &status); err != nil {
		t.Fatal(err)
	}
	if !since.Equal(after) || status != "queued" {
		t.Fatal("innocent ticket priority changed")
	}
}

func TestLoadingFailureAndGapTimeout(t *testing.T) {
	fixture := setup(t)
	id, _ := fixture.assigned(t)
	fixture.event(t, id, 1, "loaded", fixture.players[0], "")
	fixture.app.Match.Now = func() time.Time { return time.Now().Add(time.Hour) }
	if err := fixture.app.Match.LoadingExpired(fixture.ctx, id); err != nil {
		t.Fatal(err)
	}
	state, err := fixture.app.Runs.Snapshot(fixture.ctx, id)
	if err != nil || state.EndReason != "loading_failed" {
		t.Fatal("loading not aborted")
	}
	if _, err := fixture.app.Runs.ApplyEvent(fixture.ctx, run.Event{EventID: store.ID("late"), Source: "pve_event_bot", SourceGeneration: 1, RunID: id, Sequence: 2, SchemaVersion: 2, EventType: "loaded", ActorPlayerID: &fixture.players[1], OccurredAt: time.Now()}); err == nil {
		t.Fatal("late loading revived aborted run")
	}
}

func TestWorkerRestartAssetsAndRedisRebuild(t *testing.T) {
	fixture := setup(t)
	id, _ := fixture.assigned(t)
	for index, player := range fixture.players {
		fixture.event(t, id, int64(index+1), "loaded", player, "")
	}
	fixture.event(t, id, 5, "kill", fixture.players[0], "enemy")
	if err := fixture.app.Recover(fixture.ctx); err != nil {
		t.Fatal(err)
	}
	if err := fixture.app.Worker.Once(fixture.ctx); err != nil {
		t.Fatal(err)
	}
	state, err := fixture.app.Runs.Snapshot(fixture.ctx, id)
	if err != nil || !state.Settled || state.EndReason != "server_restart" {
		t.Fatal("pending recovery failed")
	}
	if err := fixture.app.Worker.Once(fixture.ctx); err != nil {
		t.Fatal(err)
	}
	if err := fixture.cache.Del(fixture.ctx, "v2:queue:training_ground", "v2:rewards").Err(); err != nil {
		t.Fatal(err)
	}
	if err := fixture.app.Projection.Rebuild(fixture.ctx); err != nil {
		t.Fatal(err)
	}
}

func TestFinalReinforcementConcurrentEvents(t *testing.T) {
	fixture := setup(t)
	id, _ := fixture.assigned(t)
	for index, player := range fixture.players {
		fixture.event(t, id, int64(index+1), "loaded", player, "")
	}
	fixture.event(t, id, 5, "death", fixture.players[0], "")
	fixture.event(t, id, 6, "death", fixture.players[1], "")
	var stateData []byte
	if err := fixture.db.QueryRowContext(fixture.ctx, "SELECT state FROM pve_runs WHERE id=?", id).Scan(&stateData); err != nil {
		t.Fatal(err)
	}
	var state run.State
	_ = json.Unmarshal(stateData, &state)
	state.ReinforcementUsed = 2
	if _, err := fixture.db.ExecContext(fixture.ctx, "UPDATE pve_runs SET state=? WHERE id=?", store.JSON(state), id); err != nil {
		t.Fatal(err)
	}
	var wait sync.WaitGroup
	outcomes := make(chan error, 2)
	for index := 0; index < 2; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			_, err := fixture.app.Runs.ApplyEvent(fixture.ctx, run.Event{EventID: store.ID("concurrent"), Source: "pve_event_bot", SourceGeneration: 1, RunID: id, Sequence: 7, SchemaVersion: 2, EventType: "reinforcement.reserve", ActorPlayerID: &fixture.players[index], TargetID: store.ID("spawn"), OccurredAt: time.Now().UTC()})
			outcomes <- err
		}(index)
	}
	wait.Wait()
	close(outcomes)
	success := 0
	for err := range outcomes {
		if err == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("expected one reservation got %d", success)
	}
}

func TestInternalEventAuthAndRevokedSessions(t *testing.T) {
	fixture := setup(t)
	token, _ := auth.GenerateToken(fixture.secret, fixture.players[0], "test")
	cfg := config.Load()
	cfg.PVE.TestEventsEnabled = true
	cfg.PVE.TestEventsToken = "test-event-" + store.ID("secret")
	internal := httptest.NewServer(router.NewPVEInternalHandler(fixture.db, cfg, fixture.app))
	defer internal.Close()
	response, err := http.Post(internal.URL+"/internal/v2/runs/anything/events", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 403 {
		t.Fatal("internal listener accepts player")
	}
	var request *http.Request
	request, _ = http.NewRequest("GET", fixture.server.URL+"/api/v2/friends", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatal("valid session rejected")
	}
	_, err = fixture.db.ExecContext(fixture.ctx, "INSERT INTO pve_session_revocations(player_id,revoked_through) VALUES(?,UTC_TIMESTAMP(3))", fixture.players[0])
	if err != nil {
		t.Fatal(err)
	}
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 403 {
		t.Fatal("revoked session allowed")
	}
}

func setup(t *testing.T) *fixture {
	t.Helper()
	if os.Getenv("PVE_INTEGRATION") != "1" {
		t.Skip("set PVE_INTEGRATION=1 for isolated MySQL/Redis tests")
	}
	password := os.Getenv("PVE_TEST_PASSWORD")
	if password == "" {
		t.Fatal("temporary test password required")
	}
	dsn := mysql.Config{User: "pve_test", Passwd: password, Net: "tcp", Addr: "127.0.0.1:23306", DBName: "game_realtime_v2_test", ParseTime: true, Loc: time.UTC}
	redisAddress := "127.0.0.1:26379"
	if os.Getenv("PVE_TEST_NETWORK") == "compose" {
		dsn.Addr = "gm-v2-test-mysql-1:3306"
		redisAddress = "gm-v2-test-redis-1:6379"
	}
	db, err := sql.Open("mysql", dsn.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(10)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	t.Cleanup(func() { _ = db.Close() })
	var databaseName string
	if err := db.QueryRowContext(ctx, "SELECT DATABASE()").Scan(&databaseName); err != nil {
		t.Fatal(err)
	}
	if databaseName != "game_realtime_v2_test" {
		t.Fatal("unsafe database")
	}
	if status, err := database.ApplyV2Foundation(ctx, db, "../../internal/database/migrations/day37_v2_pve_foundation.sql"); err != nil || (status != "reconciled" && status != "applied") {
		t.Fatalf("migration ledger: status=%s err=%v", status, err)
	}
	if _, err := database.ApplyV2Products(ctx, db, "../../internal/database/migrations/day38_v2_product_model.sql"); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ApplyV2Engineering(ctx, db, "../../internal/database/migrations/day39_v2_engineering.sql"); err != nil {
		t.Fatal(err)
	}
	cache := redis.NewClient(&redis.Options{Addr: redisAddress, DB: 14})
	t.Cleanup(func() { _ = cache.Close() })
	if err := cache.Ping(ctx).Err(); err != nil {
		t.Fatal(err)
	}
	rows, err := db.QueryContext(ctx, "SHOW TABLES")
	if err != nil {
		t.Fatal(err)
	}
	tables := []string{}
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			t.Fatal(err)
		}
		if strings.HasPrefix(table, "pve_") {
			tables = append(tables, table)
		}
	}
	rows.Close()
	for _, table := range tables {
		if _, err := db.ExecContext(ctx, "DELETE FROM "+table); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ExecContext(ctx, "INSERT IGNORE INTO pve_match_lanes(id) VALUES('training_ground:normal')"); err != nil {
		t.Fatal(err)
	}
	rules, err := task.Load("")
	if err != nil {
		t.Fatal(err)
	}
	fixture := &fixture{db: db, ctx: ctx, cache: cache, app: pve.New(db, cache, rules), secret: "integration-only-random-" + store.ID("secret")}
	if err := fixture.app.Publish(ctx); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 4; index++ {
		name := store.ID("v2_test")
		result, err := db.ExecContext(ctx, "INSERT INTO players(username,password_hash,nickname,banned_reason) VALUES(?,'synthetic',?,'')", name, name)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := result.LastInsertId()
		fixture.players = append(fixture.players, id)
		if _, err := db.ExecContext(ctx, "INSERT INTO player_assets(player_id) VALUES(?)", id); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		cleanup := context.Background()
		for _, id := range fixture.players {
			_, _ = db.ExecContext(cleanup, "DELETE FROM asset_ledger WHERE player_id=?", id)
			_, _ = db.ExecContext(cleanup, "DELETE FROM player_assets WHERE player_id=?", id)
			_, _ = db.ExecContext(cleanup, "DELETE FROM players WHERE id=?", id)
		}
	})
	cfg := config.Load()
	cfg.GameplayMode = "v2"
	cfg.JWTSecret = fixture.secret
	fixture.server = httptest.NewServer(router.New(ctx, db, cache, cfg, fixture.app))
	t.Cleanup(fixture.server.Close)
	return fixture
}
func (f *fixture) command(t *testing.T, player int64, kind string, input any) json.RawMessage {
	t.Helper()
	output, err := f.app.Command(f.ctx, player, store.ID("command"), kind, store.JSON(input))
	if err != nil {
		t.Fatalf("%s: %v", kind, err)
	}
	return output
}
func (f *fixture) event(t *testing.T, id string, sequence int64, kind string, player int64, target string) *run.EventResult {
	t.Helper()
	contributors := f.contributors
	if contributors == nil {
		contributors = f.players
	}
	output, err := f.app.Runs.ApplyEvent(f.ctx, run.Event{EventID: fmt.Sprintf("event_%s_%d", id, sequence), Source: "pve_event_bot", RunID: id, Sequence: sequence, SchemaVersion: 2, SourceGeneration: 1, EventType: kind, ActorPlayerID: &player, TargetID: target, Contributors: contributors, OccurredAt: time.Unix(sequence, 0).UTC()})
	if err != nil {
		t.Fatalf("event %s %d: %v", kind, sequence, err)
	}
	return output
}
func (f *fixture) assigned(t *testing.T) (string, string) {
	t.Helper()
	first, second := f.players[0], f.players[1]
	request := f.command(t, first, "v2.social.friend_request", map[string]any{"player_id": second})
	var friendship struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(request, &friendship); err != nil {
		t.Fatal(err)
	}
	f.command(t, second, "v2.social.friend_response", map[string]any{"request_id": friendship.ID, "accept": true})
	created := f.command(t, first, "v2.party.create", map[string]any{})
	var group party.Party
	if err := json.Unmarshal(created, &group); err != nil {
		t.Fatal(err)
	}
	invitation := f.command(t, first, "v2.party.invite", map[string]any{"party_id": group.ID, "player_id": second})
	var token struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(invitation, &token); err != nil {
		t.Fatal(err)
	}
	f.command(t, second, "v2.party.accept_invite", map[string]any{"token": token.Token})
	f.command(t, first, "v2.party.plan_update", map[string]any{"party_id": group.ID, "plan": party.Plan{Operation: "training_ground", Difficulty: "normal", FillPolicy: "public", AllowPartial: true}})
	for index, player := range []int64{first, second} {
		key := "hunter"
		if index == 1 {
			key = "technician"
		}
		f.command(t, player, "v2.party.selection", map[string]any{"party_id": group.ID, "task_key": key, "task_version": f.app.Runs.Rules.Version})
	}
	current, err := f.app.Party.Snapshot(f.ctx, group.ID, first)
	if err != nil {
		t.Fatal(err)
	}
	for _, member := range current.Members {
		f.command(t, member.PlayerID, "v2.party.ready", map[string]any{"party_id": group.ID, "ready": true, "roster_version": current.RosterVersion, "plan_version": current.PlanVersion, "selection_version": member.SelectionVersion})
	}
	f.command(t, first, "v2.match.enqueue", map[string]any{"party_id": group.ID})
	for index, player := range f.players[2:] {
		selection := run.TaskSelection{}
		if index == 0 {
			selection = run.TaskSelection{TaskKey: "scout", TaskVersion: f.app.Runs.Rules.Version}
		}
		f.command(t, player, "v2.match.enqueue", map[string]any{"plan": party.Plan{Operation: "training_ground", Difficulty: "normal", FillPolicy: "public", AllowPartial: true}, "task": selection})
	}
	if err := f.app.Match.Match(f.ctx); err != nil {
		t.Fatal(err)
	}
	var proposal string
	if err := f.db.QueryRowContext(f.ctx, "SELECT id FROM pve_match_proposals WHERE status='pending' LIMIT 1").Scan(&proposal); err != nil {
		t.Fatal(err)
	}
	var assigned struct {
		RunID string `json:"run_id"`
	}
	for _, player := range f.players {
		output := f.command(t, player, "v2.match.proposal_confirm", map[string]any{"proposal_id": proposal, "revision": 1})
		if err := json.Unmarshal(output, &assigned); err != nil {
			t.Fatal(err)
		}
	}
	if assigned.RunID == "" {
		t.Fatal("missing run assignment")
	}
	return assigned.RunID, group.ID
}

func TestFourPlayerPersistentWorkflow(t *testing.T) {
	fixture := setup(t)
	id, partyID := fixture.assigned(t)
	sequence := int64(1)
	for _, player := range fixture.players {
		fixture.event(t, id, sequence, "loaded", player, "")
		sequence++
	}
	for index := 0; index < 3; index++ {
		fixture.event(t, id, sequence, "kill", fixture.players[index], strconv.Itoa(index))
		sequence++
	}
	fixture.event(t, id, sequence, "interact", fixture.players[3], "terminal")
	sequence++
	result := fixture.event(t, id, sequence, "reach", fixture.players[2], "exit")
	if !result.RunClosed {
		t.Fatal("shared operation did not end")
	}
	if err := fixture.app.Settlement.Settle(fixture.ctx, id); err != nil {
		t.Fatal(err)
	}
	if err := fixture.app.Settlement.Settle(fixture.ctx, id); err != nil {
		t.Fatal(err)
	}
	expected := []int64{130, 140, 125, 100}
	for index, player := range fixture.players {
		var balance int64
		if err := fixture.db.QueryRowContext(fixture.ctx, "SELECT soft_currency FROM player_assets WHERE player_id=?", player).Scan(&balance); err != nil {
			t.Fatal(err)
		}
		if balance != expected[index] {
			t.Fatalf("player %d balance %d expected %d", index, balance, expected[index])
		}
	}
	group, err := fixture.app.Party.Snapshot(fixture.ctx, partyID, fixture.players[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(group.Members) != 2 {
		t.Fatal("strangers leaked into friend party")
	}
	var locks int
	if err := fixture.db.QueryRowContext(fixture.ctx, "SELECT COUNT(*) FROM pve_player_activity_locks").Scan(&locks); err != nil || locks != 0 {
		t.Fatal("activity not released")
	}
	if err := fixture.app.Projection.Rebuild(fixture.ctx); err != nil {
		t.Fatal(err)
	}
}
func TestEventGapDuplicateAndConflict(t *testing.T) {
	fixture := setup(t)
	id, _ := fixture.assigned(t)
	event := run.Event{EventID: store.ID("event"), Source: "pve_event_bot", SourceGeneration: 1, RunID: id, Sequence: 2, SchemaVersion: 2, EventType: "loaded", ActorPlayerID: &fixture.players[1], OccurredAt: time.Now().UTC()}
	pending, err := fixture.app.Runs.ApplyEvent(fixture.ctx, event)
	if err != nil || !pending.GapDetected {
		t.Fatal("event not persisted pending")
	}
	fixture.event(t, id, 1, "loaded", fixture.players[0], "")
	retry, err := fixture.app.Runs.ApplyEvent(fixture.ctx, event)
	if err != nil || !retry.Duplicate || retry.AppliedSequence != 2 {
		t.Fatalf("gap retry failed: %+v %v", retry, err)
	}
	event.TargetID = "changed"
	if _, err := fixture.app.Runs.ApplyEvent(fixture.ctx, event); err == nil {
		t.Fatal("changed same event accepted")
	}
}
func TestWebSocketModeAndCommandIdempotency(t *testing.T) {
	fixture := setup(t)
	player := fixture.players[0]
	token, err := auth.GenerateToken(fixture.secret, player, "test")
	if err != nil {
		t.Fatal(err)
	}
	connection, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(fixture.server.URL, "http")+"/ws?token="+token, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	var response map[string]any
	if err := connection.ReadJSON(&response); err != nil {
		t.Fatal(err)
	}
	if err := connection.WriteJSON(map[string]any{"type": "mission.finish", "schema_version": 2, "data": map[string]any{}}); err != nil {
		t.Fatal(err)
	}
	if err := connection.ReadJSON(&response); err != nil {
		t.Fatal(err)
	}
	if response["code"] != float64(40971) {
		t.Fatal("legacy write bypassed mode")
	}
	operation := store.ID("intent")
	input := store.JSON(map[string]any{})
	first, err := fixture.app.Command(fixture.ctx, player, operation, "v2.party.create", input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := fixture.app.Command(fixture.ctx, player, operation, "v2.party.create", input)
	if err != nil || string(first) != string(second) {
		t.Fatal("command idempotency failed")
	}
}
