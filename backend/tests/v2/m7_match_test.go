package v2_test

import (
	"errors"
	"fmt"
	"testing"

	"game-realtime-gm/backend/internal/pve/party"
	"game-realtime-gm/backend/internal/pve/store"
)

func TestM7CancellationAndConfirmationDeadlineRaces(t *testing.T) {
	for iteration := 0; iteration < 4; iteration++ {
		t.Run(fmt.Sprintf("cancel_%d", iteration), func(t *testing.T) {
			fixture := setup(t)
			for _, player := range fixture.players {
				fixture.command(t, player, "v2.match.enqueue", map[string]any{"plan": party.Plan{Operation: "training_ground", Difficulty: "normal", FillPolicy: "public"}})
			}
			gate := make(chan struct{})
			cancellation, matching := make(chan error, 1), make(chan error, 1)
			go func() {
				<-gate
				_, err := fixture.app.Command(fixture.ctx, fixture.players[0], store.ID("cancel"), "v2.match.cancel", store.JSON(map[string]any{}))
				cancellation <- err
			}()
			go func() { <-gate; matching <- fixture.app.Match.Match(fixture.ctx) }()
			close(gate)
			cancelErr, matchErr := <-cancellation, <-matching
			if matchErr != nil || (cancelErr != nil && !errors.Is(cancelErr, store.Conflict)) {
				t.Fatalf("cancel/match race errors=%v/%v", cancelErr, matchErr)
			}
			var reservations, locks int
			if err := fixture.db.QueryRow("SELECT COUNT(*) FROM pve_match_proposal_members m JOIN pve_match_proposals p ON p.id=m.proposal_id WHERE m.player_id=? AND p.status='pending'", fixture.players[0]).Scan(&reservations); err != nil {
				t.Fatal(err)
			}
			if err := fixture.db.QueryRow("SELECT COUNT(*) FROM pve_player_activity_locks WHERE player_id=?", fixture.players[0]).Scan(&locks); err != nil {
				t.Fatal(err)
			}
			if cancelErr == nil && (reservations != 0 || locks != 0) {
				t.Fatal("successful cancellation retained a reservation or activity lock")
			}
			if cancelErr != nil && (reservations != 1 || locks != 1) {
				t.Fatal("match winner lost or duplicated its reservation")
			}
		})
	}
	t.Run("final_confirmation_vs_deadline", func(t *testing.T) {
		fixture := setup(t)
		for _, player := range fixture.players {
			fixture.command(t, player, "v2.match.enqueue", map[string]any{"plan": party.Plan{Operation: "training_ground", Difficulty: "normal", FillPolicy: "public"}})
		}
		if err := fixture.app.Match.Match(fixture.ctx); err != nil {
			t.Fatal(err)
		}
		var proposal string
		if err := fixture.db.QueryRow("SELECT id FROM pve_match_proposals WHERE status='pending'").Scan(&proposal); err != nil {
			t.Fatal(err)
		}
		input := map[string]any{"proposal_id": proposal, "revision": 1}
		for _, player := range fixture.players[:3] {
			fixture.command(t, player, "v2.match.proposal_confirm", input)
		}
		gate := make(chan struct{})
		confirmation, expiration := make(chan error, 1), make(chan error, 1)
		go func() {
			<-gate
			_, err := fixture.app.Command(fixture.ctx, fixture.players[3], store.ID("confirm"), "v2.match.proposal_confirm", store.JSON(input))
			confirmation <- err
		}()
		go func() {
			<-gate
			_, err := fixture.db.Exec("UPDATE pve_match_proposals SET deadline_at=UTC_TIMESTAMP(3)-INTERVAL 1 SECOND WHERE id=?", proposal)
			if err == nil {
				err = fixture.app.Match.Expire(fixture.ctx)
			}
			expiration <- err
		}()
		close(gate)
		confirmErr, expireErr := <-confirmation, <-expiration
		if expireErr != nil || (confirmErr != nil && !errors.Is(confirmErr, store.Conflict)) {
			t.Fatalf("confirm/deadline race errors=%v/%v", confirmErr, expireErr)
		}
		var status string
		var runs int
		if err := fixture.db.QueryRow("SELECT status FROM pve_match_proposals WHERE id=?", proposal).Scan(&status); err != nil {
			t.Fatal(err)
		}
		if err := fixture.db.QueryRow("SELECT COUNT(*) FROM pve_runs").Scan(&runs); err != nil {
			t.Fatal(err)
		}
		if confirmErr == nil && (status != "assigned" || runs != 1) {
			t.Fatal("successful final confirmation did not produce exactly one run")
		}
		if confirmErr != nil && (status != "rejected" || runs != 0) {
			t.Fatal("expired confirmation retained a run or live proposal")
		}
	})
}
