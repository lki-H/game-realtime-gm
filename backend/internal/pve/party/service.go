package party

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"game-realtime-gm/backend/internal/pve/store"
	"game-realtime-gm/backend/internal/pve/task"
)

type Plan struct {
	RuleVersion  string   `json:"rule_version,omitempty"`
	Operation    string   `json:"operation"`
	Difficulty   string   `json:"difficulty"`
	FillPolicy   string   `json:"fill_policy"`
	AllowPartial bool     `json:"allow_partial"`
	Tags         []string `json:"tags,omitempty"`
}
type Request struct {
	PartyID          string  `json:"party_id"`
	PlayerID         int64   `json:"player_id"`
	Token            string  `json:"token"`
	Ready            bool    `json:"ready"`
	RosterVersion    int64   `json:"roster_version"`
	PlanVersion      int64   `json:"plan_version"`
	SelectionVersion int64   `json:"selection_version"`
	TaskKey          string  `json:"task_key"`
	TaskVersion      string  `json:"task_version"`
	JoinPolicy       string  `json:"join_policy"`
	Plan             Plan    `json:"plan"`
	PostID           string  `json:"post_id"`
	ApplicationID    int64   `json:"application_id"`
	Accept           bool    `json:"accept"`
	RunID            string  `json:"run_id"`
	ProposalID       string  `json:"proposal_id"`
	Revision         int64   `json:"revision"`
	OwnerID          int64   `json:"owner_id"`
	PlayerIDs        []int64 `json:"player_ids"`
}
type Member struct {
	PlayerID         int64           `json:"player_id"`
	MembershipID     string          `json:"membership_id"`
	Ready            bool            `json:"ready"`
	SelectionVersion int64           `json:"selection_version"`
	TaskSelection    json.RawMessage `json:"task_selection,omitempty"`
}
type Party struct {
	ID            string   `json:"id"`
	OwnerID       int64    `json:"owner_id"`
	RosterVersion int64    `json:"roster_version"`
	PlanVersion   int64    `json:"plan_version"`
	JoinPolicy    string   `json:"join_policy"`
	Status        string   `json:"status"`
	Plan          Plan     `json:"plan"`
	Members       []Member `json:"members"`
}
type Service struct {
	DB         *sql.DB
	RegroupTTL time.Duration
}

func (group *Party) ForPlayer(player int64) *Party {
	var result Party
	_ = json.Unmarshal(store.JSON(group), &result)
	for index := range result.Members {
		if result.Members[index].PlayerID != player {
			result.Members[index].TaskSelection = nil
		}
	}
	return &result
}

func NewService(db *sql.DB) *Service { return &Service{DB: db, RegroupTTL: 2 * time.Minute} }
func Load(ctx context.Context, transaction *sql.Tx, id string) (*Party, error) {
	return load(ctx, transaction, id, true)
}
func Read(ctx context.Context, transaction *sql.Tx, id string, player int64) (*Party, error) {
	group, err := load(ctx, transaction, id, false)
	if err != nil {
		return nil, err
	}
	if !Contains(group, player) {
		return nil, store.Forbidden
	}
	return group.ForPlayer(player), nil
}
func load(ctx context.Context, transaction *sql.Tx, id string, locked bool) (*Party, error) {
	var party Party
	var plan []byte
	query := "SELECT id,owner_id,roster_version,plan_version,join_policy,status,plan FROM pve_parties WHERE id=?"
	if locked {
		query += " FOR UPDATE"
	}
	err := transaction.QueryRowContext(ctx, query, id).Scan(&party.ID, &party.OwnerID, &party.RosterVersion, &party.PlanVersion, &party.JoinPolicy, &party.Status, &plan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, store.NotFound
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(plan, &party.Plan); err != nil {
		return nil, err
	}
	rows, err := transaction.QueryContext(ctx, "SELECT player_id,membership_id,ready,selection_version,COALESCE(task_selection,JSON_OBJECT()) FROM pve_party_members WHERE party_id=? AND status='active' ORDER BY joined_at,player_id", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	party.Members = []Member{}
	for rows.Next() {
		var member Member
		if err := rows.Scan(&member.PlayerID, &member.MembershipID, &member.Ready, &member.SelectionVersion, &member.TaskSelection); err != nil {
			return nil, err
		}
		party.Members = append(party.Members, member)
	}
	return &party, rows.Err()
}
func (s *Service) Snapshot(ctx context.Context, id string, player int64) (*Party, error) {
	var result *Party
	err := store.Transaction(ctx, s.DB, func(transaction *sql.Tx) error {
		var err error
		result, err = Load(ctx, transaction, id)
		if err != nil {
			return err
		}
		if !Contains(result, player) {
			return store.Forbidden
		}
		for index := range result.Members {
			if result.Members[index].PlayerID != player {
				result.Members[index].TaskSelection = nil
			}
		}
		return nil
	})
	return result, err
}
func Contains(party *Party, player int64) bool {
	for _, member := range party.Members {
		if member.PlayerID == player {
			return true
		}
	}
	return false
}
func (s *Service) Execute(ctx context.Context, transaction *sql.Tx, player int64, kind string, input Request) (any, error) {
	if kind == "create" {
		if err := store.RequireIdle(ctx, transaction, player); err != nil {
			return nil, err
		}
		if err := store.RequireNoRecruitment(ctx, transaction, player); err != nil {
			return nil, err
		}
		if err := store.RequireNoRecruitment(ctx, transaction, player); err != nil {
			return nil, err
		}
		id := store.ID("party")
		plan := Plan{Operation: "training_ground", Difficulty: "normal", RuleVersion: "training_ground.v1", FillPolicy: "no_fill", AllowPartial: true}
		if _, err := transaction.ExecContext(ctx, "INSERT INTO pve_parties(id,owner_id,plan) VALUES(?,?,?)", id, player, store.JSON(plan)); err != nil {
			return nil, err
		}
		if _, err := transaction.ExecContext(ctx, "INSERT INTO pve_party_members(membership_id,party_id,player_id) VALUES(?,?,?)", store.ID("member"), id, player); err != nil {
			return nil, err
		}
		return Load(ctx, transaction, id)
	}
	if kind == "accept_invite" {
		var id string
		var invitee int64
		var expires time.Time
		var status string
		if err := transaction.QueryRowContext(ctx, "SELECT party_id,invitee_id,expires_at,status FROM pve_party_invites WHERE token_hash=? FOR UPDATE", store.Hash([]byte(input.Token))).Scan(&id, &invitee, &expires, &status); err != nil {
			return nil, store.NotFound
		}
		if invitee != player || status != "pending" || time.Now().UTC().After(expires) {
			return nil, store.Forbidden
		}
		input.PartyID = id
		if _, err := transaction.ExecContext(ctx, "UPDATE pve_party_invites SET status='accepted' WHERE token_hash=?", store.Hash([]byte(input.Token))); err != nil {
			return nil, err
		}
	}
	party, err := Load(ctx, transaction, input.PartyID)
	if err != nil {
		return nil, err
	}
	member := Contains(party, player)
	if kind == "join" || kind == "accept_invite" {
		var guests int
		if err := transaction.QueryRowContext(ctx, "SELECT COUNT(*) FROM pve_recruitment_preferences WHERE party_id=? AND expires_at>UTC_TIMESTAMP(3)", party.ID).Scan(&guests); err != nil {
			return nil, err
		}
		if party.Status != "open" || len(party.Members)+guests >= 4 {
			return nil, store.Conflict
		}
		if err := store.RequireIdle(ctx, transaction, player); err != nil {
			return nil, err
		}
		if err := store.RequireNoRecruitment(ctx, transaction, player); err != nil {
			return nil, err
		}
		if err := store.RequireNoRecruitment(ctx, transaction, player); err != nil {
			return nil, err
		}
		blocked, err := store.Blocked(ctx, transaction, party.OwnerID, player)
		if err != nil {
			return nil, err
		}
		if blocked {
			return nil, store.Forbidden
		}
		if kind == "join" {
			friends, err := store.Friends(ctx, transaction, party.OwnerID, player)
			if err != nil {
				return nil, err
			}
			if !friends || party.JoinPolicy != "friends_only" {
				return nil, store.Forbidden
			}
		}
		if _, err := transaction.ExecContext(ctx, "INSERT INTO pve_party_members(membership_id,party_id,player_id) VALUES(?,?,?)", store.ID("member"), party.ID, player); err != nil {
			return nil, err
		}
		return changed(ctx, transaction, party, true)
	}
	if !member {
		return nil, store.Forbidden
	}
	if kind == "ready" {
		if party.Status != "open" || input.RosterVersion != party.RosterVersion || input.PlanVersion != party.PlanVersion {
			return nil, store.Conflict
		}
		result, err := transaction.ExecContext(ctx, "UPDATE pve_party_members SET ready=? WHERE party_id=? AND player_id=? AND status='active' AND selection_version=?", input.Ready, party.ID, player, input.SelectionVersion)
		if err != nil {
			return nil, err
		}
		count, _ := result.RowsAffected()
		if count == 0 {
			var version int64
			if err := transaction.QueryRowContext(ctx, "SELECT selection_version FROM pve_party_members WHERE party_id=? AND player_id=? AND status='active'", party.ID, player).Scan(&version); err != nil || version != input.SelectionVersion {
				return nil, store.Conflict
			}
		}
		return Load(ctx, transaction, party.ID)
	}
	if kind == "selection" {
		if party.Status != "open" {
			return nil, store.Conflict
		}
		_, err := transaction.ExecContext(ctx, "UPDATE pve_party_members SET task_selection=?,selection_version=selection_version+1,ready=0 WHERE party_id=? AND player_id=? AND status='active'", store.JSON(map[string]any{"task_key": input.TaskKey, "task_version": input.TaskVersion}), party.ID, player)
		if err != nil {
			return nil, err
		}
		if _, err := transaction.ExecContext(ctx, "UPDATE pve_task_pauses SET paused=0 WHERE player_id=? AND task_key=? AND task_version=?", player, input.TaskKey, input.TaskVersion); err != nil {
			return nil, err
		}
		return Load(ctx, transaction, party.ID)
	}
	if kind == "leave" {
		if err := store.RequireIdle(ctx, transaction, player); err != nil {
			return nil, err
		}
		if party.Status != "open" {
			return nil, store.Conflict
		}
		if _, err := transaction.ExecContext(ctx, "UPDATE pve_party_members SET status='left',ready=0 WHERE party_id=? AND player_id=? AND status='active'", party.ID, player); err != nil {
			return nil, err
		}
		if len(party.Members) == 1 {
			if _, err := transaction.ExecContext(ctx, "UPDATE pve_recruitment_applications SET status='withdrawn' WHERE id IN (SELECT application_id FROM pve_recruitment_roster WHERE party_id=?) AND status IN ('pending','accepted')", party.ID); err != nil {
				return nil, err
			}
			if _, err := transaction.ExecContext(ctx, "DELETE FROM pve_recruitment_roster WHERE party_id=?", party.ID); err != nil {
				return nil, err
			}
			if _, err := transaction.ExecContext(ctx, "DELETE FROM pve_recruitment_preferences WHERE party_id=?", party.ID); err != nil {
				return nil, err
			}
			_, err := transaction.ExecContext(ctx, "UPDATE pve_parties SET status='closed' WHERE id=?", party.ID)
			return map[string]any{"closed": true}, err
		}
		if party.OwnerID == player {
			for _, candidate := range party.Members {
				if candidate.PlayerID != player {
					if _, err := transaction.ExecContext(ctx, "UPDATE pve_parties SET owner_id=? WHERE id=?", candidate.PlayerID, party.ID); err != nil {
						return nil, err
					}
					break
				}
			}
		}
		return changed(ctx, transaction, party, true)
	}
	if party.OwnerID != player {
		return nil, store.Forbidden
	}
	if party.Status != "open" {
		return nil, store.Conflict
	}
	switch kind {
	case "transfer":
		if !Contains(party, input.PlayerID) {
			return nil, store.Invalid
		}
		_, err := transaction.ExecContext(ctx, "UPDATE pve_parties SET owner_id=? WHERE id=?", input.PlayerID, party.ID)
		if err != nil {
			return nil, err
		}
		if _, err := transaction.ExecContext(ctx, "UPDATE pve_recruitment_posts SET owner_id=? WHERE party_id=? AND status='open'", input.PlayerID, party.ID); err != nil {
			return nil, err
		}
		return Load(ctx, transaction, party.ID)
	case "plan_update":
		if input.Plan.Operation != "training_ground" || input.Plan.Difficulty != "normal" || (input.Plan.FillPolicy != "no_fill" && input.Plan.FillPolicy != "public") || len(input.Plan.Tags) > 3 {
			return nil, store.Invalid
		}
		for _, tag := range input.Plan.Tags {
			if tag != "newcomer" && tag != "tasks" && tag != "experienced" {
				return nil, store.Invalid
			}
		}
		version := input.Plan.RuleVersion
		if version == "" {
			version = "training_ground.v1"
			input.Plan.RuleVersion = version
		}
		if _, err := task.RuleTx(ctx, transaction, input.Plan.Operation, version); err != nil {
			return nil, err
		}
		if _, err := transaction.ExecContext(ctx, "UPDATE pve_parties SET plan=?,fill_policy=?,plan_version=plan_version+1 WHERE id=?", store.JSON(input.Plan), input.Plan.FillPolicy, party.ID); err != nil {
			return nil, err
		}
		return changed(ctx, transaction, party, false)
	case "join_policy":
		if input.JoinPolicy != "invite_only" && input.JoinPolicy != "friends_only" {
			return nil, store.Invalid
		}
		_, err := transaction.ExecContext(ctx, "UPDATE pve_parties SET join_policy=? WHERE id=?", input.JoinPolicy, party.ID)
		if err != nil {
			return nil, err
		}
		return Load(ctx, transaction, party.ID)
	case "invite":
		if input.PlayerID <= 0 || len(party.Members) >= 4 {
			return nil, store.Invalid
		}
		blocked, err := store.Blocked(ctx, transaction, player, input.PlayerID)
		if err != nil {
			return nil, err
		}
		if blocked {
			return nil, store.Forbidden
		}
		token := rand.Text()
		_, err = transaction.ExecContext(ctx, "INSERT INTO pve_party_invites(party_id,inviter_id,invitee_id,token_hash,expires_at) VALUES(?,?,?,?,?)", party.ID, player, input.PlayerID, store.Hash([]byte(token)), time.Now().UTC().Add(10*time.Minute))
		if err != nil {
			return nil, err
		}
		value := map[string]any{"party_id": party.ID, "token": token}
		return value, store.Notify(ctx, transaction, []int64{input.PlayerID}, "v2.party.invited", value)
	default:
		return nil, store.NotFound
	}
}
func changed(ctx context.Context, transaction *sql.Tx, party *Party, roster bool) (any, error) {
	if roster {
		if _, err := transaction.ExecContext(ctx, "UPDATE pve_parties SET roster_version=roster_version+1 WHERE id=?", party.ID); err != nil {
			return nil, err
		}
	}
	if _, err := transaction.ExecContext(ctx, "UPDATE pve_party_members SET ready=0 WHERE party_id=? AND status='active'", party.ID); err != nil {
		return nil, err
	}
	if _, err := transaction.ExecContext(ctx, "UPDATE pve_recruitment_preferences SET ready=0 WHERE party_id=?", party.ID); err != nil {
		return nil, err
	}
	if _, err := transaction.ExecContext(ctx, "UPDATE pve_recruitment_posts SET status='closed' WHERE party_id=? AND status='open'", party.ID); err != nil {
		return nil, err
	}
	return Load(ctx, transaction, party.ID)
}
