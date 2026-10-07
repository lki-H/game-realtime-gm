package v2_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"game-realtime-gm/backend/internal/pve/party"
	"game-realtime-gm/backend/internal/pve/run"
	"game-realtime-gm/backend/internal/pve/store"
)

func soloRun(t *testing.T, fixture *fixture, selections []string) string {
	t.Helper()
	for index, player := range fixture.players {
		selection := run.TaskSelection{}
		if index < len(selections) && selections[index] != "" {
			selection = run.TaskSelection{TaskKey: selections[index], TaskVersion: fixture.app.Runs.Rules.Version}
		}
		fixture.command(t, player, "v2.match.enqueue", map[string]any{"plan": party.Plan{Operation: "training_ground", Difficulty: "normal", FillPolicy: "public", AllowPartial: true}, "task": selection})
	}
	if err := fixture.app.Match.Match(fixture.ctx); err != nil {
		t.Fatal(err)
	}
	var id string
	if err := fixture.db.QueryRowContext(fixture.ctx, "SELECT id FROM pve_match_proposals WHERE status='pending'").Scan(&id); err != nil {
		t.Fatal(err)
	}
	var response struct {
		RunID string `json:"run_id"`
	}
	for _, player := range fixture.players {
		data := fixture.command(t, player, "v2.match.proposal_confirm", map[string]any{"proposal_id": id, "revision": 1})
		if err := json.Unmarshal(data, &response); err != nil {
			t.Fatal(err)
		}
	}
	for index, player := range fixture.players {
		fixture.event(t, response.RunID, int64(index+1), "loaded", player, "")
	}
	return response.RunID
}
func balances(t *testing.T, fixture *fixture, expected []int64) {
	t.Helper()
	for index, player := range fixture.players {
		var balance int64
		if err := fixture.db.QueryRowContext(fixture.ctx, "SELECT soft_currency FROM player_assets WHERE player_id=?", player).Scan(&balance); err != nil {
			t.Fatal(err)
		}
		if balance != expected[index] {
			t.Fatalf("balance[%d]=%d expected=%d", index, balance, expected[index])
		}
	}
}

func TestFourDifferentTasksIncludeIncompatible(t *testing.T) {
	fixture := setup(t)
	id := soloRun(t, fixture, []string{"hunter", "technician", "scout", "other_map"})
	for sequence := int64(5); sequence < 8; sequence++ {
		fixture.event(t, id, sequence, "kill", fixture.players[0], store.ID("enemy"))
	}
	fixture.event(t, id, 8, "interact", fixture.players[1], "terminal")
	fixture.event(t, id, 9, "reach", fixture.players[2], "exit")
	if err := fixture.app.Settlement.Settle(fixture.ctx, id); err != nil {
		t.Fatal(err)
	}
	state, err := fixture.app.Runs.Snapshot(fixture.ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	incompatible := state.Member(&fixture.players[3])
	if incompatible.Compatibility != "incompatible" || len(incompatible.TaskProgress) != 0 || incompatible.TaskCompleted {
		t.Fatal("incompatible task progressed")
	}
	balances(t, fixture, []int64{130, 140, 125, 100})
	var unlocked bool
	if err := fixture.db.QueryRowContext(fixture.ctx, "SELECT unlocked FROM pve_player_tasks WHERE player_id=? AND task_key='hunter_2'", fixture.players[0]).Scan(&unlocked); err != nil || !unlocked {
		t.Fatal("next task not unlocked")
	}
}

func TestFailureAndAbortSettlement(t *testing.T) {
	for _, reason := range []string{"timeout", "team_wipe", "system_abort"} {
		t.Run(reason, func(t *testing.T) {
			fixture := setup(t)
			id := soloRun(t, fixture, nil)
			fixture.event(t, id, 5, "kill", fixture.players[0], "enemy")
			switch reason {
			case "timeout":
				fixture.app.Runs.Now = func() time.Time { return time.Now().Add(time.Hour) }
				if err := fixture.app.Runs.Tick(fixture.ctx, id); err != nil {
					t.Fatal(err)
				}
			case "team_wipe":
				state, err := fixture.app.Runs.Snapshot(fixture.ctx, id)
				if err != nil {
					t.Fatal(err)
				}
				state.ReinforcementUsed = state.ReinforcementBudget
				if err := store.Transaction(fixture.ctx, fixture.db, func(transaction *sql.Tx) error { return run.Save(fixture.ctx, transaction, state) }); err != nil {
					t.Fatal(err)
				}
				for index, player := range fixture.players {
					fixture.event(t, id, int64(index+6), "death", player, "")
				}
			default:
				fixture.event(t, id, 6, "system_abort", fixture.players[0], "")
			}
			if err := fixture.app.Settlement.Settle(fixture.ctx, id); err != nil {
				t.Fatal(err)
			}
			state, err := fixture.app.Runs.Snapshot(fixture.ctx, id)
			if err != nil || state.EndReason != reason {
				t.Fatalf("terminal=%+v err=%v", state, err)
			}
			amount := int64(20)
			if reason == "system_abort" {
				amount = 0
			}
			balances(t, fixture, []int64{amount, amount, amount, amount})
		})
	}
}

func TestReconnectGenerationAndPermanentExit(t *testing.T) {
	fixture := setup(t)
	id := soloRun(t, fixture, []string{"hunter"})
	player := fixture.players[0]
	if err := fixture.app.Runs.Connect(fixture.ctx, player, "old", true); err != nil {
		t.Fatal(err)
	}
	if err := fixture.app.Runs.Connect(fixture.ctx, player, "old", false); err != nil {
		t.Fatal(err)
	}
	if err := fixture.app.Runs.Connect(fixture.ctx, player, "new", true); err != nil {
		t.Fatal(err)
	}
	if err := fixture.app.Runs.Connect(fixture.ctx, player, "old", false); err != nil {
		t.Fatal(err)
	}
	state, err := fixture.app.Runs.Snapshot(fixture.ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	member := state.Member(&player)
	if !member.Connected || member.ConnectionID != "new" {
		t.Fatal("stale disconnect overwrote new connection")
	}
	attempt := member.TaskAttemptID
	fixture.command(t, player, "v2.run.leave", map[string]any{"run_id": id})
	if err := fixture.app.Runs.Connect(fixture.ctx, player, "after_exit", true); err != nil {
		t.Fatal(err)
	}
	fixture.event(t, id, 5, "kill", fixture.players[1], "enemy")
	state, err = fixture.app.Runs.Snapshot(fixture.ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	member = state.Member(&player)
	if member.Status != "left" || member.TaskAttemptID != attempt || member.TaskProgress["kills"] != 0 {
		t.Fatal("permanent exit regained eligibility")
	}
	if _, err := fixture.app.Command(fixture.ctx, fixture.players[1], store.ID("cmd"), "v2.run.finish", store.JSON(map[string]any{"run_id": id})); !errors.Is(err, store.NotFound) {
		t.Fatal("player finish allowed")
	}
}

func TestGapDeadlineAndOldSource(t *testing.T) {
	fixture := setup(t)
	id := soloRun(t, fixture, nil)
	event := run.Event{EventID: store.ID("event"), RunID: id, Source: "pve_event_bot", SourceGeneration: 2, SchemaVersion: 2, Sequence: 5, EventType: "kill", ActorPlayerID: &fixture.players[0], TargetID: "enemy", OccurredAt: time.Now().UTC()}
	if _, err := fixture.app.Runs.ApplyEvent(fixture.ctx, event); !errors.Is(err, store.Forbidden) {
		t.Fatal("invalid source generation accepted")
	}
	event.SourceGeneration = 1
	event.Sequence = 6
	result, err := fixture.app.Runs.ApplyEvent(fixture.ctx, event)
	if err != nil || !result.GapDetected || result.ReceivedSequence != 4 {
		t.Fatal("gap watermark advanced")
	}
	fixture.app.Runs.Now = func() time.Time { return time.Now().Add(time.Hour) }
	if err := fixture.app.Runs.Tick(fixture.ctx, id); err != nil {
		t.Fatal(err)
	}
	state, err := fixture.app.Runs.Snapshot(fixture.ctx, id)
	if err != nil || state.EndReason != "event_gap" || state.FinalSequence != 4 {
		t.Fatal("gap timeout did not preserve known boundary")
	}
	if err := fixture.app.Settlement.Settle(fixture.ctx, id); err != nil {
		t.Fatal(err)
	}
}

func TestConfirmationDeadlinePartialStartAndNeedsRepair(t *testing.T) {
	fixture := setup(t)
	fixture.command(t, fixture.players[0], "v2.match.enqueue", map[string]any{"plan": party.Plan{Operation: "training_ground", Difficulty: "normal", FillPolicy: "public", AllowPartial: true}})
	if err := fixture.app.Match.Match(fixture.ctx); err != nil {
		t.Fatal(err)
	}
	var proposals int
	_ = fixture.db.QueryRowContext(fixture.ctx, "SELECT COUNT(*) FROM pve_match_proposals").Scan(&proposals)
	if proposals != 0 {
		t.Fatal("partial start before consent timeout")
	}
	fixture.app.Match.Now = func() time.Time { return time.Now().Add(time.Minute) }
	if err := fixture.app.Match.Match(fixture.ctx); err != nil {
		t.Fatal(err)
	}
	fixture.app.Match.Now = func() time.Time { return time.Now().Add(time.Hour) }
	if err := fixture.app.Match.Expire(fixture.ctx); err != nil {
		t.Fatal(err)
	}
	var locks int
	_ = fixture.db.QueryRowContext(fixture.ctx, "SELECT COUNT(*) FROM pve_player_activity_locks").Scan(&locks)
	if locks != 0 {
		t.Fatal("confirmation timeout retains activity")
	}
	if _, err := fixture.db.ExecContext(fixture.ctx, "INSERT INTO pve_pending_operations(operation_id,operation_type,aggregate_id,payload,attempts,next_attempt_at) VALUES('repair_test','settlement','missing_run','{}',4,UTC_TIMESTAMP(3))"); err != nil {
		t.Fatal(err)
	}
	fixture.app.Worker.Tick = nil
	if err := fixture.app.Worker.Once(fixture.ctx); err != nil {
		t.Fatal(err)
	}
	var status string
	if err := fixture.db.QueryRowContext(fixture.ctx, "SELECT status FROM pve_pending_operations WHERE operation_id='repair_test'").Scan(&status); err != nil || status != "needs_repair" {
		t.Fatal("unbounded worker retries")
	}
}

func TestWholeFriendTicketCannotSplitOrKickDuringRun(t *testing.T) {
	fixture := setup(t)
	id, group := fixture.assigned(t)
	for index, player := range fixture.players {
		fixture.event(t, id, int64(index+1), "loaded", player, "")
	}
	for _, kind := range []string{"v2.party.leave", "v2.party.invite", "v2.party.transfer", "v2.party.kick"} {
		if _, err := fixture.app.Command(fixture.ctx, fixture.players[0], store.ID("cmd"), kind, store.JSON(map[string]any{"party_id": group, "player_id": fixture.players[2]})); err == nil {
			t.Fatalf("run party action allowed: %s", kind)
		}
	}
	if _, err := fixture.app.Command(fixture.ctx, fixture.players[1], store.ID("cmd"), "v2.match.enqueue", store.JSON(map[string]any{"plan": party.Plan{Operation: "training_ground", Difficulty: "normal", FillPolicy: "public", AllowPartial: true}})); err == nil {
		t.Fatal("friend ticket split")
	}
	fixture.command(t, fixture.players[0], "v2.run.leave", map[string]any{"run_id": id})
	for sequence := int64(5); sequence < 8; sequence++ {
		fixture.event(t, id, sequence, "kill", fixture.players[2], store.ID("enemy"))
	}
	fixture.event(t, id, 8, "interact", fixture.players[2], "terminal")
	fixture.event(t, id, 9, "reach", fixture.players[2], "exit")
	if err := fixture.app.Settlement.Settle(fixture.ctx, id); err != nil {
		t.Fatal(err)
	}
	var balance int64
	_ = fixture.db.QueryRowContext(fixture.ctx, "SELECT soft_currency FROM player_assets WHERE player_id=?", fixture.players[0]).Scan(&balance)
	if balance != 0 {
		t.Fatal("voluntary exit got later success reward")
	}
}

func TestAssetLedgerReconcilesWithGrantedRewards(t *testing.T) {
	fixture := setup(t)
	id := soloRun(t, fixture, nil)
	for sequence := int64(5); sequence < 8; sequence++ {
		fixture.event(t, id, sequence, "kill", fixture.players[0], store.ID("enemy"))
	}
	fixture.event(t, id, 8, "interact", fixture.players[1], "terminal")
	fixture.event(t, id, 9, "reach", fixture.players[2], "exit")
	if err := fixture.app.Worker.Once(fixture.ctx); err != nil {
		t.Fatal(err)
	}
	var grants, ledger, balance int64
	if err := fixture.db.QueryRowContext(fixture.ctx, "SELECT COALESCE(SUM(amount),0) FROM pve_reward_grants").Scan(&grants); err != nil {
		t.Fatal(err)
	}
	_ = fixture.db.QueryRowContext(fixture.ctx, "SELECT COALESCE(SUM(delta),0) FROM asset_ledger WHERE pve_run_id=?", id).Scan(&ledger)
	for _, player := range fixture.players {
		var current int64
		_ = fixture.db.QueryRowContext(fixture.ctx, "SELECT soft_currency FROM player_assets WHERE player_id=?", player).Scan(&current)
		balance += current
	}
	if grants != ledger || ledger != balance {
		t.Fatal("reward ledger balance mismatch")
	}
	if err := fixture.app.Worker.Once(context.Background()); err != nil {
		t.Fatal(err)
	}
}
