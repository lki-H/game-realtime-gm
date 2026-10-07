package party

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"game-realtime-gm/backend/internal/pve/store"
)

func (s *Service) Recruit(ctx context.Context, transaction *sql.Tx, player int64, kind string, input Request) (any, error) {
	if kind == "selection" || kind == "ready" || kind == "leave" {
		group, err := Load(ctx, transaction, input.PartyID)
		if err != nil {
			return nil, err
		}
		if group.Status != "open" {
			return nil, store.Conflict
		}
		var version int64
		var expires time.Time
		if err := transaction.QueryRowContext(ctx, "SELECT selection_version,expires_at FROM pve_recruitment_preferences WHERE party_id=? AND player_id=? FOR UPDATE", input.PartyID, player).Scan(&version, &expires); err != nil {
			return nil, store.Forbidden
		}
		if kind != "leave" && !time.Now().Before(expires) {
			return nil, store.Conflict
		}
		if err := store.RequireIdle(ctx, transaction, player); err != nil {
			return nil, err
		}
		switch kind {
		case "selection":
			if _, err := transaction.ExecContext(ctx, "UPDATE pve_task_pauses SET paused=0 WHERE player_id=? AND task_key=? AND task_version=?", player, input.TaskKey, input.TaskVersion); err != nil {
				return nil, err
			}
			_, err = transaction.ExecContext(ctx, "UPDATE pve_recruitment_preferences SET task_selection=?,selection_version=selection_version+1,ready=0 WHERE party_id=? AND player_id=?", store.JSON(map[string]any{"task_key": input.TaskKey, "task_version": input.TaskVersion}), input.PartyID, player)
		case "ready":
			if input.SelectionVersion != version || input.RosterVersion != group.RosterVersion || input.PlanVersion != group.PlanVersion {
				return nil, store.Conflict
			}
			_, err = transaction.ExecContext(ctx, "UPDATE pve_recruitment_preferences SET ready=?,roster_version=?,plan_version=? WHERE party_id=? AND player_id=?", input.Ready, group.RosterVersion, group.PlanVersion, input.PartyID, player)
		case "leave":
			if _, err = transaction.ExecContext(ctx, "UPDATE pve_recruitment_applications SET status='withdrawn' WHERE id=(SELECT application_id FROM pve_recruitment_roster WHERE party_id=? AND player_id=?)", input.PartyID, player); err != nil {
				return nil, err
			}
			if _, err = transaction.ExecContext(ctx, "DELETE FROM pve_recruitment_roster WHERE party_id=? AND player_id=?", input.PartyID, player); err != nil {
				return nil, err
			}
			_, err = transaction.ExecContext(ctx, "DELETE FROM pve_recruitment_preferences WHERE party_id=? AND player_id=?", input.PartyID, player)
			if err == nil {
				err = rosterChanged(ctx, transaction, group.ID)
			}
		}
		if kind == "selection" {
			version++
		}
		return map[string]any{"updated": true, "selection_version": version}, err
	}
	if kind == "withdraw" {
		result, err := transaction.ExecContext(ctx, "UPDATE pve_recruitment_applications SET status='withdrawn' WHERE id=? AND applicant_id=? AND status='pending'", input.ApplicationID, player)
		if err != nil {
			return nil, err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return nil, err
		}
		if count != 1 {
			return nil, store.Conflict
		}
		return map[string]any{"withdrawn": true}, nil
	}
	if kind == "publish" {
		group, err := Load(ctx, transaction, input.PartyID)
		if err != nil {
			return nil, err
		}
		if group.OwnerID != player || group.Status != "open" {
			return nil, store.Forbidden
		}
		if len(group.Members) >= 4 {
			return nil, store.Conflict
		}
		var posts int
		if err := transaction.QueryRowContext(ctx, "SELECT COUNT(*) FROM pve_recruitment_posts WHERE owner_id=? AND created_at>DATE_SUB(UTC_TIMESTAMP(3),INTERVAL 1 MINUTE)", player).Scan(&posts); err != nil {
			return nil, err
		}
		if posts >= 5 {
			return nil, store.Conflict
		}
		if _, err := transaction.ExecContext(ctx, "UPDATE pve_recruitment_posts SET status='closed' WHERE party_id=? AND status='open'", group.ID); err != nil {
			return nil, err
		}
		id := store.ID("post")
		_, err = transaction.ExecContext(ctx, "INSERT INTO pve_recruitment_posts(id,party_id,owner_id,operation_id,difficulty,slots,tags,expires_at) VALUES(?,?,?,?,?,?,?,?)", id, group.ID, player, group.Plan.Operation, group.Plan.Difficulty, 4-len(group.Members), store.JSON(group.Plan.Tags), time.Now().UTC().Add(15*time.Minute))
		return map[string]any{"post_id": id}, err
	}
	if kind == "apply" {
		var owner int64
		var status string
		var expires time.Time
		if err := transaction.QueryRowContext(ctx, "SELECT owner_id,status,expires_at FROM pve_recruitment_posts WHERE id=? FOR UPDATE", input.PostID).Scan(&owner, &status, &expires); err != nil {
			return nil, store.NotFound
		}
		if status != "open" || time.Now().UTC().After(expires) || owner == player {
			return nil, store.Conflict
		}
		blocked, err := store.Blocked(ctx, transaction, owner, player)
		if err != nil {
			return nil, err
		}
		if blocked {
			return nil, store.Forbidden
		}
		if err := store.RequireIdle(ctx, transaction, player); err != nil {
			return nil, err
		}
		var partyCount int
		if err := transaction.QueryRowContext(ctx, "SELECT COUNT(*) FROM pve_party_members WHERE player_id=? AND status='active'", player).Scan(&partyCount); err != nil {
			return nil, err
		}
		if partyCount > 0 {
			return nil, store.Conflict
		}
		var pending int
		if err := transaction.QueryRowContext(ctx, "SELECT COUNT(*) FROM pve_recruitment_applications WHERE applicant_id=? AND status='pending' AND created_at>DATE_SUB(UTC_TIMESTAMP(3),INTERVAL 15 MINUTE)", player).Scan(&pending); err != nil {
			return nil, err
		}
		if pending >= 10 {
			return nil, store.Conflict
		}
		result, err := transaction.ExecContext(ctx, "INSERT INTO pve_recruitment_applications(post_id,applicant_id) VALUES(?,?)", input.PostID, player)
		if err != nil {
			return nil, err
		}
		id, err := result.LastInsertId()
		if err != nil {
			return nil, err
		}
		if err := store.Notify(ctx, transaction, []int64{owner}, "v2.recruitment.applied", map[string]any{"post_id": input.PostID, "application_id": id, "player_id": player}); err != nil {
			return nil, err
		}
		return map[string]any{"application_id": id}, err
	}
	if kind == "respond" {
		var partyID string
		var owner, applicant int64
		var status string
		var postStatus string
		var expires time.Time
		err := transaction.QueryRowContext(ctx, "SELECT p.party_id,p.owner_id,a.applicant_id,a.status,p.expires_at,p.status FROM pve_recruitment_applications a JOIN pve_recruitment_posts p ON p.id=a.post_id WHERE a.id=? FOR UPDATE", input.ApplicationID).Scan(&partyID, &owner, &applicant, &status, &expires, &postStatus)
		if err != nil {
			return nil, store.NotFound
		}
		if owner != player || status != "pending" || postStatus != "open" || time.Now().UTC().After(expires) {
			return nil, store.Forbidden
		}
		group, err := Load(ctx, transaction, partyID)
		if err != nil {
			return nil, err
		}
		if group.Status != "open" {
			return nil, store.Conflict
		}
		if group.OwnerID != player {
			return nil, store.Forbidden
		}
		next := "rejected"
		if input.Accept {
			if err := store.LockPlayers(ctx, transaction, []int64{applicant}); err != nil {
				return nil, err
			}
			if err := store.RequireNoRecruitment(ctx, transaction, applicant); err != nil {
				return nil, err
			}
			var partyCount int
			if err := transaction.QueryRowContext(ctx, "SELECT COUNT(*) FROM pve_party_members WHERE player_id=? AND status='active'", applicant).Scan(&partyCount); err != nil {
				return nil, err
			}
			if partyCount > 0 {
				return nil, store.Conflict
			}
			blocked, err := store.Blocked(ctx, transaction, player, applicant)
			if err != nil {
				return nil, err
			}
			if blocked {
				return nil, store.Forbidden
			}
			if err := store.RequireIdle(ctx, transaction, applicant); err != nil {
				return nil, err
			}
			var guests int
			if err := transaction.QueryRowContext(ctx, "SELECT COUNT(*) FROM pve_recruitment_roster WHERE party_id=?", partyID).Scan(&guests); err != nil {
				return nil, err
			}
			if guests+len(group.Members) >= 4 {
				return nil, store.Conflict
			}
			if _, err := transaction.ExecContext(ctx, "INSERT INTO pve_recruitment_roster(party_id,player_id,application_id) VALUES(?,?,?)", partyID, applicant, input.ApplicationID); err != nil {
				return nil, err
			}
			if err := rosterChanged(ctx, transaction, partyID); err != nil {
				return nil, err
			}
			if err := transaction.QueryRowContext(ctx, "SELECT roster_version FROM pve_parties WHERE id=?", partyID).Scan(&group.RosterVersion); err != nil {
				return nil, err
			}
			if _, err := transaction.ExecContext(ctx, "INSERT INTO pve_recruitment_preferences(party_id,player_id,task_selection,roster_version,plan_version,expires_at) VALUES(?,?,'{}',?,?,?)", partyID, applicant, group.RosterVersion, group.PlanVersion, expires); err != nil {
				return nil, err
			}
			if err := store.Notify(ctx, transaction, []int64{applicant}, "v2.recruitment.accepted", map[string]any{"party_id": partyID, "application_id": input.ApplicationID, "roster_version": group.RosterVersion, "plan_version": group.PlanVersion, "selection_version": 1, "expires_at": expires}); err != nil {
				return nil, err
			}
			next = "accepted"
		}
		_, err = transaction.ExecContext(ctx, "UPDATE pve_recruitment_applications SET status=? WHERE id=?", next, input.ApplicationID)
		if err == nil {
			err = store.Notify(ctx, transaction, []int64{applicant}, "v2.recruitment.responded", map[string]any{"application_id": input.ApplicationID, "party_id": partyID, "status": next})
		}
		return map[string]any{"status": next}, err
	}
	return nil, store.NotFound
}

func rosterChanged(ctx context.Context, transaction *sql.Tx, id string) error {
	if _, err := transaction.ExecContext(ctx, "UPDATE pve_parties SET roster_version=roster_version+1 WHERE id=?", id); err != nil {
		return err
	}
	if _, err := transaction.ExecContext(ctx, "UPDATE pve_party_members SET ready=0 WHERE party_id=? AND status='active'", id); err != nil {
		return err
	}
	_, err := transaction.ExecContext(ctx, "UPDATE pve_recruitment_preferences SET ready=0 WHERE party_id=?", id)
	return err
}

func (s *Service) ExpireRecruitment(ctx context.Context) error {
	rows, err := s.DB.QueryContext(ctx, "SELECT DISTINCT party_id FROM pve_recruitment_preferences WHERE expires_at<=UTC_TIMESTAMP(3) ORDER BY party_id LIMIT 50")
	if err != nil {
		return err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := store.Transaction(ctx, s.DB, func(transaction *sql.Tx) error {
			if _, err := Load(ctx, transaction, id); err != nil {
				return err
			}
			if _, err := transaction.ExecContext(ctx, "UPDATE pve_recruitment_applications SET status='expired' WHERE id IN (SELECT r.application_id FROM pve_recruitment_roster r JOIN pve_recruitment_preferences p ON p.party_id=r.party_id AND p.player_id=r.player_id WHERE p.party_id=? AND p.expires_at<=UTC_TIMESTAMP(3))", id); err != nil {
				return err
			}
			if _, err := transaction.ExecContext(ctx, "DELETE r FROM pve_recruitment_roster r JOIN pve_recruitment_preferences p ON p.party_id=r.party_id AND p.player_id=r.player_id WHERE p.party_id=? AND p.expires_at<=UTC_TIMESTAMP(3)", id); err != nil {
				return err
			}
			result, err := transaction.ExecContext(ctx, "DELETE FROM pve_recruitment_preferences WHERE party_id=? AND expires_at<=UTC_TIMESTAMP(3)", id)
			if err != nil {
				return err
			}
			count, err := result.RowsAffected()
			if err != nil {
				return err
			}
			if count == 0 {
				return nil
			}
			return rosterChanged(ctx, transaction, id)
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) Applications(ctx context.Context, player int64) ([]map[string]any, error) {
	rows, err := s.DB.QueryContext(ctx, "SELECT a.id,a.post_id,a.applicant_id,a.status,p.party_id,p.expires_at FROM pve_recruitment_applications a JOIN pve_recruitment_posts p ON p.id=a.post_id JOIN pve_parties g ON g.id=p.party_id WHERE a.applicant_id=? OR (g.owner_id=? AND p.owner_id=g.owner_id) ORDER BY a.created_at DESC,a.id DESC LIMIT 50", player, player)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []map[string]any{}
	for rows.Next() {
		var id, applicant int64
		var post, status, room string
		var expires time.Time
		if err := rows.Scan(&id, &post, &applicant, &status, &room, &expires); err != nil {
			return nil, err
		}
		if status == "pending" && !time.Now().Before(expires) {
			status = "expired"
		}
		result = append(result, map[string]any{"id": id, "post_id": post, "applicant_id": applicant, "status": status, "party_id": room, "expires_at": expires})
	}
	return result, rows.Err()
}
func (s *Service) RecruitmentSnapshot(ctx context.Context, partyID string, player int64) (map[string]any, error) {
	var value map[string]any
	err := store.Transaction(ctx, s.DB, func(transaction *sql.Tx) error {
		group, err := Load(ctx, transaction, partyID)
		if err != nil {
			return err
		}
		var count int
		if err := transaction.QueryRowContext(ctx, "SELECT COUNT(*) FROM pve_recruitment_preferences WHERE party_id=? AND player_id=?", partyID, player).Scan(&count); err != nil {
			return err
		}
		if !Contains(group, player) && count == 0 {
			return store.Forbidden
		}
		rows, err := transaction.QueryContext(ctx, "SELECT player_id,ready,selection_version,task_selection,expires_at FROM pve_recruitment_preferences WHERE party_id=? ORDER BY player_id", partyID)
		if err != nil {
			return err
		}
		guests := []map[string]any{}
		for rows.Next() {
			var guest int64
			var ready bool
			var version int64
			var content json.RawMessage
			var expires time.Time
			if err := rows.Scan(&guest, &ready, &version, &content, &expires); err != nil {
				rows.Close()
				return err
			}
			item := map[string]any{"player_id": guest, "ready": ready, "selection_version": version, "expires_at": expires}
			if guest == player {
				item["task_selection"] = content
			}
			guests = append(guests, item)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		value = map[string]any{"party": group.ForPlayer(player), "guests": guests}
		return nil
	})
	return value, err
}
