package v2_test

import (
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"game-realtime-gm/backend/internal/pve/party"
	"game-realtime-gm/backend/internal/pve/store"
)

func TestPostM7FriendResponseCannotRecreateBlockedFriendship(t *testing.T) {
	fixture := setup(t)
	requester, recipient := fixture.players[0], fixture.players[1]
	var request struct{ ID int64 }
	if err := json.Unmarshal(fixture.command(t, requester, "v2.social.friend_request", map[string]any{"player_id": recipient}), &request); err != nil {
		t.Fatal(err)
	}
	block, err := fixture.db.BeginTx(fixture.ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		t.Fatal(err)
	}
	defer block.Rollback()
	if err := store.LockPlayers(fixture.ctx, block, []int64{requester}); err != nil {
		t.Fatal(err)
	}
	if _, err := block.ExecContext(fixture.ctx, "INSERT INTO pve_social_blocks(blocker_id,blocked_id) VALUES(?,?)", requester, recipient); err != nil {
		t.Fatal(err)
	}
	if _, err := block.ExecContext(fixture.ctx, "UPDATE pve_social_friendships SET status='removed' WHERE player_low_id=? AND player_high_id=?", requester, recipient); err != nil {
		t.Fatal(err)
	}
	response := make(chan error, 1)
	go func() {
		_, err := fixture.app.Command(fixture.ctx, recipient, store.ID("accept"), "v2.social.friend_response", store.JSON(map[string]any{"request_id": request.ID, "accept": true}))
		response <- err
	}()
	time.Sleep(100 * time.Millisecond)
	if _, err := block.ExecContext(fixture.ctx, "UPDATE pve_social_friend_requests SET status='rejected' WHERE id=? AND status='pending'", request.ID); err != nil {
		t.Fatal(err)
	}
	if err := block.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := <-response; err != nil && !errors.Is(err, store.Conflict) && !errors.Is(err, store.Forbidden) {
		t.Fatal(err)
	}
	var active int
	if err := fixture.db.QueryRow("SELECT COUNT(*) FROM pve_social_friendships WHERE player_low_id=? AND player_high_id=? AND status='active'", requester, recipient).Scan(&active); err != nil || active != 0 {
		t.Fatalf("blocked friendship became active: count=%d", active)
	}
}

func TestPostM7FriendRequestsCannotSpamPendingNotifications(t *testing.T) {
	fixture := setup(t)
	requester, recipient := fixture.players[0], fixture.players[1]
	fixture.command(t, requester, "v2.social.friend_request", map[string]any{"player_id": recipient})
	if _, err := fixture.app.Command(fixture.ctx, requester, store.ID("duplicate"), "v2.social.friend_request", store.JSON(map[string]any{"player_id": recipient})); !errors.Is(err, store.Conflict) {
		t.Fatal("new operation repeated an already pending friend request")
	}
	var notifications int
	if err := fixture.db.QueryRow("SELECT COUNT(*) FROM pve_outbox_records WHERE event_type='v2.social.friend_requested'").Scan(&notifications); err != nil || notifications != 1 {
		t.Fatalf("duplicate pending notification count=%d", notifications)
	}
}

func TestPostM7FriendRequestRateCountsRepeatedPairActions(t *testing.T) {
	fixture := setup(t)
	requester, recipient := fixture.players[0], fixture.players[1]
	for index := 0; index < 20; index++ {
		var request struct{ ID int64 }
		if err := json.Unmarshal(fixture.command(t, requester, "v2.social.friend_request", map[string]any{"player_id": recipient}), &request); err != nil {
			t.Fatal(err)
		}
		fixture.command(t, requester, "v2.social.friend_withdraw", map[string]any{"request_id": request.ID})
	}
	if _, err := fixture.app.Command(fixture.ctx, requester, store.ID("limited"), "v2.social.friend_request", store.JSON(map[string]any{"player_id": recipient})); !errors.Is(err, store.Conflict) {
		t.Fatal("repeated request/withdraw bypassed the per-minute budget")
	}
}

func TestPostM7PartyInviteRejectsInvalidTargets(t *testing.T) {
	fixture := setup(t)
	owner, banned := fixture.players[0], fixture.players[1]
	var group party.Party
	if err := json.Unmarshal(fixture.command(t, owner, "v2.party.create", map[string]any{}), &group); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.db.Exec("UPDATE players SET status='banned' WHERE id=?", banned); err != nil {
		t.Fatal(err)
	}
	for _, target := range []int64{owner, banned, 9223372036854775807} {
		if _, err := fixture.app.Command(fixture.ctx, owner, store.ID("invite"), "v2.party.invite", store.JSON(map[string]any{"party_id": group.ID, "player_id": target})); err == nil {
			t.Errorf("invalid invite target=%d was accepted", target)
		}
	}
	var invitations int
	if err := fixture.db.QueryRow("SELECT COUNT(*) FROM pve_party_invites WHERE party_id=?", group.ID).Scan(&invitations); err != nil || invitations != 0 {
		t.Fatalf("invalid invitations persisted: %d", invitations)
	}
}
