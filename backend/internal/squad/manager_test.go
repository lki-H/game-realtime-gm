package squad

import (
	"errors"
	"testing"
)

func TestHandleDisconnectClearsReady(t *testing.T) {
	manager := NewManager()
	created, err := manager.Create(1, "player01")
	if err != nil {
		t.Fatalf("create squad failed: %v", err)
	}
	if _, err := manager.Join(created.ID, 2, "player02"); err != nil {
		t.Fatalf("join squad failed: %v", err)
	}
	if _, err := manager.SetReady(2, true); err != nil {
		t.Fatalf("set ready failed: %v", err)
	}

	state, changed, leaderChanged, err := manager.HandleDisconnect(2)
	if err != nil {
		t.Fatalf("disconnect failed: %v", err)
	}
	if !changed || leaderChanged {
		t.Fatalf("unexpected flags: changed=%v leader_changed=%v", changed, leaderChanged)
	}

	member := findMember(t, state, 2)
	if member.Online {
		t.Fatal("disconnected member should be offline")
	}
	if member.Ready {
		t.Fatal("disconnected member should not remain ready")
	}
}

func TestHandleDisconnectTransfersLeader(t *testing.T) {
	manager := NewManager()
	created, err := manager.Create(1, "player01")
	if err != nil {
		t.Fatalf("create squad failed: %v", err)
	}
	if _, err := manager.Join(created.ID, 2, "player02"); err != nil {
		t.Fatalf("join squad failed: %v", err)
	}
	if _, err := manager.Join(created.ID, 3, "player03"); err != nil {
		t.Fatalf("join squad failed: %v", err)
	}

	state, changed, leaderChanged, err := manager.HandleDisconnect(1)
	if err != nil {
		t.Fatalf("disconnect leader failed: %v", err)
	}
	if !changed || !leaderChanged {
		t.Fatalf("unexpected flags: changed=%v leader_changed=%v", changed, leaderChanged)
	}
	if state.LeaderID != 2 {
		t.Fatalf("expected player 2 to become leader, got %d", state.LeaderID)
	}
}

func TestReconnectDoesNotReclaimLeader(t *testing.T) {
	manager := NewManager()
	created, err := manager.Create(1, "player01")
	if err != nil {
		t.Fatalf("create squad failed: %v", err)
	}
	if _, err := manager.Join(created.ID, 2, "player02"); err != nil {
		t.Fatalf("join squad failed: %v", err)
	}
	if _, _, _, err := manager.HandleDisconnect(1); err != nil {
		t.Fatalf("disconnect leader failed: %v", err)
	}

	state, changed, err := manager.HandleReconnect(1)
	if err != nil {
		t.Fatalf("reconnect failed: %v", err)
	}
	if !changed {
		t.Fatal("reconnect should change online state")
	}
	if state.LeaderID != 2 {
		t.Fatalf("reconnected player should not reclaim leader, got %d", state.LeaderID)
	}

	member := findMember(t, state, 1)
	if !member.Online {
		t.Fatal("reconnected member should be online")
	}
	if member.Ready {
		t.Fatal("reconnected member should remain not ready")
	}
}

func TestStats(t *testing.T) {
	manager := NewManager()
	first, err := manager.Create(1, "player01")
	if err != nil {
		t.Fatalf("create first squad failed: %v", err)
	}
	if _, err := manager.Join(first.ID, 2, "player02"); err != nil {
		t.Fatalf("join first squad failed: %v", err)
	}
	if _, err := manager.Create(3, "player03"); err != nil {
		t.Fatalf("create second squad failed: %v", err)
	}
	if _, _, _, err := manager.HandleDisconnect(2); err != nil {
		t.Fatalf("disconnect member failed: %v", err)
	}

	stats := manager.Stats()
	if stats.Squads != 2 || stats.Members != 3 || stats.OnlineMembers != 2 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
}

func TestLeaveRemovesMembersAndDisbandsSquad(t *testing.T) {
	manager := NewManager()
	created, err := manager.Create(1, "player01")
	if err != nil {
		t.Fatalf("create squad failed: %v", err)
	}
	if _, err := manager.Join(created.ID, 2, "player02"); err != nil {
		t.Fatalf("join squad failed: %v", err)
	}

	state, disbanded, err := manager.Leave(2)
	if err != nil {
		t.Fatalf("member leave failed: %v", err)
	}
	if disbanded {
		t.Fatal("squad should remain while leader is present")
	}
	if len(state.Members) != 1 || state.Members[0].PlayerID != 1 {
		t.Fatalf("unexpected members after leave: %+v", state.Members)
	}
	if _, err := manager.GetByPlayer(2); !errors.Is(err, ErrPlayerNotInSquad) {
		t.Fatalf("left player should not remain in squad index, got %v", err)
	}

	state, disbanded, err = manager.Leave(1)
	if err != nil {
		t.Fatalf("leader leave failed: %v", err)
	}
	if !disbanded || state != nil {
		t.Fatalf("last member should disband squad: state=%+v disbanded=%v", state, disbanded)
	}
	if stats := manager.Stats(); stats.Squads != 0 || stats.Members != 0 {
		t.Fatalf("disbanded squad should not remain in stats: %+v", stats)
	}
}

func findMember(t *testing.T, state *Squad, playerID int64) Member {
	t.Helper()
	for _, member := range state.Members {
		if member.PlayerID == playerID {
			return member
		}
	}
	t.Fatalf("player %d not found in squad", playerID)
	return Member{}
}
