package party

import (
	"context"
	"database/sql"
	"encoding/json"
	"sort"
	"time"

	"game-realtime-gm/backend/internal/pve/run"
	"game-realtime-gm/backend/internal/pve/store"
	"game-realtime-gm/backend/internal/pve/task"
)

type regroupSource struct {
	PlayerID         int64           `json:"player_id"`
	PartyID          string          `json:"party_id"`
	OwnerID          int64           `json:"owner_id"`
	MembershipID     string          `json:"membership_id"`
	RosterVersion    int64           `json:"roster_version"`
	PlanVersion      int64           `json:"plan_version"`
	SelectionVersion int64           `json:"selection_version"`
	TaskSelection    json.RawMessage `json:"task_selection"`
}

func regroupSources(ctx context.Context, transaction *sql.Tx, players []int64) ([]regroupSource, error) {
	result := []regroupSource{}
	for _, player := range players {
		if err := store.RequireIdle(ctx, transaction, player); err != nil {
			return nil, err
		}
		var recruitment int
		if err := transaction.QueryRowContext(ctx, "SELECT COUNT(*) FROM pve_recruitment_preferences WHERE player_id=? AND expires_at>UTC_TIMESTAMP(3)", player).Scan(&recruitment); err != nil {
			return nil, err
		}
		if recruitment > 0 {
			return nil, store.Conflict
		}
		var room string
		err := transaction.QueryRowContext(ctx, "SELECT party_id FROM pve_party_members WHERE player_id=? AND status='active'", player).Scan(&room)
		if err == sql.ErrNoRows {
			result = append(result, regroupSource{PlayerID: player})
			continue
		}
		if err != nil {
			return nil, err
		}
		group, err := Load(ctx, transaction, room)
		if err != nil {
			return nil, err
		}
		if group.Status != "open" {
			return nil, store.Conflict
		}
		for _, member := range group.Members {
			if member.PlayerID == player {
				result = append(result, regroupSource{PlayerID: player, PartyID: room, OwnerID: group.OwnerID, MembershipID: member.MembershipID, RosterVersion: group.RosterVersion, PlanVersion: group.PlanVersion, SelectionVersion: member.SelectionVersion, TaskSelection: member.TaskSelection})
			}
		}
	}
	return result, nil
}
func (service *Service) Regroup(ctx context.Context, transaction *sql.Tx, player int64, kind string, input Request) (any, error) {
	if kind == "propose" {
		players := append([]int64(nil), input.PlayerIDs...)
		sort.Slice(players, func(first, second int) bool { return players[first] < players[second] })
		if len(players) < 1 || len(players) > 4 {
			return nil, store.Invalid
		}
		self, owner := false, false
		for index, target := range players {
			if target <= 0 || (index > 0 && players[index-1] == target) {
				return nil, store.Invalid
			}
			self = self || target == player
			owner = owner || target == input.OwnerID
		}
		if !self || !owner {
			return nil, store.Invalid
		}
		if err := store.LockPlayers(ctx, transaction, players); err != nil {
			return nil, err
		}
		state, err := run.Load(ctx, transaction, input.RunID)
		if err != nil {
			return nil, err
		}
		if state.Status != "closed" || !state.Settled {
			return nil, store.Conflict
		}
		for _, target := range players {
			if state.Member(&target) == nil {
				return nil, store.Forbidden
			}
			for _, other := range players {
				if target >= other {
					continue
				}
				blocked, err := store.Blocked(ctx, transaction, target, other)
				if err != nil {
					return nil, err
				}
				if blocked {
					return nil, store.Forbidden
				}
			}
		}
		plan := input.Plan
		if plan.Operation == "" {
			plan = Plan{Operation: state.OperationName, Difficulty: state.Difficulty, RuleVersion: state.Rules.Version, FillPolicy: "no_fill", AllowPartial: true}
		}
		if plan.Operation != state.OperationName || plan.Difficulty != state.Difficulty || (plan.FillPolicy != "public" && plan.FillPolicy != "no_fill") || len(plan.Tags) > 3 {
			return nil, store.Invalid
		}
		if plan.RuleVersion == "" {
			plan.RuleVersion = state.Rules.Version
		}
		for _, tag := range plan.Tags {
			if tag != "newcomer" && tag != "tasks" && tag != "experienced" {
				return nil, store.Invalid
			}
		}
		if _, err := task.RuleTx(ctx, transaction, plan.Operation, plan.RuleVersion); err != nil {
			return nil, err
		}
		sources, err := regroupSources(ctx, transaction, players)
		if err != nil {
			return nil, err
		}
		var pending int
		if err := transaction.QueryRowContext(ctx, "SELECT COUNT(*) FROM pve_regroup_proposals WHERE proposer_id=? AND status='pending' AND expires_at>UTC_TIMESTAMP(3)", player).Scan(&pending); err != nil {
			return nil, err
		}
		if pending >= 3 {
			return nil, store.Conflict
		}
		id := store.ID("regroup")
		expires := time.Now().UTC().Add(service.RegroupTTL)
		if _, err := transaction.ExecContext(ctx, "INSERT INTO pve_regroup_proposals(id,source_run_id,proposer_id,owner_id,plan,sources,expires_at) VALUES(?,?,?,?,?,?,?)", id, input.RunID, player, input.OwnerID, store.JSON(plan), store.JSON(sources), expires); err != nil {
			return nil, err
		}
		for _, target := range players {
			if _, err := transaction.ExecContext(ctx, "INSERT INTO pve_regroup_members(proposal_id,player_id) VALUES(?,?)", id, target); err != nil {
				return nil, err
			}
		}
		if err := store.Notify(ctx, transaction, players, "v2.party.regroup_proposed", map[string]any{"proposal_id": id, "owner_id": input.OwnerID, "players": players, "plan": plan, "revision": 1, "expires_at": expires}); err != nil {
			return nil, err
		}
		return map[string]any{"proposal_id": id, "revision": 1, "expires_at": expires}, nil
	}
	if kind != "respond" {
		return nil, store.NotFound
	}
	var owner int64
	var revision int64
	var status string
	var expires time.Time
	var planJSON, sourcesJSON []byte
	if err := transaction.QueryRowContext(ctx, "SELECT owner_id,revision,status,expires_at,plan,sources FROM pve_regroup_proposals WHERE id=? FOR UPDATE", input.ProposalID).Scan(&owner, &revision, &status, &expires, &planJSON, &sourcesJSON); err != nil {
		return nil, err
	}
	if status != "pending" || revision != input.Revision || !time.Now().Before(expires) {
		return nil, store.Conflict
	}
	var count int
	if err := transaction.QueryRowContext(ctx, "SELECT COUNT(*) FROM pve_regroup_members WHERE proposal_id=? AND player_id=?", input.ProposalID, player).Scan(&count); err != nil {
		return nil, err
	}
	if count != 1 {
		return nil, store.Forbidden
	}
	if !input.Accept {
		_, err := transaction.ExecContext(ctx, "UPDATE pve_regroup_proposals SET status='rejected' WHERE id=?", input.ProposalID)
		return map[string]any{"status": "rejected"}, err
	}
	if _, err := transaction.ExecContext(ctx, "UPDATE pve_regroup_members SET response='accepted' WHERE proposal_id=? AND player_id=?", input.ProposalID, player); err != nil {
		return nil, err
	}
	if err := transaction.QueryRowContext(ctx, "SELECT COUNT(*) FROM pve_regroup_members WHERE proposal_id=? AND response<>'accepted'", input.ProposalID).Scan(&count); err != nil {
		return nil, err
	}
	if count > 0 {
		return map[string]any{"status": "pending"}, nil
	}
	var plan Plan
	var sources []regroupSource
	if json.Unmarshal(planJSON, &plan) != nil || json.Unmarshal(sourcesJSON, &sources) != nil {
		return nil, store.Invalid
	}
	players := []int64{}
	for _, source := range sources {
		players = append(players, source.PlayerID)
	}
	if err := store.LockPlayers(ctx, transaction, players); err != nil {
		return nil, err
	}
	current, err := regroupSources(ctx, transaction, players)
	if err != nil {
		return nil, err
	}
	if store.Hash(store.JSON(current)) != store.Hash(store.JSON(json.RawMessage(sourcesJSON))) {
		return nil, store.Conflict
	}
	for _, first := range players {
		for _, second := range players {
			if first >= second {
				continue
			}
			blocked, err := store.Blocked(ctx, transaction, first, second)
			if err != nil {
				return nil, err
			}
			if blocked {
				return nil, store.Forbidden
			}
		}
	}
	if _, err := task.RuleTx(ctx, transaction, plan.Operation, plan.RuleVersion); err != nil {
		return nil, err
	}
	room := sources[0].PartyID
	same := room != ""
	for _, source := range sources {
		same = same && source.PartyID == room
	}
	if same {
		group, err := Load(ctx, transaction, room)
		if err != nil {
			return nil, err
		}
		same = len(group.Members) == len(players)
	}
	if same {
		if _, err := transaction.ExecContext(ctx, "UPDATE pve_parties SET owner_id=?,plan=?,plan_version=plan_version+1 WHERE id=?", owner, store.JSON(plan), room); err != nil {
			return nil, err
		}
		if _, err := transaction.ExecContext(ctx, "UPDATE pve_party_members SET ready=0 WHERE party_id=? AND status='active'", room); err != nil {
			return nil, err
		}
	} else {
		room = store.ID("party")
		if _, err := transaction.ExecContext(ctx, "INSERT INTO pve_parties(id,owner_id,plan) VALUES(?,?,?)", room, owner, store.JSON(plan)); err != nil {
			return nil, err
		}
		oldRooms := map[string]bool{}
		for _, source := range sources {
			if source.PartyID != "" {
				if _, err := transaction.ExecContext(ctx, "UPDATE pve_party_members SET status='left',ready=0 WHERE membership_id=? AND status='active'", source.MembershipID); err != nil {
					return nil, err
				}
				oldRooms[source.PartyID] = true
			}
			selection := source.TaskSelection
			if len(selection) == 0 {
				selection = store.JSON(map[string]any{})
			}
			if _, err := transaction.ExecContext(ctx, "INSERT INTO pve_party_members(membership_id,party_id,player_id,selection_version,task_selection) VALUES(?,?,?,?,?)", store.ID("member"), room, source.PlayerID, maxVersion(source.SelectionVersion), selection); err != nil {
				return nil, err
			}
		}
		for previous := range oldRooms {
			group, err := Load(ctx, transaction, previous)
			if err != nil {
				return nil, err
			}
			if len(group.Members) == 0 {
				if _, err := transaction.ExecContext(ctx, "UPDATE pve_parties SET status='closed',roster_version=roster_version+1 WHERE id=?", previous); err != nil {
					return nil, err
				}
			} else {
				newOwner := group.OwnerID
				if !Contains(group, newOwner) {
					newOwner = group.Members[0].PlayerID
				}
				if _, err := transaction.ExecContext(ctx, "UPDATE pve_parties SET owner_id=?,roster_version=roster_version+1 WHERE id=?", newOwner, previous); err != nil {
					return nil, err
				}
				if _, err := transaction.ExecContext(ctx, "UPDATE pve_party_members SET ready=0 WHERE party_id=? AND status='active'", previous); err != nil {
					return nil, err
				}
			}
		}
	}
	if _, err := transaction.ExecContext(ctx, "UPDATE pve_regroup_proposals SET status='committed',result_party_id=? WHERE id=?", room, input.ProposalID); err != nil {
		return nil, err
	}
	if err := store.Notify(ctx, transaction, players, "v2.party.regroup_committed", map[string]any{"proposal_id": input.ProposalID, "party_id": room}); err != nil {
		return nil, err
	}
	return map[string]any{"status": "committed", "party_id": room}, nil
}
func maxVersion(value int64) int64 {
	if value < 1 {
		return 1
	}
	return value
}
func (service *Service) RegroupSnapshot(ctx context.Context, id string, player int64) (map[string]any, error) {
	var count int
	if err := service.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM pve_regroup_members WHERE proposal_id=? AND player_id=?", id, player).Scan(&count); err != nil {
		return nil, err
	}
	if count != 1 {
		return nil, store.Forbidden
	}
	var owner int64
	var status string
	var revision int64
	var plan json.RawMessage
	var room sql.NullString
	var expires time.Time
	if err := service.DB.QueryRowContext(ctx, "SELECT owner_id,status,revision,plan,result_party_id,expires_at FROM pve_regroup_proposals WHERE id=?", id).Scan(&owner, &status, &revision, &plan, &room, &expires); err != nil {
		return nil, err
	}
	if status == "pending" && !time.Now().Before(expires) {
		status = "expired"
	}
	rows, err := service.DB.QueryContext(ctx, "SELECT player_id,response FROM pve_regroup_members WHERE proposal_id=? ORDER BY player_id", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	members := []map[string]any{}
	for rows.Next() {
		var target int64
		var response string
		if err := rows.Scan(&target, &response); err != nil {
			return nil, err
		}
		members = append(members, map[string]any{"player_id": target, "response": response})
	}
	return map[string]any{"id": id, "owner_id": owner, "status": status, "revision": revision, "plan": plan, "party_id": room.String, "members": members, "expires_at": expires}, rows.Err()
}
