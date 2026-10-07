package v2_test

import (
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"game-realtime-gm/backend/internal/pve/party"
	"game-realtime-gm/backend/internal/pve/run"
	"game-realtime-gm/backend/internal/pve/store"
	"github.com/go-sql-driver/mysql"
)

func TestReadyVersionsAndCancelBeforePlanChange(t *testing.T) {
	fixture := setup(t)
	created := fixture.command(t, fixture.players[0], "v2.party.create", map[string]any{})
	var group party.Party
	if err := json.Unmarshal(created, &group); err != nil {
		t.Fatal(err)
	}
	ready := map[string]any{"party_id": group.ID, "ready": true, "roster_version": group.RosterVersion, "plan_version": group.PlanVersion, "selection_version": 1}
	fixture.command(t, fixture.players[0], "v2.party.ready", ready)
	fixture.command(t, fixture.players[0], "v2.party.selection", map[string]any{"party_id": group.ID, "task_key": "hunter", "task_version": fixture.app.Runs.Rules.Version})
	if _, err := fixture.app.Command(fixture.ctx, fixture.players[0], store.ID("cmd"), "v2.party.ready", store.JSON(ready)); !errors.Is(err, store.Conflict) {
		t.Fatalf("old selection ready accepted: %v", err)
	}
	ready["selection_version"] = 2
	fixture.command(t, fixture.players[0], "v2.party.ready", ready)
	fixture.command(t, fixture.players[0], "v2.match.enqueue", map[string]any{"party_id": group.ID})
	plan := map[string]any{"party_id": group.ID, "plan": party.Plan{Operation: "training_ground", Difficulty: "normal", FillPolicy: "public", AllowPartial: true}}
	if _, err := fixture.app.Command(fixture.ctx, fixture.players[0], store.ID("cmd"), "v2.party.plan_update", store.JSON(plan)); !errors.Is(err, store.Conflict) {
		t.Fatalf("queued plan changed: %v", err)
	}
	operation := store.ID("cancel")
	for attempt := 0; attempt < 2; attempt++ {
		if _, err := fixture.app.Command(fixture.ctx, fixture.players[0], operation, "v2.match.cancel", store.JSON(map[string]any{})); err != nil {
			t.Fatal(err)
		}
	}
	fixture.command(t, fixture.players[0], "v2.party.plan_update", plan)
	if _, err := fixture.app.Command(fixture.ctx, fixture.players[0], store.ID("cmd"), "v2.party.ready", store.JSON(ready)); !errors.Is(err, store.Conflict) {
		t.Fatalf("old plan ready accepted: %v", err)
	}
}

func TestNoFillAndFullPartyProposeImmediately(t *testing.T) {
	for _, fill := range []string{"no_fill", "public"} {
		t.Run(fill, func(t *testing.T) {
			fixture := setup(t)
			players := fixture.players
			if fill == "no_fill" {
				players = players[:1]
			}
			for _, player := range players {
				fixture.command(t, player, "v2.match.enqueue", map[string]any{"plan": party.Plan{Operation: "training_ground", Difficulty: "normal", FillPolicy: fill, AllowPartial: false}})
			}
			if err := fixture.app.Match.Match(fixture.ctx); err != nil {
				t.Fatal(err)
			}
			var proposals int
			if err := fixture.db.QueryRowContext(fixture.ctx, "SELECT COUNT(*) FROM pve_match_proposals WHERE status='pending'").Scan(&proposals); err != nil || proposals < 1 {
				t.Fatalf("unnecessary fill wait: proposals=%d err=%v", proposals, err)
			}
		})
	}
}

func TestProposalRoundsBoundAutomaticRequeue(t *testing.T) {
	fixture := setup(t)
	fixture.app.Match.MaxProposalRounds = 1
	for _, player := range fixture.players {
		fixture.command(t, player, "v2.match.enqueue", map[string]any{"plan": party.Plan{Operation: "training_ground", Difficulty: "normal", FillPolicy: "public", AllowPartial: true}})
	}
	if err := fixture.app.Match.Match(fixture.ctx); err != nil {
		t.Fatal(err)
	}
	var proposal string
	if err := fixture.db.QueryRowContext(fixture.ctx, "SELECT id FROM pve_match_proposals WHERE status='pending'").Scan(&proposal); err != nil {
		t.Fatal(err)
	}
	fixture.command(t, fixture.players[0], "v2.match.proposal_reject", map[string]any{"proposal_id": proposal, "revision": 1})
	var locks int
	if err := fixture.db.QueryRowContext(fixture.ctx, "SELECT COUNT(*) FROM pve_player_activity_locks").Scan(&locks); err != nil || locks != 0 {
		t.Fatalf("unbounded requeue locks=%d err=%v", locks, err)
	}
}

func TestOldLoadingCallbackPreservesNewActivityOwner(t *testing.T) {
	fixture := setup(t)
	id, _ := fixture.assigned(t)
	fixture.app.Match.Now = func() time.Time { return time.Now().Add(time.Hour) }
	if err := fixture.app.Match.LoadingExpired(fixture.ctx, id); err != nil {
		t.Fatal(err)
	}
	fixture.app.Match.Now = time.Now
	fixture.command(t, fixture.players[2], "v2.match.enqueue", map[string]any{"plan": party.Plan{Operation: "training_ground", Difficulty: "normal", FillPolicy: "public", AllowPartial: true}})
	var before, after string
	fixture.db.QueryRowContext(fixture.ctx, "SELECT activity_id FROM pve_player_activity_locks WHERE player_id=?", fixture.players[2]).Scan(&before)
	if err := fixture.app.Match.LoadingExpired(fixture.ctx, id); err != nil {
		t.Fatal(err)
	}
	fixture.db.QueryRowContext(fixture.ctx, "SELECT activity_id FROM pve_player_activity_locks WHERE player_id=?", fixture.players[2]).Scan(&after)
	if before == "" || before != after {
		t.Fatal("old callback changed new activity")
	}
}

func TestEventTransactionFailureDoesNotAdvanceWatermark(t *testing.T) {
	fixture := setup(t)
	id, _ := fixture.assigned(t)
	for index, player := range fixture.players {
		fixture.event(t, id, int64(index+1), "loaded", player, "")
	}
	if _, err := fixture.db.ExecContext(fixture.ctx, "RENAME TABLE pve_event_applications TO pve_r1_unavailable_event_applications"); err != nil {
		t.Fatal(err)
	}
	restored := false
	t.Cleanup(func() {
		if !restored {
			fixture.db.Exec("RENAME TABLE pve_r1_unavailable_event_applications TO pve_event_applications")
		}
	})
	event := run.Event{EventID: store.ID("event"), Source: "pve_event_bot", SourceGeneration: 1, RunID: id, Sequence: 5, SchemaVersion: 2, EventType: "kill", ActorPlayerID: &fixture.players[0], Contributors: fixture.players, TargetID: "failed_enemy", OccurredAt: time.Now().UTC()}
	if _, err := fixture.app.Runs.ApplyEvent(fixture.ctx, event); err == nil {
		t.Fatal("injected event did not fail")
	}
	state, err := fixture.app.Runs.Snapshot(fixture.ctx, id)
	if err != nil || state.AppliedSequence != 4 || state.ReceivedSequence != 4 {
		t.Fatalf("uncommitted watermark advanced: %+v %v", state, err)
	}
	if _, err := fixture.db.ExecContext(fixture.ctx, "RENAME TABLE pve_r1_unavailable_event_applications TO pve_event_applications"); err != nil {
		t.Fatal(err)
	}
	restored = true
	if _, err := fixture.app.Runs.ApplyEvent(fixture.ctx, event); err != nil {
		t.Fatal(err)
	}
}

func TestDeadlockRetryRollsBackBeforeReapplying(t *testing.T) {
	fixture := setup(t)
	attempts := 0
	err := store.Transaction(fixture.ctx, fixture.db, func(transaction *sql.Tx) error {
		attempts++
		if _, err := transaction.ExecContext(fixture.ctx, "UPDATE player_assets SET soft_currency=soft_currency+1 WHERE player_id=?", fixture.players[0]); err != nil {
			return err
		}
		if attempts < 3 {
			return &mysql.MySQLError{Number: 1213, Message: "injected deadlock"}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var balance int64
	if err := fixture.db.QueryRowContext(fixture.ctx, "SELECT soft_currency FROM player_assets WHERE player_id=?", fixture.players[0]).Scan(&balance); err != nil || balance != 1 || attempts != 3 {
		t.Fatalf("retry repeated write: balance=%d attempts=%d err=%v", balance, attempts, err)
	}
}
