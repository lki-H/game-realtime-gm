package leaderboard

import (
	"errors"
	"testing"
	"time"
)

func TestValidMissionID(t *testing.T) {
	valid := []string{
		"day32_training_ground",
		"raid-boss-01",
		"mission.normal",
	}
	for _, value := range valid {
		if !validMissionID(value) {
			t.Fatalf("expected %q to be valid", value)
		}
	}

	invalid := []string{
		"",
		"bad:mission",
		"mission space",
		"包含中文",
	}
	for _, value := range invalid {
		if validMissionID(value) {
			t.Fatalf("expected %q to be invalid", value)
		}
	}
}

func TestLeaderboardMemberRoundTrip(t *testing.T) {
	achievedAt := time.Date(2026, 8, 23, 10, 30, 0, 123000000, time.Local)
	member, err := formatLeaderboardMember(9, 101, achievedAt)
	if err != nil {
		t.Fatalf("format member failed: %v", err)
	}

	metadata, err := parseLeaderboardMember(member)
	if err != nil {
		t.Fatalf("parse member failed: %v", err)
	}
	if metadata.PlayerID != 9 || metadata.MissionRecordID != 101 {
		t.Fatalf("unexpected metadata: %+v", metadata)
	}
	if metadata.AchievedAt.UnixMilli() != achievedAt.UnixMilli() {
		t.Fatalf("unexpected achieved_at: %s", metadata.AchievedAt)
	}
}

func TestEarlierAchievementSortsFirstForEqualScore(t *testing.T) {
	firstTime := time.Date(2026, 8, 23, 10, 0, 0, 0, time.Local)
	laterTime := firstTime.Add(time.Minute)

	firstMember, err := formatLeaderboardMember(9, 101, firstTime)
	if err != nil {
		t.Fatalf("format first member failed: %v", err)
	}
	laterMember, err := formatLeaderboardMember(10, 102, laterTime)
	if err != nil {
		t.Fatalf("format later member failed: %v", err)
	}

	if firstMember <= laterMember {
		t.Fatalf(
			"ZREVRANGE should place earlier equal-score member first: first=%s later=%s",
			firstMember,
			laterMember,
		)
	}
}

func TestInvalidLeaderboardMember(t *testing.T) {
	if _, err := parseLeaderboardMember("broken"); !errors.Is(err, ErrInvalidLeaderboardEntry) {
		t.Fatalf("expected ErrInvalidLeaderboardEntry, got %v", err)
	}
	if _, err := formatLeaderboardMember(0, 1, time.Now()); !errors.Is(err, ErrInvalidLeaderboardEntry) {
		t.Fatalf("expected invalid player id error, got %v", err)
	}
}

func TestLeaderboardKeysShareHashTag(t *testing.T) {
	missionID := "day32_training_ground"
	if scoresKey(missionID) != "leaderboard:{day32_training_ground}:scores" {
		t.Fatalf("unexpected scores key: %s", scoresKey(missionID))
	}
	if playersKey(missionID) != "leaderboard:{day32_training_ground}:players" {
		t.Fatalf("unexpected players key: %s", playersKey(missionID))
	}
}
