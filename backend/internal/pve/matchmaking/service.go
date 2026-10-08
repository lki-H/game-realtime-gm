package matchmaking

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"game-realtime-gm/backend/internal/pve/control"
	"game-realtime-gm/backend/internal/pve/party"
	"game-realtime-gm/backend/internal/pve/run"
	"game-realtime-gm/backend/internal/pve/store"
	"game-realtime-gm/backend/internal/pve/task"
)

type Service struct {
	DB                *sql.DB
	Runs              *run.Service
	Now               func() time.Time
	MaxProposalRounds int
}
type Request struct {
	PartyID    string            `json:"party_id"`
	ProposalID string            `json:"proposal_id"`
	Revision   int64             `json:"revision"`
	Plan       party.Plan        `json:"plan"`
	Task       run.TaskSelection `json:"task"`
}
type Ticket struct {
	ID          string `json:"id"`
	PartyID     sql.NullString
	Operation   string
	Difficulty  string
	Fill        string
	Partial     bool
	Since       time.Time
	Members     []int64
	RuleVersion string
	Cohort      string
}

func NewService(db *sql.DB, runs *run.Service) *Service {
	return &Service{DB: db, Runs: runs, Now: time.Now, MaxProposalRounds: 8}
}
func Lane(ctx context.Context, transaction *sql.Tx) error {
	var id string
	return transaction.QueryRowContext(ctx, "SELECT id FROM pve_match_lanes WHERE id='training_ground:normal' FOR UPDATE").Scan(&id)
}
func (s *Service) Execute(ctx context.Context, transaction *sql.Tx, player int64, kind string, input Request) (any, error) {
	if err := Lane(ctx, transaction); err != nil {
		return nil, err
	}
	switch kind {
	case "enqueue":
		if err := control.RequireOpen(ctx, transaction); err != nil {
			return nil, err
		}
		if input.PartyID == "" {
			if err := store.RequireNoRecruitment(ctx, transaction, player); err != nil {
				return nil, err
			}
			var count int
			if err := transaction.QueryRowContext(ctx, "SELECT COUNT(*) FROM pve_party_members WHERE player_id=? AND status='active'", player).Scan(&count); err != nil {
				return nil, err
			}
			if count > 0 {
				return nil, store.Conflict
			}
		}
		plan := input.Plan
		members := []party.Member{{PlayerID: player, TaskSelection: store.JSON(input.Task)}}
		if input.PartyID != "" {
			group, err := party.Load(ctx, transaction, input.PartyID)
			if err != nil {
				return nil, err
			}
			if group.OwnerID != player {
				return nil, store.Forbidden
			}
			if group.Status != "open" {
				return nil, store.Conflict
			}
			for _, member := range group.Members {
				if !member.Ready {
					return nil, store.Conflict
				}
			}
			members = group.Members
			plan = group.Plan
			rows, err := transaction.QueryContext(ctx, "SELECT r.player_id,p.ready,p.roster_version,p.plan_version,p.selection_version,p.task_selection,p.expires_at FROM pve_recruitment_roster r JOIN pve_recruitment_preferences p ON p.party_id=r.party_id AND p.player_id=r.player_id WHERE r.party_id=?", group.ID)
			if err != nil {
				return nil, err
			}
			for rows.Next() {
				var guest int64
				var ready bool
				var rosterVersion, planVersion, selectionVersion int64
				var selection json.RawMessage
				var expires time.Time
				if err := rows.Scan(&guest, &ready, &rosterVersion, &planVersion, &selectionVersion, &selection, &expires); err != nil {
					rows.Close()
					return nil, err
				}
				if !ready || rosterVersion != group.RosterVersion || planVersion != group.PlanVersion || !s.Now().Before(expires) {
					rows.Close()
					return nil, store.Conflict
				}
				members = append(members, party.Member{PlayerID: guest, TaskSelection: selection, SelectionVersion: selectionVersion})
			}
			err = rows.Err()
			rows.Close()
			if err != nil {
				return nil, err
			}
		}
		if plan.RuleVersion == "" {
			plan.RuleVersion = s.Runs.Rules.Version
		}
		rules, err := task.RuleTx(ctx, transaction, plan.Operation, plan.RuleVersion)
		if err != nil {
			return nil, err
		}
		if plan.Operation != rules.Operation || plan.Difficulty != rules.Difficulty || (plan.FillPolicy != "public" && plan.FillPolicy != "no_fill") || len(members) > 4 {
			return nil, store.Invalid
		}
		for _, member := range members {
			if err := store.RequireIdle(ctx, transaction, member.PlayerID); err != nil {
				return nil, err
			}
			var status string
			if err := transaction.QueryRowContext(ctx, "SELECT status FROM players WHERE id=?", member.PlayerID).Scan(&status); err != nil {
				return nil, err
			}
			if status != "normal" {
				return nil, store.Forbidden
			}
			if member.MembershipID == "" && input.PartyID != "" {
				var rooms int
				if err := transaction.QueryRowContext(ctx, "SELECT COUNT(*) FROM pve_party_members WHERE player_id=? AND status='active'", member.PlayerID).Scan(&rooms); err != nil {
					return nil, err
				}
				if rooms > 0 {
					return nil, store.Conflict
				}
			}
		}
		for firstIndex, first := range members {
			for _, second := range members[firstIndex+1:] {
				blocked, err := store.Blocked(ctx, transaction, first.PlayerID, second.PlayerID)
				if err != nil {
					return nil, err
				}
				if blocked {
					return nil, store.Forbidden
				}
			}
		}
		id := store.ID("ticket")
		now := s.Now().UTC()
		var source any
		if input.PartyID != "" {
			source = input.PartyID
		}
		_, err = transaction.ExecContext(ctx, "INSERT INTO pve_match_tickets(id,source_party_id,operation_id,operation_name,difficulty,fill_policy,allow_partial,queue_priority_since,stage_entered_at) VALUES(?,?,?,?,?,?,?,?,?)", id, source, id, plan.Operation, plan.Difficulty, plan.FillPolicy, plan.AllowPartial, now, now)
		if err != nil {
			return nil, err
		}
		rootKind := "solo"
		if input.PartyID != "" {
			rootKind = "party"
		}
		if _, err := transaction.ExecContext(ctx, "INSERT INTO pve_match_ticket_plans(ticket_id,rule_version,cohort_id,source_kind) VALUES(?,?,?,?)", id, rules.Version, id, rootKind); err != nil {
			return nil, err
		}
		for _, member := range members {
			var selection run.TaskSelection
			if len(member.TaskSelection) > 0 {
				if err := json.Unmarshal(member.TaskSelection, &selection); err != nil {
					return nil, err
				}
			}
			if selection.TaskKey != "" {
				definition, exists := rules.Task(selection.TaskKey)
				if !exists || selection.TaskVersion != definition.LogicalVersion(rules) {
					return nil, store.Invalid
				}
				progress, err := task.LoadProgress(ctx, transaction, member.PlayerID, definition, rules, definition.Period(now))
				if err != nil {
					return nil, err
				}
				if progress.Completed || !progress.Unlocked {
					return nil, store.Conflict
				}
			}
			memberTicket := id
			kind := rootKind
			if input.PartyID != "" && member.MembershipID == "" {
				memberTicket = store.ID("ticket")
				kind = "recruitment"
				if _, err := transaction.ExecContext(ctx, "INSERT INTO pve_match_tickets(id,operation_id,operation_name,difficulty,fill_policy,allow_partial,queue_priority_since,stage_entered_at) VALUES(?,?,?,?,?,?,?,?)", memberTicket, memberTicket, plan.Operation, plan.Difficulty, plan.FillPolicy, plan.AllowPartial, now, now); err != nil {
					return nil, err
				}
				if _, err := transaction.ExecContext(ctx, "INSERT INTO pve_match_ticket_plans(ticket_id,rule_version,cohort_id,source_kind) VALUES(?,?,?,'recruitment')", memberTicket, rules.Version, id); err != nil {
					return nil, err
				}
			}
			if _, err := transaction.ExecContext(ctx, "INSERT INTO pve_match_ticket_members(ticket_id,player_id,membership_id,source_kind,selection_version,task_selection) VALUES(?,?,?,?,?,?)", memberTicket, member.PlayerID, nullable(member.MembershipID), kind, member.SelectionVersion, store.JSON(selection)); err != nil {
				return nil, err
			}
			if _, err := transaction.ExecContext(ctx, "INSERT INTO pve_player_activity_locks(player_id,activity_type,activity_id) VALUES(?,'ticket',?)", member.PlayerID, memberTicket); err != nil {
				return nil, err
			}
		}
		if input.PartyID != "" {
			if _, err := transaction.ExecContext(ctx, "UPDATE pve_parties SET status='queued' WHERE id=?", input.PartyID); err != nil {
				return nil, err
			}
			if _, err := transaction.ExecContext(ctx, "DELETE FROM pve_recruitment_roster WHERE party_id=?", input.PartyID); err != nil {
				return nil, err
			}
			if _, err := transaction.ExecContext(ctx, "DELETE FROM pve_recruitment_preferences WHERE party_id=?", input.PartyID); err != nil {
				return nil, err
			}
		}
		return map[string]any{"ticket_id": id, "status": "queued"}, nil
	case "cancel":
		var ticket string
		err := transaction.QueryRowContext(ctx, "SELECT activity_id FROM pve_player_activity_locks WHERE player_id=? AND activity_type='ticket'", player).Scan(&ticket)
		if err != nil {
			return nil, store.NotFound
		}
		var source sql.NullString
		var status string
		if err := transaction.QueryRowContext(ctx, "SELECT source_party_id,status FROM pve_match_tickets WHERE id=? FOR UPDATE", ticket).Scan(&source, &status); err != nil {
			return nil, err
		}
		if source.Valid {
			group, err := party.Load(ctx, transaction, source.String)
			if err != nil {
				return nil, err
			}
			if group.OwnerID != player {
				return nil, store.Forbidden
			}
		}
		if status != "queued" {
			return nil, store.Conflict
		}
		if err := s.releaseTicket(ctx, transaction, ticket, false, ""); err != nil {
			return nil, err
		}
		return map[string]any{"canceled": true}, nil
	case "proposal_confirm", "proposal_reject":
		var status string
		var revision int64
		var deadline time.Time
		var existingRun sql.NullString
		err := transaction.QueryRowContext(ctx, "SELECT status,revision,deadline_at,run_id FROM pve_match_proposals WHERE id=? FOR UPDATE", input.ProposalID).Scan(&status, &revision, &deadline, &existingRun)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, store.NotFound
		}
		if err != nil {
			return nil, err
		}
		if revision != input.Revision || status != "pending" || !s.Now().Before(deadline) {
			return nil, store.Conflict
		}
		var ticket, response string
		err = transaction.QueryRowContext(ctx, "SELECT ticket_id,response FROM pve_match_proposal_members WHERE proposal_id=? AND player_id=?", input.ProposalID, player).Scan(&ticket, &response)
		if err != nil {
			return nil, store.Forbidden
		}
		if kind == "proposal_reject" {
			if err := s.reject(ctx, transaction, input.ProposalID, []string{ticket}); err != nil {
				return nil, err
			}
			return map[string]any{"rejected": true}, nil
		}
		if _, err := transaction.ExecContext(ctx, "UPDATE pve_match_proposal_members SET response='accepted',responded_at=UTC_TIMESTAMP(3) WHERE proposal_id=? AND player_id=?", input.ProposalID, player); err != nil {
			return nil, err
		}
		var pending int
		if err := transaction.QueryRowContext(ctx, "SELECT COUNT(*) FROM pve_match_proposal_members WHERE proposal_id=? AND response<>'accepted'", input.ProposalID).Scan(&pending); err != nil {
			return nil, err
		}
		if pending > 0 {
			return map[string]any{"confirmed": true}, nil
		}
		var proposalPlayers []int64
		rows, err := transaction.QueryContext(ctx, "SELECT player_id FROM pve_match_proposal_members WHERE proposal_id=? ORDER BY player_id", input.ProposalID)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return nil, err
			}
			proposalPlayers = append(proposalPlayers, id)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
		if err := store.LockPlayers(ctx, transaction, proposalPlayers); err != nil {
			return nil, err
		}
		var ruleVersion string
		if err := transaction.QueryRowContext(ctx, "SELECT p.rule_version FROM pve_match_ticket_plans p JOIN pve_match_proposal_members m ON m.ticket_id=p.ticket_id WHERE m.proposal_id=? LIMIT 1", input.ProposalID).Scan(&ruleVersion); err != nil {
			return nil, err
		}
		request := run.CreateRequest{RuleVersion: ruleVersion, OperationName: s.Runs.Rules.Operation, Difficulty: s.Runs.Rules.Difficulty, Tasks: map[int64]run.TaskSelection{}, Tickets: map[int64]string{}}
		rows, err = transaction.QueryContext(ctx, "SELECT m.player_id,m.ticket_id,t.task_selection FROM pve_match_proposal_members m JOIN pve_match_ticket_members t ON t.ticket_id=m.ticket_id AND t.player_id=m.player_id WHERE m.proposal_id=? ORDER BY m.player_id", input.ProposalID)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id int64
			var ticket string
			var data []byte
			if err := rows.Scan(&id, &ticket, &data); err != nil {
				rows.Close()
				return nil, err
			}
			var selection run.TaskSelection
			if err := json.Unmarshal(data, &selection); err != nil {
				rows.Close()
				return nil, err
			}
			request.PlayerIDs = append(request.PlayerIDs, id)
			request.Tasks[id] = selection
			request.Tickets[id] = ticket
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
		created, err := s.Runs.CreateTx(ctx, transaction, request)
		if err != nil {
			return nil, err
		}
		for firstIndex, first := range request.PlayerIDs {
			for _, second := range request.PlayerIDs[firstIndex+1:] {
				blocked, err := store.Blocked(ctx, transaction, first, second)
				if err != nil {
					return nil, err
				}
				if blocked {
					return nil, store.Forbidden
				}
			}
		}
		if _, err := transaction.ExecContext(ctx, "UPDATE pve_match_proposals SET status='confirmed',run_id=? WHERE id=?", created.ID, input.ProposalID); err != nil {
			return nil, err
		}
		for _, id := range request.PlayerIDs {
			if _, err := transaction.ExecContext(ctx, "UPDATE pve_player_activity_locks SET activity_type='run',activity_id=? WHERE player_id=?", created.ID, id); err != nil {
				return nil, err
			}
		}
		if _, err := transaction.ExecContext(ctx, "UPDATE pve_match_tickets SET status='assigned' WHERE id IN (SELECT ticket_id FROM pve_match_proposal_members WHERE proposal_id=?)", input.ProposalID); err != nil {
			return nil, err
		}
		if _, err := transaction.ExecContext(ctx, "UPDATE pve_parties SET status='in_run' WHERE id IN (SELECT source_party_id FROM pve_match_tickets WHERE id IN (SELECT ticket_id FROM pve_match_proposal_members WHERE proposal_id=?))", input.ProposalID); err != nil {
			return nil, err
		}
		if err := store.Notify(ctx, transaction, request.PlayerIDs, "v2.match.assigned", map[string]any{"run_id": created.ID}); err != nil {
			return nil, err
		}
		return map[string]any{"run_id": created.ID, "status": "loading"}, nil
	default:
		return nil, store.NotFound
	}
}
func (s *Service) Match(ctx context.Context) error {
	return store.Transaction(ctx, s.DB, func(transaction *sql.Tx) error {
		if err := Lane(ctx, transaction); err != nil {
			return err
		}
		if err := control.RequireOpen(ctx, transaction); err != nil {
			if errors.Is(err, store.Maintenance) {
				return nil
			}
			return err
		}
		rows, err := transaction.QueryContext(ctx, "SELECT t.id,t.source_party_id,t.operation_name,t.difficulty,t.fill_policy,t.allow_partial,t.queue_priority_since,p.rule_version,COALESCE(p.cohort_id,'') FROM pve_match_tickets t JOIN pve_match_ticket_plans p ON p.ticket_id=t.id WHERE t.status='queued' ORDER BY t.queue_priority_since,t.id LIMIT 100")
		if err != nil {
			return err
		}
		tickets := []Ticket{}
		for rows.Next() {
			var ticket Ticket
			if err := rows.Scan(&ticket.ID, &ticket.PartyID, &ticket.Operation, &ticket.Difficulty, &ticket.Fill, &ticket.Partial, &ticket.Since, &ticket.RuleVersion, &ticket.Cohort); err != nil {
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
		for index := range tickets {
			rows, err := transaction.QueryContext(ctx, "SELECT player_id FROM pve_match_ticket_members WHERE ticket_id=? ORDER BY player_id", tickets[index].ID)
			if err != nil {
				return err
			}
			for rows.Next() {
				var id int64
				if err := rows.Scan(&id); err != nil {
					rows.Close()
					return err
				}
				tickets[index].Members = append(tickets[index].Members, id)
			}
			err = rows.Err()
			rows.Close()
			if err != nil {
				return err
			}
		}
		used := map[string]bool{}
		for _, anchor := range tickets {
			if used[anchor.ID] {
				continue
			}
			rules, err := task.RuleTx(ctx, transaction, anchor.Operation, anchor.RuleVersion)
			if errors.Is(err, store.Conflict) {
				continue
			}
			if err != nil {
				return err
			}
			selected := []Ticket{anchor}
			count := len(anchor.Members)
			selectedIDs := map[string]bool{anchor.ID: true}
			if anchor.Cohort != "" {
				cohort := []Ticket{}
				cohortCount := count
				for _, candidate := range tickets {
					if candidate.ID != anchor.ID && !used[candidate.ID] && candidate.Cohort == anchor.Cohort && candidate.RuleVersion == anchor.RuleVersion {
						cohort = append(cohort, candidate)
						cohortCount += len(candidate.Members)
					}
				}
				if cohortCount <= 4 {
					for _, candidate := range cohort {
						selected = append(selected, candidate)
						selectedIDs[candidate.ID] = true
					}
					count = cohortCount
				}
			}
			if count > 4 {
				continue
			}
			if anchor.Fill == "public" && count < 4 {
				for _, candidate := range tickets {
					if selectedIDs[candidate.ID] || used[candidate.ID] || candidate.Fill != "public" || candidate.RuleVersion != anchor.RuleVersion || candidate.Operation != anchor.Operation || candidate.Difficulty != anchor.Difficulty || count+len(candidate.Members) > 4 {
						continue
					}
					family := []Ticket{candidate}
					familyCount := len(candidate.Members)
					if candidate.Cohort != "" {
						for _, relative := range tickets {
							if relative.ID != candidate.ID && !used[relative.ID] && !selectedIDs[relative.ID] && relative.Cohort == candidate.Cohort {
								family = append(family, relative)
								familyCount += len(relative.Members)
							}
						}
					}
					if count+familyCount > 4 {
						continue
					}
					blocked := false
					for _, existing := range selected {
						for _, first := range existing.Members {
							for _, relative := range family {
								for _, second := range relative.Members {
									hasBlock, err := store.Blocked(ctx, transaction, first, second)
									if err != nil {
										return err
									}
									blocked = blocked || hasBlock
								}
							}
						}
					}
					if blocked {
						continue
					}
					selected = append(selected, family...)
					for _, relative := range family {
						selectedIDs[relative.ID] = true
					}
					count += familyCount
				}
			}
			if count < 4 && anchor.Fill == "public" {
				allowed := true
				for _, ticket := range selected {
					allowed = allowed && ticket.Partial && s.Now().Sub(ticket.Since) >= time.Duration(rules.FillWait)*time.Second
				}
				if !allowed {
					continue
				}
			}
			proposal := store.ID("proposal")
			deadline := s.Now().UTC().Add(time.Duration(rules.Confirmation) * time.Second)
			if _, err := transaction.ExecContext(ctx, "INSERT INTO pve_match_proposals(id,operation_name,difficulty,deadline_at) VALUES(?,?,?,?)", proposal, anchor.Operation, anchor.Difficulty, deadline); err != nil {
				return err
			}
			players := []int64{}
			for _, ticket := range selected {
				used[ticket.ID] = true
				if _, err := transaction.ExecContext(ctx, "UPDATE pve_match_tickets SET status='proposed' WHERE id=? AND status='queued'", ticket.ID); err != nil {
					return err
				}
				for _, player := range ticket.Members {
					players = append(players, player)
					if _, err := transaction.ExecContext(ctx, "INSERT INTO pve_match_proposal_members(proposal_id,ticket_id,player_id) VALUES(?,?,?)", proposal, ticket.ID, player); err != nil {
						return err
					}
				}
			}
			if err := store.Notify(ctx, transaction, players, "v2.match.proposal", map[string]any{"proposal_id": proposal, "revision": 1, "players": players, "deadline_at": deadline}); err != nil {
				return err
			}
		}
		return nil
	})
}
func (s *Service) releaseTicket(ctx context.Context, transaction *sql.Tx, ticket string, requeue bool, expectedRun string) error {
	if requeue {
		if err := control.RequireOpen(ctx, transaction); err != nil {
			if !errors.Is(err, store.Maintenance) {
				return err
			}
			requeue = false
		}
	}
	if _, err := transaction.ExecContext(ctx, "UPDATE pve_match_ticket_plans SET cohort_id=NULL WHERE ticket_id=?", ticket); err != nil {
		return err
	}
	if requeue && s.MaxProposalRounds > 0 {
		var rounds int
		if err := transaction.QueryRowContext(ctx, "SELECT COUNT(DISTINCT proposal_id) FROM pve_match_proposal_members WHERE ticket_id=?", ticket).Scan(&rounds); err != nil {
			return err
		}
		if rounds >= s.MaxProposalRounds {
			requeue = false
			rows, err := transaction.QueryContext(ctx, "SELECT player_id FROM pve_match_ticket_members WHERE ticket_id=?", ticket)
			if err != nil {
				return err
			}
			players := []int64{}
			for rows.Next() {
				var player int64
				if err := rows.Scan(&player); err != nil {
					rows.Close()
					return err
				}
				players = append(players, player)
			}
			err = rows.Err()
			rows.Close()
			if err != nil {
				return err
			}
			if err := store.Notify(ctx, transaction, players, "v2.match.paused", map[string]any{"ticket_id": ticket, "reason": "proposal_round_limit"}); err != nil {
				return err
			}
		}
	}
	next := "canceled"
	if requeue {
		next = "queued"
	}
	if _, err := transaction.ExecContext(ctx, "UPDATE pve_match_tickets SET status=?,stage_entered_at=UTC_TIMESTAMP(3) WHERE id=?", next, ticket); err != nil {
		return err
	}
	if !requeue {
		if _, err := transaction.ExecContext(ctx, "DELETE FROM pve_player_activity_locks WHERE ((activity_type='ticket' AND activity_id=?) OR (activity_type='run' AND activity_id=?)) AND player_id IN (SELECT player_id FROM pve_match_ticket_members WHERE ticket_id=?)", ticket, expectedRun, ticket); err != nil {
			return err
		}
		if _, err := transaction.ExecContext(ctx, "UPDATE pve_parties SET status='open' WHERE id=(SELECT source_party_id FROM pve_match_tickets WHERE id=?)", ticket); err != nil {
			return err
		}
		_, err := transaction.ExecContext(ctx, "UPDATE pve_party_members SET ready=0 WHERE party_id=(SELECT source_party_id FROM pve_match_tickets WHERE id=?)", ticket)
		return err
	}
	_, err := transaction.ExecContext(ctx, "UPDATE pve_player_activity_locks SET activity_type='ticket',activity_id=? WHERE ((activity_type='ticket' AND activity_id=?) OR (activity_type='run' AND activity_id=?)) AND player_id IN (SELECT player_id FROM pve_match_ticket_members WHERE ticket_id=?)", ticket, ticket, expectedRun, ticket)
	return err
}
func (s *Service) reject(ctx context.Context, transaction *sql.Tx, proposal string, failed []string) error {
	rows, err := transaction.QueryContext(ctx, "SELECT DISTINCT ticket_id FROM pve_match_proposal_members WHERE proposal_id=?", proposal)
	if err != nil {
		return err
	}
	tickets := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		tickets = append(tickets, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, ticket := range tickets {
		responsible := false
		for _, id := range failed {
			responsible = responsible || ticket == id
		}
		if err := s.releaseTicket(ctx, transaction, ticket, !responsible, ""); err != nil {
			return err
		}
	}
	_, err = transaction.ExecContext(ctx, "UPDATE pve_match_proposals SET status='rejected' WHERE id=?", proposal)
	return err
}
func (s *Service) Expire(ctx context.Context) error {
	return store.Transaction(ctx, s.DB, func(transaction *sql.Tx) error {
		if err := Lane(ctx, transaction); err != nil {
			return err
		}
		rows, err := transaction.QueryContext(ctx, "SELECT id FROM pve_match_proposals WHERE status='pending' AND deadline_at<=?", s.Now().UTC())
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
			rows, err := transaction.QueryContext(ctx, "SELECT DISTINCT ticket_id FROM pve_match_proposal_members WHERE proposal_id=? AND response<>'accepted'", id)
			if err != nil {
				return err
			}
			failed := []string{}
			for rows.Next() {
				var ticket string
				if err := rows.Scan(&ticket); err != nil {
					rows.Close()
					return err
				}
				failed = append(failed, ticket)
			}
			rows.Close()
			if err := s.reject(ctx, transaction, id, failed); err != nil {
				return err
			}
		}
		return nil
	})
}
func (s *Service) LoadingExpired(ctx context.Context, id string) error {
	return store.Transaction(ctx, s.DB, func(transaction *sql.Tx) error {
		if err := Lane(ctx, transaction); err != nil {
			return err
		}
		state, err := run.Load(ctx, transaction, id)
		if err != nil {
			return err
		}
		if state.Status != "loading" || s.Now().Before(state.DeadlineAt) {
			return nil
		}
		failed := map[string]bool{}
		tickets := map[string]bool{}
		for _, member := range state.Participants {
			tickets[member.TicketID] = true
			if member.Status == "loading" {
				failed[member.TicketID] = true
			}
		}
		for ticket := range tickets {
			if ticket != "" {
				if err := s.releaseTicket(ctx, transaction, ticket, !failed[ticket], id); err != nil {
					return err
				}
			}
		}
		state.Status = "aborted"
		state.EndReason = "loading_failed"
		now := s.Now().UTC()
		state.EndedAt = &now
		for index := range state.Participants {
			state.Participants[index].Status = "left"
		}
		return run.Save(ctx, transaction, state)
	})
}
func nullable(value string) any {
	if value == "" {
		return nil
	}
	return value
}
