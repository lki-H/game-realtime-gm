package settlement

import (
	"context"
	"errors"
	"math"
	"testing"

	"game-realtime-gm/backend/internal/mission"
)

func TestValidRequestKey(t *testing.T) {
	valid := []string{
		"settlement_day31_0001",
		"nonce-1234567890-abcd",
	}
	for _, value := range valid {
		if !validRequestKey(value) {
			t.Fatalf("expected value %q to be valid", value)
		}
	}

	invalid := []string{
		"short",
		"settlement:day31:0001",
		"包含中文的key_0001",
	}
	for _, value := range invalid {
		if validRequestKey(value) {
			t.Fatalf("expected value %q to be invalid", value)
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

func TestNextAssetBalance(t *testing.T) {
	balance, err := nextAssetBalance(100, 25)
	if err != nil {
		t.Fatalf("next balance failed: %v", err)
	}
	if balance != 125 {
		t.Fatalf("expected 125, got %d", balance)
	}

	if _, err := nextAssetBalance(-1, 100); !errors.Is(err, ErrInvalidAssetBalance) {
		t.Fatalf("negative current balance should fail, got %v", err)
	}
	if _, err := nextAssetBalance(100, 0); !errors.Is(err, ErrInvalidAssetBalance) {
		t.Fatalf("zero delta should fail, got %v", err)
	}
	if _, err := nextAssetBalance(math.MaxInt64, 1); !errors.Is(err, ErrInvalidAssetBalance) {
		t.Fatalf("overflow should fail, got %v", err)
	}
}

func TestCreateRejectsMissingIdempotencyKey(t *testing.T) {
	service := NewService(nil, mission.NewManager())

	_, _, err := service.Create(
		context.Background(),
		1,
		"squad_1",
		"mission_instance_1234567890",
		"",
		"nonce_day31_request_0001",
	)
	if !errors.Is(err, ErrIdempotencyKeyRequired) {
		t.Fatalf("expected ErrIdempotencyKeyRequired, got %v", err)
	}
}

func TestCreateRejectsMissionSquadChanged(t *testing.T) {
	missionManager, missionState := finishedMission(t)
	service := NewService(nil, missionManager)

	_, _, err := service.Create(
		context.Background(),
		1,
		"squad_2",
		missionState.ID,
		"settlement_day31_0001",
		"nonce_day31_request_0001",
	)
	if !errors.Is(err, ErrMissionSquadChanged) {
		t.Fatalf("expected ErrMissionSquadChanged, got %v", err)
	}
}

func TestCreateRejectsPlayerOutsideMission(t *testing.T) {
	missionManager, missionState := finishedMission(t)
	service := NewService(nil, missionManager)

	_, _, err := service.Create(
		context.Background(),
		9,
		missionState.SquadID,
		missionState.ID,
		"settlement_day31_0002",
		"nonce_day31_request_0002",
	)
	if !errors.Is(err, ErrPlayerNotInMission) {
		t.Fatalf("expected ErrPlayerNotInMission, got %v", err)
	}
}

func TestCreateRejectsUnfinishedMission(t *testing.T) {
	manager := mission.NewManager()
	created, err := manager.Create("training_ground", "squad_1", []int64{1, 2})
	if err != nil {
		t.Fatalf("create mission failed: %v", err)
	}
	service := NewService(nil, manager)

	_, _, err = service.Create(
		context.Background(),
		1,
		created.SquadID,
		created.ID,
		"settlement_day31_0003",
		"nonce_day31_request_0003",
	)
	if !errors.Is(err, ErrMissionNotFinished) {
		t.Fatalf("expected ErrMissionNotFinished, got %v", err)
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
