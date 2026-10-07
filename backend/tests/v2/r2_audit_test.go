package v2_test

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"game-realtime-gm/backend/internal/pve/party"
	"game-realtime-gm/backend/internal/pve/run"
	"game-realtime-gm/backend/internal/pve/store"
)

func recruitedPair(t *testing.T, fixture *fixture) string {
	t.Helper()
	var group party.Party
	if err := json.Unmarshal(fixture.command(t, fixture.players[0], "v2.party.create", map[string]any{}), &group); err != nil {
		t.Fatal(err)
	}
	var post struct {
		ID string `json:"post_id"`
	}
	if err := json.Unmarshal(fixture.command(t, fixture.players[0], "v2.recruitment.publish", map[string]any{"party_id": group.ID}), &post); err != nil {
		t.Fatal(err)
	}
	var application struct {
		ID int64 `json:"application_id"`
	}
	if err := json.Unmarshal(fixture.command(t, fixture.players[1], "v2.recruitment.apply", map[string]any{"post_id": post.ID}), &application); err != nil {
		t.Fatal(err)
	}
	fixture.command(t, fixture.players[0], "v2.recruitment.respond", map[string]any{"application_id": application.ID, "accept": true})
	return group.ID
}

func TestR2AuditRecruitmentOccupancyIsExclusive(t *testing.T) {
	fixture := setup(t)
	recruitedPair(t, fixture)
	for _, kind := range []string{"v2.party.create", "v2.match.enqueue"} {
		input := map[string]any{}
		if kind == "v2.match.enqueue" {
			input["plan"] = party.Plan{Operation: "training_ground", Difficulty: "normal", FillPolicy: "public", AllowPartial: true}
		}
		if _, err := fixture.app.Command(fixture.ctx, fixture.players[1], store.ID("audit"), kind, store.JSON(input)); !errors.Is(err, store.Conflict) {
			t.Fatalf("reserved guest entered %s: %v", kind, err)
		}
	}
}

func TestR2AuditCompletedSelectionCanBeClearedForNextQueue(t *testing.T) {
	fixture := setup(t)
	id, room := finishOldRun(t, fixture)
	group, err := fixture.app.Party.Snapshot(fixture.ctx, room, fixture.players[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, member := range group.Members {
		if len(member.TaskSelection) > 0 && string(member.TaskSelection) != "{}" {
			t.Fatalf("settled completed selection still blocks next queue: run=%s selection=%s", id, member.TaskSelection)
		}
	}
}

func TestR2AuditProgressCompletionDoesNotRegress(t *testing.T) {
	fixture := setup(t)
	id := productRun(t, fixture, []string{"daily_support"})
	fixture.event(t, id, 2, "interact", fixture.players[0], "terminal")
	fixture.app.Worker.Tick = nil
	if err := fixture.app.Worker.Once(fixture.ctx); err != nil {
		t.Fatal(err)
	}
	fixture.event(t, id, 3, "kill", fixture.players[0], "enemy")
	var content []byte
	if err := fixture.db.QueryRowContext(fixture.ctx, "SELECT progress FROM pve_task_periods WHERE player_id=?", fixture.players[0]).Scan(&content); err != nil {
		t.Fatal(err)
	}
	var counts map[string]int64
	if err := json.Unmarshal(content, &counts); err != nil {
		t.Fatal(err)
	}
	if counts["assist"] != 1 {
		t.Fatalf("completion progress regressed: %s", content)
	}
}

func TestR2AuditSameRunAndRegroupVersionMutation(t *testing.T) {
	fixture := setup(t)
	id, room := finishOldRun(t, fixture)
	var proposal struct {
		ID string `json:"proposal_id"`
	}
	input := map[string]any{"run_id": id, "owner_id": fixture.players[0], "player_ids": fixture.players[:2]}
	if err := json.Unmarshal(fixture.command(t, fixture.players[0], "v2.party.regroup_propose", input), &proposal); err != nil {
		t.Fatal(err)
	}
	fixture.command(t, fixture.players[0], "v2.party.regroup_respond", map[string]any{"proposal_id": proposal.ID, "revision": 1, "accept": true})
	fixture.command(t, fixture.players[1], "v2.party.regroup_respond", map[string]any{"proposal_id": proposal.ID, "revision": 1, "accept": true})
	snapshot, err := fixture.app.Party.RegroupSnapshot(fixture.ctx, proposal.ID, fixture.players[0])
	if err != nil || snapshot["party_id"] != room {
		t.Fatalf("same-party regroup changed room: %v %v", snapshot, err)
	}
}

func TestR2AuditRestartClearsRecruitReady(t *testing.T) {
	fixture := setup(t)
	room := recruitedPair(t, fixture)
	group, err := fixture.app.Party.Snapshot(fixture.ctx, room, fixture.players[0])
	if err != nil {
		t.Fatal(err)
	}
	fixture.command(t, fixture.players[1], "v2.recruitment.ready", map[string]any{"party_id": room, "ready": true, "roster_version": group.RosterVersion, "plan_version": group.PlanVersion, "selection_version": 1})
	if err := fixture.app.Recover(fixture.ctx); err != nil {
		t.Fatal(err)
	}
	var ready bool
	if err := fixture.db.QueryRowContext(fixture.ctx, "SELECT ready FROM pve_recruitment_preferences WHERE player_id=?", fixture.players[1]).Scan(&ready); err != nil {
		t.Fatal(err)
	}
	if ready {
		t.Fatal("restart preserved guest consent")
	}
}

func TestR2AuditSupportTargetsAndActorQualification(t *testing.T) {
	fixture := setup(t)
	id := productRun(t, fixture, []string{"medic", ""})
	event := run.Event{EventID: store.ID("audit"), RunID: id, Source: "pve_event_bot", SourceGeneration: 1, Sequence: 3, SchemaVersion: 2, EventType: "damage", ActorPlayerID: &fixture.players[0], TargetID: "self", OccurredAt: time.Now().UTC(), Payload: store.JSON(map[string]any{"action_id": "damage_action_1", "target_player_id": fixture.players[0], "effective_amount": 10})}
	if _, err := fixture.app.Runs.ApplyEvent(fixture.ctx, event); !errors.Is(err, store.Forbidden) {
		t.Fatalf("self damage accepted: %v", err)
	}
}

func TestR2AuditPausedTaskCanBeReselected(t *testing.T) {
	fixture := setup(t)
	fixture.command(t, fixture.players[0], "v2.party.create", map[string]any{})
	fixture.command(t, fixture.players[0], "v2.task.pause", map[string]any{"task_key": "hunter", "task_version": "training_ground.v1"})
	if _, err := fixture.app.Command(fixture.ctx, fixture.players[0], store.ID("audit"), "v2.party.selection", store.JSON(map[string]any{"party_id": "missing", "task_key": "hunter", "task_version": "training_ground.v1"})); !errors.Is(err, store.NotFound) {
		t.Fatalf("unexpected missing party result=%v", err)
	}
	var partyID string
	fixture.db.QueryRowContext(fixture.ctx, "SELECT party_id FROM pve_party_members WHERE player_id=? AND status='active'", fixture.players[0]).Scan(&partyID)
	if _, err := fixture.app.Command(fixture.ctx, fixture.players[0], store.ID("audit"), "v2.party.selection", store.JSON(map[string]any{"party_id": partyID, "task_key": "hunter", "task_version": "training_ground.v1"})); err != nil {
		t.Fatal(err)
	}
	var paused bool
	fixture.db.QueryRowContext(fixture.ctx, "SELECT paused FROM pve_task_pauses WHERE player_id=? AND task_key='hunter' AND task_version='training_ground.v1'", fixture.players[0]).Scan(&paused)
	if paused {
		t.Fatal("reselect did not resume paused task")
	}
}
