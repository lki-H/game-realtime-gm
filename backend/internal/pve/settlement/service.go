package settlement

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"game-realtime-gm/backend/internal/pve/run"
	"game-realtime-gm/backend/internal/pve/store"
	"game-realtime-gm/backend/internal/pve/task"
	"math"
	"time"
)

var ErrSettlementPending = errors.New("pve settlement pending participants")

type Service struct{ DB *sql.DB }

func NewService(db *sql.DB) *Service { return &Service{DB: db} }
func (service *Service) CompleteTask(ctx context.Context, runID string, payload json.RawMessage) error {
	var input struct {
		PlayerID int64 `json:"player_id"`
	}
	if json.Unmarshal(payload, &input) != nil || input.PlayerID <= 0 {
		return store.Invalid
	}
	return store.Transaction(ctx, service.DB, func(transaction *sql.Tx) error {
		state, err := run.Load(ctx, transaction, runID)
		if err != nil {
			return err
		}
		member := state.Member(&input.PlayerID)
		if member == nil {
			return store.Forbidden
		}
		definition, exists := state.Rules.Task(member.TaskKey)
		if !exists || !definition.Immediate() || member.Compatibility != "compatible" {
			return store.Conflict
		}
		if member.TaskCompleted {
			return nil
		}
		for _, objective := range definition.Objectives {
			if member.TaskProgress[objective.Key] < objective.Required {
				return store.Conflict
			}
		}
		if state.Status != "running" && state.Status != "ending" && state.Status != "closed" {
			return store.Conflict
		}
		if err := settleTask(ctx, transaction, state, member); err != nil {
			return err
		}
		return run.Save(ctx, transaction, state)
	})
}
func (service *Service) Settle(ctx context.Context, runID string) error {
	state, err := service.snapshot(ctx, runID)
	if err != nil {
		return err
	}
	for _, participant := range state.Participants {
		err = errors.Join(err, service.SettleParticipant(ctx, runID, participant.PlayerID))
	}
	return errors.Join(err, service.FinalizeRun(ctx, runID))
}
func (service *Service) SettleParticipant(ctx context.Context, runID string, playerID int64) error {
	return store.Transaction(ctx, service.DB, func(transaction *sql.Tx) error {
		state, err := run.Load(ctx, transaction, runID)
		if err != nil {
			return err
		}
		if state.Status != "ending" && state.Status != "closed" {
			return store.Conflict
		}
		member := state.Member(&playerID)
		if member == nil {
			return store.Forbidden
		}
		var existing int
		if err := transaction.QueryRowContext(ctx, "SELECT COUNT(*) FROM pve_participant_results WHERE run_id=? AND player_id=? AND result_status='settled'", runID, playerID).Scan(&existing); err != nil {
			return err
		}
		if existing > 0 {
			return nil
		}
		wasLeft := member.Status == "left"
		if err := grant(ctx, transaction, state.ID, member.PlayerID, "base", baseReward(state, member.Contribution > 0 && !wasLeft)); err != nil {
			return err
		}
		if member.TaskKey != "" && member.Compatibility == "compatible" {
			if err := settleTask(ctx, transaction, state, member); err != nil {
				return err
			}
		}
		if _, err := transaction.ExecContext(ctx, `INSERT INTO pve_participant_results(run_id,player_id,result_status,contribution_qualified,task_completed,settled_at) VALUES(?,?,'settled',?,?,?) ON DUPLICATE KEY UPDATE result_status='settled', contribution_qualified=VALUES(contribution_qualified), task_completed=VALUES(task_completed), settled_at=VALUES(settled_at)`, state.ID, member.PlayerID, member.Contribution > 0 && !wasLeft, member.TaskCompleted, time.Now().UTC()); err != nil {
			return err
		}
		if _, err := transaction.ExecContext(ctx, "DELETE FROM pve_player_activity_locks WHERE player_id=? AND activity_id=?", member.PlayerID, state.ID); err != nil {
			return err
		}
		if !wasLeft {
			member.Status = "completed"
		}
		if err := run.Save(ctx, transaction, state); err != nil {
			return err
		}
		return store.Notify(ctx, transaction, []int64{member.PlayerID}, "v2.participant.result", map[string]any{"run_id": state.ID, "player_id": member.PlayerID, "status": "settled"})
	})
}
func (service *Service) FinalizeRun(ctx context.Context, runID string) error {
	return store.Transaction(ctx, service.DB, func(transaction *sql.Tx) error {
		state, err := run.Load(ctx, transaction, runID)
		if err != nil {
			return err
		}
		if state.Settled {
			return nil
		}
		var settled int
		if err := transaction.QueryRowContext(ctx, "SELECT COUNT(*) FROM pve_participant_results WHERE run_id=? AND result_status='settled'", runID).Scan(&settled); err != nil {
			return err
		}
		if settled < len(state.Participants) {
			return ErrSettlementPending
		}
		for _, participant := range state.Participants {
			if !participant.TaskCompleted {
				continue
			}
			if _, err := transaction.ExecContext(ctx, "UPDATE pve_party_members SET task_selection=JSON_OBJECT(),selection_version=selection_version+1,ready=0 WHERE player_id=? AND status='active' AND JSON_UNQUOTE(JSON_EXTRACT(task_selection,'$.task_key'))=? AND JSON_UNQUOTE(JSON_EXTRACT(task_selection,'$.task_version'))=?", participant.PlayerID, participant.TaskKey, participant.TaskVersion); err != nil {
				return err
			}
		}
		if _, err := transaction.ExecContext(ctx, "UPDATE pve_parties SET status='open' WHERE id IN (SELECT source_party_id FROM pve_match_tickets WHERE id IN (SELECT source_ticket_id FROM pve_run_participants WHERE run_id=?))", state.ID); err != nil {
			return err
		}
		if _, err := transaction.ExecContext(ctx, "UPDATE pve_party_members SET ready=0 WHERE party_id IN (SELECT source_party_id FROM pve_match_tickets WHERE id IN (SELECT source_ticket_id FROM pve_run_participants WHERE run_id=?))", state.ID); err != nil {
			return err
		}
		state.Settled = true
		state.Status = "closed"
		if err := run.Save(ctx, transaction, state); err != nil {
			return err
		}
		return store.Notify(ctx, transaction, state.PlayerIDs(), "v2.run.result", map[string]any{"run_id": state.ID, "reason": state.EndReason, "settled": true})
	})
}
func (service *Service) snapshot(ctx context.Context, runID string) (*run.State, error) {
	var data []byte
	if err := service.DB.QueryRowContext(ctx, "SELECT state FROM pve_runs WHERE id=?", runID).Scan(&data); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, store.NotFound
		}
		return nil, err
	}
	var state run.State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return &state, nil
}
func baseReward(state *run.State, qualified bool) int64 {
	if !qualified {
		return 0
	}
	switch state.EndReason {
	case "success":
		return state.Rules.SuccessReward
	case "timeout", "team_wipe":
		return state.Rules.FailureReward
	default:
		return state.Rules.AbortReward
	}
}
func settleTask(ctx context.Context, transaction *sql.Tx, state *run.State, member *run.Participant) error {
	definition, exists := state.Rules.Task(member.TaskKey)
	if !exists {
		return store.Invalid
	}
	complete := true
	for _, objective := range definition.Objectives {
		complete = complete && member.TaskProgress[objective.Key] >= objective.Required
	}
	if definition.RequiresSuccess {
		complete = complete && state.EndReason == "success" && member.Status != "left"
	}
	if definition.NoDeaths {
		complete = complete && member.Deaths == 0 && member.Status != "left"
	}
	member.TaskCompleted = complete
	if complete {
		period := member.TaskPeriod
		if period == "" {
			period = "once"
		}
		var prior int
		if err := transaction.QueryRowContext(ctx, "SELECT COUNT(*) FROM pve_task_completions WHERE player_id=? AND task_key=? AND task_version=? AND period_id=?", member.PlayerID, member.TaskKey, definition.LogicalVersion(state.Rules), period).Scan(&prior); err != nil {
			return err
		}
		if prior == 0 {
			if _, err := transaction.ExecContext(ctx, "INSERT INTO pve_task_completions(player_id,task_key,task_version,period_id,run_id,attempt_id) VALUES(?,?,?,?,?,?)", member.PlayerID, member.TaskKey, definition.LogicalVersion(state.Rules), period, state.ID, member.TaskAttemptID); err != nil {
				return err
			}
			if err := grant(ctx, transaction, state.ID, member.PlayerID, "task:"+member.TaskKey, definition.Reward); err != nil {
				return err
			}
		}
		if err := task.SaveProgress(ctx, transaction, member.PlayerID, definition, state.Rules, period, member.TaskProgress, true); err != nil {
			return err
		}
		if definition.Next != "" {
			next, _ := state.Rules.Task(definition.Next)
			if _, err := transaction.ExecContext(ctx, "INSERT INTO pve_player_tasks(player_id,task_key,version,progress,completed,unlocked) VALUES(?,?,?,'{}',0,1) ON DUPLICATE KEY UPDATE unlocked=1", member.PlayerID, definition.Next, next.LogicalVersion(state.Rules)); err != nil {
				return err
			}
		}
		if prior == 0 {
			if err := store.Notify(ctx, transaction, []int64{member.PlayerID}, "v2.task.completed", map[string]any{"run_id": state.ID, "task_key": member.TaskKey, "task_version": definition.LogicalVersion(state.Rules), "period_id": period, "reward": definition.Reward, "next": definition.Next}); err != nil {
				return err
			}
		}
	}
	status := "closed"
	if complete {
		status = "completed"
	}
	_, err := transaction.ExecContext(ctx, "UPDATE pve_player_task_attempts SET status=? WHERE id=?", status, member.TaskAttemptID)
	return err
}
func grant(ctx context.Context, transaction *sql.Tx, runID string, player int64, source string, amount int64) error {
	if amount == 0 {
		return nil
	}
	if amount < 0 {
		return store.Invalid
	}
	var balance int64
	if err := transaction.QueryRowContext(ctx, "SELECT soft_currency FROM player_assets WHERE player_id=? FOR UPDATE", player).Scan(&balance); err != nil {
		return err
	}
	if balance < 0 || amount > math.MaxInt64-balance {
		return store.Invalid
	}
	var existing int64
	err := transaction.QueryRowContext(ctx, "SELECT id FROM pve_reward_grants WHERE run_id=? AND player_id=? AND reward_source=?", runID, player, source).Scan(&existing)
	if err == nil {
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	result, err := transaction.ExecContext(ctx, "INSERT INTO pve_reward_grants(run_id,player_id,reward_source,asset_type,amount,status) VALUES(?,?,?,'soft_currency',?,'granted')", runID, player, source, amount)
	if err != nil {
		return err
	}
	grantID, err := result.LastInsertId()
	if err != nil {
		return err
	}
	if _, err := transaction.ExecContext(ctx, "UPDATE player_assets SET soft_currency=?,updated_at=UTC_TIMESTAMP(3) WHERE player_id=?", balance+amount, player); err != nil {
		return err
	}
	_, err = transaction.ExecContext(ctx, "INSERT INTO asset_ledger(player_id,pve_grant_id,pve_run_id,asset_type,delta,balance_before,balance_after,reason) VALUES(?,?,?,'soft_currency',?,?,?,'pve_reward')", player, grantID, runID, amount, balance, balance+amount)
	return err
}
