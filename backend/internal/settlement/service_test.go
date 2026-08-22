package settlement

import (
	"context"
	"errors"
	"testing"

	"game-realtime-gm/backend/internal/mission"
)

func TestValidNonce(t *testing.T) {
	valid := []string{
		"settle_20260822_0001",
		"nonce-1234567890-abcd",
	}
	for _, nonce := range valid {
		if !validNonce(nonce) {
			t.Fatalf("expected nonce %q to be valid", nonce)
		}
	}

	invalid := []string{
		"short",
		"settle:20260822:0001",
		"包含中文的nonce_0001",
	}
	for _, nonce := range invalid {
		if validNonce(nonce) {
			t.Fatalf("expected nonce %q to be invalid", nonce)
		}
	}
}

func TestContainsPlayer(t *testing.T) {
	playerIDs := []int64{1, 2, 3}
	if !containsPlayer(playerIDs, 2) {
		t.Fatal("player 2 should be found")
	}
	if containsPlayer(playerIDs, 9) {
		t.Fatal("player 9 should not be found")
	}
}

func TestCalculateScore(t *testing.T) {
	tests := []struct {
		seconds  int64
		expected int64
	}{
		{seconds: 0, expected: 1000},
		{seconds: 30, expected: 970},
		{seconds: 1000, expected: 0},
		{seconds: 1200, expected: 0},
	}

	for _, test := range tests {
		if actual := calculateScore(test.seconds); actual != test.expected {
			t.Fatalf("seconds=%d expected=%d actual=%d", test.seconds, test.expected, actual)
		}
	}
}

func TestCreateRejectsMissionSquadChanged(t *testing.T) {
	missionManager, missionState := finishedMission(t)
	service := NewService(nil, missionManager)

	_, err := service.Create(
		context.Background(),
		1,
		"squad_2",
		missionState.ID,
		"settle_20260822_0001",
	)
	if !errors.Is(err, ErrMissionSquadChanged) {
		t.Fatalf("expected ErrMissionSquadChanged, got %v", err)
	}
}

func TestCreateRejectsPlayerOutsideMission(t *testing.T) {
	missionManager, missionState := finishedMission(t)
	service := NewService(nil, missionManager)

	_, err := service.Create(
		context.Background(),
		9,
		missionState.SquadID,
		missionState.ID,
		"settle_20260822_0002",
	)
	if !errors.Is(err, ErrPlayerNotInMission) {
		t.Fatalf("expected ErrPlayerNotInMission, got %v", err)
	}
}

func finishedMission(t *testing.T) (*mission.Manager, *mission.Instance) {
	t.Helper()
	manager := mission.NewManager()
	created, err := manager.Create("training_ground", "squad_1", []int64{1, 2})
	if err != nil {
		t.Fatalf("create mission failed: %v", err)
	}
	if _, err := manager.Transition(created.ID, mission.StatusReady); err != nil {
		t.Fatalf("ready mission failed: %v", err)
	}
	if _, err := manager.Transition(created.ID, mission.StatusRunning); err != nil {
		t.Fatalf("start mission failed: %v", err)
	}
	finished, err := manager.Transition(created.ID, mission.StatusFinished)
	if err != nil {
		t.Fatalf("finish mission failed: %v", err)
	}
	return manager, finished
}
