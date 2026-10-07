package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"game-realtime-gm/backend/internal/config"
	"game-realtime-gm/backend/internal/database"
	"game-realtime-gm/backend/internal/pve/run"
	"game-realtime-gm/backend/internal/pve/store"
	"github.com/gorilla/websocket"
	"golang.org/x/crypto/bcrypt"
)

type verifier struct {
	endpoint, password, prefix string
	http                       *http.Client
	fault                      bool
}
type player struct {
	id         int64
	token      string
	connection *websocket.Conn
}

func main() {
	mode := flag.String("mode", "prepare", "prepare, scenario, soak or check")
	endpoint := flag.String("endpoint", "http://127.0.0.1:8080", "local V2 service")
	duration := flag.Duration("duration", 5*time.Minute, "soak duration")
	flag.Parse()
	parsed, err := url.Parse(*endpoint)
	if err != nil || parsed.Scheme != "http" || parsed.Hostname() != "127.0.0.1" {
		fail(errors.New("local loopback endpoint required"))
	}
	cfg := config.Load()
	if cfg.Database.Name != "game_realtime_v2_test" || cfg.Database.Port != "23306" || cfg.GameplayMode != "v2" {
		fail(errors.New("isolated V2 test configuration required"))
	}
	password, prefix := os.Getenv("PVE_VERIFY_PASSWORD"), os.Getenv("PVE_VERIFY_PREFIX")
	if len(password) < 24 || !store.ValidID(prefix) || len(prefix) > 40 {
		fail(errors.New("temporary verification password and prefix required"))
	}
	verifier := &verifier{endpoint: strings.TrimRight(*endpoint, "/"), password: password, prefix: prefix, http: &http.Client{Timeout: 10 * time.Second}}
	switch *mode {
	case "prepare":
		err = verifier.prepare()
	case "scenario":
		_, err = verifier.scenario()
	case "fault":
		verifier.fault = true
		_, err = verifier.scenario()
	case "repair-fixture":
		err = verifier.restoreAsset()
	case "soak":
		err = verifier.soak(*duration)
	case "check":
		err = verifier.check()
	default:
		err = errors.New("unknown verification mode")
	}
	if err != nil {
		fail(err)
	}
}

func (verifier *verifier) request(method, path, token string, input any, output any) error {
	var body io.Reader
	if input != nil {
		body = bytes.NewReader(store.JSON(input))
	}
	request, err := http.NewRequest(method, verifier.endpoint+path, body)
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := verifier.http.Do(request)
	if err != nil {
		return fmt.Errorf("HTTP transport failed: %T", err)
	}
	defer response.Body.Close()
	var envelope struct {
		Code int
		Data json.RawMessage
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&envelope); err != nil {
		return errors.New("invalid HTTP response")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 || envelope.Code != 0 {
		return fmt.Errorf("HTTP %s %s status=%d code=%d", method, path, response.StatusCode, envelope.Code)
	}
	if output != nil {
		return json.Unmarshal(envelope.Data, output)
	}
	return nil
}

func (verifier *verifier) prepare() error {
	for index := 0; index < 4; index++ {
		name := fmt.Sprintf("%s_%d", verifier.prefix, index)
		var registered struct{ ID int64 }
		if err := verifier.request("POST", "/api/register", "", map[string]any{"username": name, "password": verifier.password, "nickname": "local verifier"}, &registered); err != nil {
			return err
		}
		fmt.Printf("prepared player index=%d id=%d\n", index, registered.ID)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	db, err := database.NewMySQLDB(ctx, config.Load().Database)
	if err != nil {
		return errors.New("isolated database connection failed")
	}
	defer db.Close()
	hash, err := bcrypt.GenerateFromPassword([]byte(verifier.password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	for _, role := range []string{"gm", "operator"} {
		if _, err := db.ExecContext(ctx, "INSERT INTO admins(username,password_hash,display_name,role) VALUES(?,?,?,?)", verifier.prefix+"_"+role, string(hash), "local verification", role); err != nil {
			return errors.New("verification admin creation failed")
		}
	}
	fmt.Println("isolated accounts prepared; credentials remain in environment")
	return nil
}

func (verifier *verifier) login(index int) (*player, error) {
	var result struct {
		Token  string
		Player struct{ ID int64 }
	}
	if err := verifier.request("POST", "/api/login", "", map[string]any{"username": fmt.Sprintf("%s_%d", verifier.prefix, index), "password": verifier.password}, &result); err != nil {
		return nil, err
	}
	current := &player{id: result.Player.ID, token: result.Token}
	if err := verifier.connect(current); err != nil {
		return nil, err
	}
	return current, nil
}

func (verifier *verifier) connect(current *player) error {
	connection, response, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(verifier.endpoint, "http")+"/ws", http.Header{"Authorization": []string{"Bearer " + current.token}})
	if err != nil {
		if response != nil {
			response.Body.Close()
		}
		return errors.New("websocket handshake failed")
	}
	current.connection = connection
	var welcome struct{ Type string }
	_ = connection.SetReadDeadline(time.Now().Add(10 * time.Second))
	if err := connection.ReadJSON(&welcome); err != nil {
		return errors.New("websocket welcome missing")
	}
	if welcome.Type != "server.welcome" {
		return errors.New("invalid websocket welcome")
	}
	return nil
}

func (verifier *verifier) command(current *player, kind string, input any, output any) error {
	requestID := store.ID("verify")
	_ = current.connection.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if err := current.connection.WriteJSON(map[string]any{"schema_version": 2, "type": kind, "request_id": requestID, "operation_id": store.ID("operation"), "data": input}); err != nil {
		return errors.New("websocket write failed")
	}
	_ = current.connection.SetReadDeadline(time.Now().Add(10 * time.Second))
	for {
		var message struct {
			Request string `json:"request_id"`
			Code    int
			Data    json.RawMessage
		}
		if err := current.connection.ReadJSON(&message); err != nil {
			return errors.New("websocket response missing")
		}
		if message.Request != requestID {
			continue
		}
		if message.Code != 0 {
			return fmt.Errorf("command %s rejected code=%d", kind, message.Code)
		}
		if output != nil {
			return json.Unmarshal(message.Data, output)
		}
		return nil
	}
}

func (verifier *verifier) scenario() (string, error) {
	players := []*player{}
	defer func() {
		for _, current := range players {
			if current.connection != nil {
				current.connection.Close()
			}
		}
	}()
	for index := 0; index < 4; index++ {
		current, err := verifier.login(index)
		if err != nil {
			return "", err
		}
		players = append(players, current)
	}
	for _, current := range players {
		if err := verifier.command(current, "v2.match.enqueue", map[string]any{"plan": map[string]any{"operation": "training_ground", "difficulty": "normal", "rule_version": "training_ground.v1", "fill_policy": "public", "allow_partial": false}, "task": map[string]any{}}, nil); err != nil {
			return "", err
		}
	}
	var activity struct {
		Proposal struct {
			ID       string
			Revision int64
		}
		Run struct{ ID string }
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if err := verifier.request("GET", "/api/v2/me/activity", players[0].token, nil, &activity); err != nil {
			return "", err
		}
		if activity.Proposal.ID != "" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if activity.Proposal.ID == "" {
		return "", errors.New("proposal timeout")
	}
	var assignment struct {
		RunID string `json:"run_id"`
	}
	for _, current := range players {
		if err := verifier.command(current, "v2.match.proposal_confirm", map[string]any{"proposal_id": activity.Proposal.ID, "revision": activity.Proposal.Revision}, &assignment); err != nil {
			return "", err
		}
	}
	if assignment.RunID == "" {
		return "", errors.New("assignment missing")
	}
	players[0].connection.Close()
	if err := verifier.connect(players[0]); err != nil {
		return "", err
	}
	var state run.State
	if err := verifier.request("GET", "/api/v2/runs/"+assignment.RunID+"/snapshot", players[0].token, nil, &state); err != nil {
		return "", err
	}
	if state.ID != assignment.RunID {
		return "", errors.New("reconnect changed run")
	}
	if verifier.fault {
		db, err := database.NewMySQLDB(context.Background(), config.Load().Database)
		if err != nil {
			return "", errors.New("isolated fault connection failed")
		}
		_, err = db.Exec("DELETE FROM player_assets WHERE player_id=?", players[3].id)
		db.Close()
		if err != nil {
			return "", errors.New("asset failure fixture rejected")
		}
	}
	if err := verifier.events(assignment.RunID, players); err != nil {
		return "", err
	}
	deadline = time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		if verifier.fault {
			db, err := database.NewMySQLDB(context.Background(), config.Load().Database)
			if err != nil {
				return "", errors.New("isolated fault query failed")
			}
			var status string
			err = db.QueryRow("SELECT status FROM pve_pending_operations WHERE aggregate_id=? AND operation_type='settlement'", assignment.RunID).Scan(&status)
			db.Close()
			if err != nil {
				return "", errors.New("fault operation missing")
			}
			if status == "needs_repair" {
				fmt.Printf("fault reached needs_repair run=%s\n", assignment.RunID)
				return assignment.RunID, nil
			}
		}
		if err := verifier.request("GET", "/api/v2/runs/"+assignment.RunID+"/results", players[0].token, nil, &state); err != nil {
			return "", err
		}
		if state.Settled {
			fmt.Printf("scenario settled run=%s players=4 reconnect=true\n", state.ID)
			return state.ID, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return "", errors.New("settlement timeout")
}

func (verifier *verifier) events(runID string, players []*player) error {
	ids := []int64{}
	for _, current := range players {
		ids = append(ids, current.id)
	}
	sequence := int64(1)
	send := func(kind string, actor int64, target string) error {
		event := run.Event{EventID: store.ID("verify_event"), RunID: runID, Source: "pve_event_bot", SourceGeneration: 1, Sequence: sequence, SchemaVersion: 2, EventType: kind, ActorPlayerID: &actor, Contributors: ids, TargetID: target, OccurredAt: time.Now().UTC()}
		request, _ := http.NewRequest("POST", "http://127.0.0.1:8090/internal/v2/runs/"+runID+"/events", bytes.NewReader(store.JSON(event)))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-PVE-Test-Token", os.Getenv("PVE_TEST_EVENTS_TOKEN"))
		response, err := verifier.http.Do(request)
		if err != nil {
			return errors.New("internal event transport failed")
		}
		response.Body.Close()
		if response.StatusCode != 200 {
			return fmt.Errorf("internal event %s rejected status=%d", kind, response.StatusCode)
		}
		sequence++
		return nil
	}
	for _, current := range players {
		if err := send("loaded", current.id, ""); err != nil {
			return err
		}
	}
	for index := 0; index < 3; index++ {
		if err := send("kill", ids[index], store.ID("enemy")); err != nil {
			return err
		}
	}
	if err := send("interact", ids[3], "terminal"); err != nil {
		return err
	}
	return send("reach", ids[2], "exit")
}

func (verifier *verifier) soak(duration time.Duration) error {
	if duration < time.Second || duration > time.Hour {
		return errors.New("soak duration must be 1s through 1h")
	}
	started := time.Now()
	cycles := 0
	timings := []float64{}
	for time.Since(started) < duration {
		cycleStart := time.Now()
		if _, err := verifier.scenario(); err != nil {
			return err
		}
		cycles++
		timings = append(timings, time.Since(cycleStart).Seconds())
		if err := verifier.check(); err != nil {
			return err
		}
		time.Sleep(time.Second)
	}
	sort.Float64s(timings)
	fmt.Printf("scenario_latency_seconds p50=%.3f p95=%.3f max=%.3f\n", timings[len(timings)/2], timings[int(float64(len(timings)-1)*0.95)], timings[len(timings)-1])
	fmt.Printf("soak completed cycles=%d elapsed_seconds=%.2f scenario_failures=0\n", cycles, time.Since(started).Seconds())
	return nil
}

func (verifier *verifier) restoreAsset() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	db, err := database.NewMySQLDB(ctx, config.Load().Database)
	if err != nil {
		return errors.New("isolated fixture connection failed")
	}
	defer db.Close()
	result, err := db.ExecContext(ctx, `INSERT INTO player_assets(player_id,soft_currency)
SELECT p.id,COALESCE((SELECT SUM(l.delta) FROM asset_ledger l WHERE l.player_id=p.id),0) FROM players p
LEFT JOIN player_assets a ON a.player_id=p.id WHERE p.username=? AND a.player_id IS NULL
AND EXISTS (SELECT 1 FROM pve_run_participants rp JOIN pve_pending_operations o ON o.aggregate_id=rp.run_id WHERE rp.player_id=p.id AND o.status='needs_repair')`, verifier.prefix+"_3")
	if err != nil {
		return errors.New("isolated asset restoration failed")
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		return errors.New("expected one missing isolated asset")
	}
	fmt.Println("isolated missing asset restored from ledger; operator retry remains required")
	return nil
}

func (verifier *verifier) check() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	db, err := database.NewMySQLDB(ctx, config.Load().Database)
	if err != nil {
		return errors.New("isolated check connection failed")
	}
	defer db.Close()
	for _, query := range []string{
		"SELECT COUNT(*) FROM asset_ledger WHERE balance_after<>balance_before+delta",
		"SELECT COUNT(*) FROM pve_reward_grants g JOIN players p ON p.id=g.player_id LEFT JOIN asset_ledger l ON l.pve_grant_id=g.id WHERE g.status='granted' AND (l.id IS NULL OR l.delta<>g.amount)",
		"SELECT COUNT(*) FROM (SELECT p.id FROM players p JOIN player_assets a ON a.player_id=p.id LEFT JOIN asset_ledger l ON l.player_id=p.id GROUP BY p.id,a.soft_currency HAVING a.soft_currency<>COALESCE(SUM(l.delta),0)) mismatch",
	} {
		var count int
		if err := db.QueryRowContext(ctx, query).Scan(&count); err != nil {
			return errors.New("asset reconciliation query failed")
		}
		if count != 0 {
			return fmt.Errorf("asset reconciliation mismatches=%d", count)
		}
	}
	fmt.Println("isolated asset reconciliation mismatches=0")
	return nil
}

func fail(err error) { fmt.Fprintln(os.Stderr, "pve_verify:", err); os.Exit(1) }
