package matchmaking

import (
	"context"
	"database/sql"

	"game-realtime-gm/backend/internal/pve/store"
)

func (s *Service) DrainTx(ctx context.Context, transaction *sql.Tx) error {
	rows, err := transaction.QueryContext(ctx, "SELECT id FROM pve_match_tickets WHERE status IN ('queued','proposed') ORDER BY id")
	if err != nil {
		return err
	}
	var tickets []string
	for rows.Next() {
		var ticket string
		if err := rows.Scan(&ticket); err != nil {
			rows.Close()
			return err
		}
		tickets = append(tickets, ticket)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, ticket := range tickets {
		members, err := transaction.QueryContext(ctx, "SELECT player_id FROM pve_match_ticket_members WHERE ticket_id=?", ticket)
		if err != nil {
			return err
		}
		var players []int64
		for members.Next() {
			var player int64
			if err := members.Scan(&player); err != nil {
				members.Close()
				return err
			}
			players = append(players, player)
		}
		err = members.Err()
		members.Close()
		if err != nil {
			return err
		}
		if err := s.releaseTicket(ctx, transaction, ticket, false, ""); err != nil {
			return err
		}
		if _, err := transaction.ExecContext(ctx, "UPDATE pve_recruitment_preferences SET ready=0 WHERE player_id IN (SELECT player_id FROM pve_match_ticket_members WHERE ticket_id=?)", ticket); err != nil {
			return err
		}
		if err := store.Notify(ctx, transaction, players, "v2.match.paused", map[string]any{"ticket_id": ticket, "reason": "maintenance"}); err != nil {
			return err
		}
	}
	_, err = transaction.ExecContext(ctx, "UPDATE pve_match_proposals SET status='aborted' WHERE status='pending'")
	return err
}
