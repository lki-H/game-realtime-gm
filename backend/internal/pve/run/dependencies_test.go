package run

import (
	"game-realtime-gm/backend/internal/pve/task"
	"testing"
	"time"
)

func TestOneEventCannotAdvanceNewlyUnlockedObjective(t *testing.T) {
	state := runningState(t)
	state.Rules.Objectives = []task.Objective{{Key: "first", Type: "kill", Required: 1, Scope: "team"}, {Key: "second", Type: "kill", Required: 1, Scope: "team", DependsOn: []string{"first"}}}
	player := int64(1)
	if err := Apply(state, Event{EventType: "kill", ActorPlayerID: &player, TargetID: "enemy_first", Sequence: 1}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if state.Objectives["first"] != 1 || state.Objectives["second"] != 0 {
		t.Fatal("event cascaded into inactive objective")
	}
	if err := Apply(state, Event{EventType: "kill", ActorPlayerID: &player, TargetID: "enemy_second", Sequence: 2}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if state.EndReason != "success" {
		t.Fatal("next event did not finish chain")
	}
}

func TestDistinctPlayersCanPerformSameInteraction(t *testing.T) {
	state := runningState(t)
	state.Rules.Tasks = append(state.Rules.Tasks, task.Definition{Key: "self_terminal", Operation: "training_ground", Objectives: []task.Objective{{Key: "use", Type: "interact", Required: 1, Scope: "self", TargetID: "terminal"}}})
	for index := 0; index < 2; index++ {
		state.Participants[index].TaskKey = "self_terminal"
		state.Participants[index].Compatibility = "compatible"
	}
	for _, player := range []int64{1, 2} {
		if err := Apply(state, Event{EventType: "interact", ActorPlayerID: &player, TargetID: "terminal"}, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	for index := 0; index < 2; index++ {
		if state.Participants[index].TaskProgress["use"] != 1 {
			t.Fatal("shared object blocked individual opportunity")
		}
	}
}
