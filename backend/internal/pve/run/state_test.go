package run

import (
	"errors"
	"testing"
	"time"

	"game-realtime-gm/backend/internal/pve/store"
	"game-realtime-gm/backend/internal/pve/task"
)

func runningState(t *testing.T) *State {
	t.Helper()
	rules, err := task.Load("")
	if err != nil {
		t.Fatal(err)
	}
	return &State{ID: "test_run", Status: "running", Rules: rules, ReinforcementBudget: 3, Objectives: map[string]int64{}, Spawns: map[string]Spawn{}, Facts: map[string]bool{}, Participants: []Participant{{PlayerID: 1, Status: "participating", LifeStatus: "alive", TaskKey: "hunter", Compatibility: "compatible", TaskProgress: map[string]int64{}}, {PlayerID: 2, Status: "participating", LifeStatus: "alive", TaskKey: "technician", Compatibility: "compatible", TaskProgress: map[string]int64{}}, {PlayerID: 3, Status: "participating", LifeStatus: "alive", TaskKey: "other_map", Compatibility: "incompatible", TaskProgress: map[string]int64{}}, {PlayerID: 4, Status: "participating", LifeStatus: "alive", Compatibility: "none", TaskProgress: map[string]int64{}}}}
}
func TestSharedGoalsAndPersonalEligibility(t *testing.T) {
	state := runningState(t)
	actor := int64(4)
	for sequence := int64(1); sequence <= 3; sequence++ {
		event := Event{EventType: "kill", ActorPlayerID: &actor, TargetID: store.ID("enemy"), Sequence: sequence}
		if err := Apply(state, event, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	if state.Participants[0].TaskProgress["kills"] != 3 || len(state.Participants[2].TaskProgress) != 0 || len(state.Participants[3].TaskProgress) != 0 {
		t.Fatal("shared eligibility incorrect")
	}
	event := Event{EventType: "interact", ActorPlayerID: &actor, TargetID: "terminal", Contributors: []int64{2}, Sequence: 4}
	if err := Apply(state, event, time.Now()); err != nil {
		t.Fatal(err)
	}
	if state.Participants[1].TaskProgress["interact"] != 1 {
		t.Fatal("eligible collaborator missed")
	}
	event.EventType = "reach"
	event.TargetID = "exit"
	event.Sequence = 5
	if err := Apply(state, event, time.Now()); err != nil {
		t.Fatal(err)
	}
	if state.Status != "ending" || state.EndReason != "success" || state.FinalSequence != 5 {
		t.Fatal("shared terminal not fixed")
	}
}
func TestSpawnReservationPreventsPrematureWipe(t *testing.T) {
	state := runningState(t)
	for index := 1; index < len(state.Participants); index++ {
		state.Participants[index].Status = "left"
	}
	player := int64(1)
	for attempt := 0; attempt < 3; attempt++ {
		if err := Apply(state, Event{EventType: "death", ActorPlayerID: &player}, time.Now()); err != nil {
			t.Fatal(err)
		}
		id := store.ID("spawn")
		if err := Apply(state, Event{EventType: "reinforcement.reserve", ActorPlayerID: &player, TargetID: id}, time.Now()); err != nil {
			t.Fatal(err)
		}
		if state.Status != "running" {
			t.Fatal("pending spawn misclassified")
		}
		if err := Apply(state, Event{EventType: "reinforcement.spawned", ActorPlayerID: &player, TargetID: id}, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	if err := Apply(state, Event{EventType: "death", ActorPlayerID: &player, Sequence: 11}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if state.EndReason != "team_wipe" {
		t.Fatal("exhausted budget did not fail")
	}
}
func TestLastSpawnIsExclusive(t *testing.T) {
	state := runningState(t)
	state.ReinforcementUsed = 2
	state.Participants[0].LifeStatus = "dead"
	state.Participants[1].LifeStatus = "dead"
	first, second := int64(1), int64(2)
	if err := Apply(state, Event{EventType: "reinforcement.reserve", ActorPlayerID: &first, TargetID: "last_spawn"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := Apply(state, Event{EventType: "reinforcement.reserve", ActorPlayerID: &second, TargetID: "other_spawn"}, time.Now()); !errors.Is(err, store.Conflict) {
		t.Fatalf("expected last budget rejection: %v", err)
	}
}
func TestExitedParticipantCannotContribute(t *testing.T) {
	state := runningState(t)
	state.Participants[0].Status = "left"
	player := int64(1)
	if err := Apply(state, Event{EventType: "kill", ActorPlayerID: &player, TargetID: "enemy"}, time.Now()); !errors.Is(err, store.Forbidden) {
		t.Fatal("left actor accepted")
	}
	player = 2
	if err := Apply(state, Event{EventType: "kill", ActorPlayerID: &player, TargetID: "enemy"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if state.Participants[0].TaskProgress["kills"] != 0 {
		t.Fatal("left recipient advanced")
	}
}
func TestRepeatedEntityDoesNotAdvanceProgress(t *testing.T) {
	state := runningState(t)
	player := int64(1)
	for index := 0; index < 2; index++ {
		if err := Apply(state, Event{EventType: "kill", ActorPlayerID: &player, TargetID: "same_enemy"}, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	if state.Objectives["shared_kills"] != 1 {
		t.Fatal("entity repeated")
	}
}
