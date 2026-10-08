package run

import (
	"testing"
	"time"

	"game-realtime-gm/backend/internal/pve/store"
)

func TestAuditRescueRequiresCompletedReinforcement(t *testing.T) {
	state := runningState(t)
	target := state.Participants[0].PlayerID
	rescuer := state.Participants[3].PlayerID
	stamp := time.Now().UTC()
	for _, kind := range []string{"death", "reinforcement.reserve", "reinforcement.spawned"} {
		if err := Apply(state, Event{EventType: kind, ActorPlayerID: &target, TargetID: "reinforcement_audit"}, stamp); err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
	}
	event := Event{EventType: "rescue", ActorPlayerID: &rescuer, TargetID: "teammate", Payload: store.JSON(map[string]any{"action_id": "rescue_audit", "target_player_id": target, "effective_amount": 1, "reinforcement_action_id": "reinforcement_audit"})}
	if err := Apply(state, event, stamp); err != nil {
		t.Fatalf("successful reinforcement cannot be credited to rescuer: %v", err)
	}
	if err := Apply(state, event, stamp); err != nil {
		t.Fatal(err)
	}
	if state.Member(&rescuer).Contribution != 1 {
		t.Fatal("same reinforcement credited more than once")
	}
	if err := Apply(state, Event{EventType: "death", ActorPlayerID: &target}, stamp); err != nil {
		t.Fatal(err)
	}
	if err := Apply(state, event, stamp); err == nil {
		t.Fatal("an earlier reinforcement credited a newly dead target")
	}
	for _, kind := range []string{"reinforcement.reserve", "reinforcement.spawned"} {
		if err := Apply(state, Event{EventType: kind, ActorPlayerID: &target, TargetID: "reinforcement_later"}, stamp); err != nil {
			t.Fatal(err)
		}
	}
	if err := Apply(state, event, stamp); err == nil {
		t.Fatal("old rescue credited a different resurrection")
	}
}

func TestAuditActorReceivesEligibleProgressWithoutAssistants(t *testing.T) {
	state := runningState(t)
	actor := state.Participants[1].PlayerID
	if err := Apply(state, Event{EventType: "interact", ActorPlayerID: &actor, TargetID: "terminal"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if state.Member(&actor).TaskProgress["interact"] != 1 {
		t.Fatal("actor omitted from eligible progress")
	}
}

func TestAuditPrivateTaskFieldsAreClearedForTeammates(t *testing.T) {
	state := runningState(t)
	state.Participants[1].TaskCompleted = true
	view := state.ForPlayer(state.Participants[0].PlayerID)
	peer := view.Participants[1]
	if peer.TaskKey != "" || peer.TaskProgress != nil || peer.TaskCompleted || peer.Compatibility != "none" {
		t.Fatal("private task status exposed to teammate")
	}
	if !state.Participants[1].TaskCompleted || state.Participants[1].Compatibility != "compatible" {
		t.Fatal("privacy projection mutated authoritative state")
	}
}
