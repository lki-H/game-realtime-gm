package mission

import (
	"errors"
	"testing"
)

func TestManagerMissionHappyPath(t *testing.T) {
	manager := NewManager()

	created, err := manager.Create("training_ground", "squad_1", []int64{1, 2})
	if err != nil {
		t.Fatalf("create mission failed: %v", err)
	}
	if created.Status != StatusWaiting {
		t.Fatalf("expected waiting, got %s", created.Status)
	}

	ready, err := manager.Transition(created.ID, StatusReady)
	if err != nil {
		t.Fatalf("mark ready failed: %v", err)
	}
	if ready.ReadyAt == nil {
		t.Fatal("ready_at should be set")
	}

	running, err := manager.Transition(created.ID, StatusRunning)
	if err != nil {
		t.Fatalf("start mission failed: %v", err)
	}
	if running.StartedAt == nil {
		t.Fatal("started_at should be set")
	}

	finished, err := manager.Transition(created.ID, StatusFinished)
	if err != nil {
		t.Fatalf("finish mission failed: %v", err)
	}
	if finished.FinishedAt == nil {
		t.Fatal("finished_at should be set")
	}
}

func TestManagerMissionCancelPath(t *testing.T) {
	manager := NewManager()
	created, err := manager.Create("training_ground", "squad_1", []int64{1})
	if err != nil {
		t.Fatalf("create mission failed: %v", err)
	}

	canceled, err := manager.Transition(created.ID, StatusCanceled)
	if err != nil {
		t.Fatalf("cancel mission failed: %v", err)
	}
	if canceled.Status != StatusCanceled {
		t.Fatalf("expected canceled, got %s", canceled.Status)
	}
}

func TestManagerRejectsInvalidTransitions(t *testing.T) {
	manager := NewManager()
	created, err := manager.Create("training_ground", "squad_1", []int64{1})
	if err != nil {
		t.Fatalf("create mission failed: %v", err)
	}

	if _, err := manager.Transition(created.ID, StatusRunning); !errors.Is(err, ErrInvalidStateTransition) {
		t.Fatalf("waiting -> running should fail, got %v", err)
	}
	if _, err := manager.Transition(created.ID, StatusFinished); !errors.Is(err, ErrInvalidStateTransition) {
		t.Fatalf("waiting -> finished should fail, got %v", err)
	}

	if _, err := manager.Transition(created.ID, StatusReady); err != nil {
		t.Fatalf("waiting -> ready failed: %v", err)
	}
	if _, err := manager.Transition(created.ID, StatusRunning); err != nil {
		t.Fatalf("ready -> running failed: %v", err)
	}
	if _, err := manager.Transition(created.ID, StatusFinished); err != nil {
		t.Fatalf("running -> finished failed: %v", err)
	}
	if _, err := manager.Transition(created.ID, StatusRunning); !errors.Is(err, ErrInvalidStateTransition) {
		t.Fatalf("finished -> running should fail, got %v", err)
	}
}

func TestManagerRejectsSecondActiveMissionForSquad(t *testing.T) {
	manager := NewManager()
	if _, err := manager.Create("training_ground", "squad_1", []int64{1, 2}); err != nil {
		t.Fatalf("first create failed: %v", err)
	}
	if _, err := manager.Create("training_ground", "squad_1", []int64{1, 2}); !errors.Is(err, ErrSquadAlreadyInMission) {
		t.Fatalf("second active mission should fail, got %v", err)
	}
}
