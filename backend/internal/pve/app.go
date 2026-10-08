package pve

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"game-realtime-gm/backend/internal/pve/control"
	"game-realtime-gm/backend/internal/pve/matchmaking"
	"game-realtime-gm/backend/internal/pve/party"
	"game-realtime-gm/backend/internal/pve/projection"
	"game-realtime-gm/backend/internal/pve/retention"
	"game-realtime-gm/backend/internal/pve/run"
	"game-realtime-gm/backend/internal/pve/settlement"
	"game-realtime-gm/backend/internal/pve/social"
	"game-realtime-gm/backend/internal/pve/store"
	"game-realtime-gm/backend/internal/pve/task"
	"game-realtime-gm/backend/internal/pve/worker"
	"github.com/redis/go-redis/v9"
)

type App struct {
	DB         *sql.DB
	Runs       *run.Service
	Party      *party.Service
	Social     *social.Service
	Match      *matchmaking.Service
	Settlement *settlement.Service
	Projection *projection.Service
	Worker     *worker.Worker
	Retention  *retention.Service
}

func New(db *sql.DB, cache *redis.Client, rules task.Rules) *App {
	runs := run.NewService(db, run.Config{Rules: rules})
	app := &App{DB: db, Runs: runs, Party: party.NewService(db), Social: social.NewService(db), Match: matchmaking.NewService(db, runs), Settlement: settlement.NewService(db), Projection: &projection.Service{DB: db, Redis: cache}}
	app.Worker = &worker.Worker{DB: db, Settle: app.Settlement.Settle, CompleteTask: app.Settlement.CompleteTask, Tick: app.Tick}
	app.Retention = &retention.Service{DB: db, Days: 90, BatchSize: 100}
	return app
}
func (app *App) Command(ctx context.Context, player int64, operation, kind string, data json.RawMessage) (json.RawMessage, error) {
	if !json.Valid(data) || len(data) > 16384 || !strings.HasPrefix(kind, "v2.") {
		return nil, store.Invalid
	}
	return store.Command(ctx, app.DB, player, operation, kind, data, func(transaction *sql.Tx) (any, error) {
		if strings.HasPrefix(kind, "v2.social.") {
			var input social.Action
			if err := json.Unmarshal(data, &input); err != nil {
				return nil, store.Invalid
			}
			return app.Social.Execute(ctx, transaction, player, strings.TrimPrefix(kind, "v2.social."), input)
		}
		if strings.HasPrefix(kind, "v2.party.") || strings.HasPrefix(kind, "v2.recruitment.") {
			var input party.Request
			if err := json.Unmarshal(data, &input); err != nil {
				return nil, store.Invalid
			}
			if (kind == "v2.party.selection" || kind == "v2.recruitment.selection") && input.TaskKey != "" {
				group, err := party.Load(ctx, transaction, input.PartyID)
				if err != nil {
					return nil, err
				}
				version := group.Plan.RuleVersion
				if version == "" {
					version = app.Runs.Rules.Version
				}
				rules, err := task.RuleTx(ctx, transaction, group.Plan.Operation, version)
				if err != nil {
					return nil, err
				}
				definition, exists := rules.Task(input.TaskKey)
				if !exists || input.TaskVersion != definition.LogicalVersion(rules) {
					return nil, store.Invalid
				}
			}
			if kind == "v2.party.regroup_propose" || kind == "v2.party.regroup_respond" {
				return app.Party.Regroup(ctx, transaction, player, strings.TrimPrefix(kind, "v2.party.regroup_"), input)
			}
			if strings.HasPrefix(kind, "v2.recruitment.") {
				return app.Party.Recruit(ctx, transaction, player, strings.TrimPrefix(kind, "v2.recruitment."), input)
			}
			result, err := app.Party.Execute(ctx, transaction, player, strings.TrimPrefix(kind, "v2.party."), input)
			if err != nil {
				return nil, err
			}
			if group, ok := result.(*party.Party); ok {
				players := []int64{}
				for _, member := range group.Members {
					players = append(players, member.PlayerID)
				}
				if err := store.Notify(ctx, transaction, players, "v2.party.changed", map[string]any{"party_id": group.ID, "roster_version": group.RosterVersion, "plan_version": group.PlanVersion}); err != nil {
					return nil, err
				}
				result = group.ForPlayer(player)
			}
			return result, nil
		}
		if strings.HasPrefix(kind, "v2.match.") {
			var input matchmaking.Request
			if err := json.Unmarshal(data, &input); err != nil {
				return nil, store.Invalid
			}
			return app.Match.Execute(ctx, transaction, player, strings.TrimPrefix(kind, "v2.match."), input)
		}
		if kind == "v2.task.pause" {
			var input struct {
				TaskKey     string `json:"task_key"`
				TaskVersion string `json:"task_version"`
			}
			if json.Unmarshal(data, &input) != nil || input.TaskKey == "" || input.TaskVersion == "" || len(input.TaskKey) > 59 || len(input.TaskVersion) > 64 {
				return nil, store.Invalid
			}
			if err := store.RequireIdle(ctx, transaction, player); err != nil {
				return nil, err
			}
			products, err := task.LoadProducts()
			if err != nil {
				return nil, err
			}
			definition, exists := products.Task(input.TaskKey)
			if !exists || input.TaskVersion != definition.LogicalVersion(products) {
				return nil, store.Invalid
			}
			if _, err := transaction.ExecContext(ctx, "UPDATE pve_party_members SET task_selection=JSON_OBJECT(),selection_version=selection_version+1,ready=0 WHERE player_id=? AND status='active' AND JSON_UNQUOTE(JSON_EXTRACT(task_selection,'$.task_key'))=?", player, input.TaskKey); err != nil {
				return nil, err
			}
			if _, err := transaction.ExecContext(ctx, "INSERT INTO pve_task_pauses(player_id,task_key,task_version) VALUES(?,?,?) ON DUPLICATE KEY UPDATE paused=1", player, input.TaskKey, input.TaskVersion); err != nil {
				return nil, err
			}
			if _, err := transaction.ExecContext(ctx, "UPDATE pve_recruitment_preferences SET task_selection=JSON_OBJECT(),selection_version=selection_version+1,ready=0 WHERE player_id=? AND JSON_UNQUOTE(JSON_EXTRACT(task_selection,'$.task_key'))=?", player, input.TaskKey); err != nil {
				return nil, err
			}
			return map[string]any{"paused": true, "progress_preserved": true}, nil
		}
		if kind == "v2.run.leave" {
			var input struct {
				RunID string `json:"run_id"`
			}
			if err := json.Unmarshal(data, &input); err != nil {
				return nil, store.Invalid
			}
			state, err := run.Load(ctx, transaction, input.RunID)
			if err != nil {
				return nil, err
			}
			if err := run.LeaveTx(ctx, transaction, state, player, app.Runs.Now().UTC()); err != nil {
				return nil, err
			}
			if err := run.Save(ctx, transaction, state); err != nil {
				return nil, err
			}
			return map[string]any{"run_id": state.ID, "left": true}, nil
		}
		return nil, store.NotFound
	})
}
func (app *App) Tick(ctx context.Context) error {
	if err := app.Party.ExpireRecruitment(ctx); err != nil {
		return err
	}
	if err := store.Transaction(ctx, app.DB, func(transaction *sql.Tx) error {
		if _, err := transaction.ExecContext(ctx, "UPDATE pve_regroup_proposals SET status='expired' WHERE status='pending' AND expires_at<=UTC_TIMESTAMP(3)"); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return err
	}
	if err := app.Match.Expire(ctx); err != nil {
		return err
	}
	if err := app.Match.Match(ctx); err != nil {
		return err
	}
	rows, err := app.DB.QueryContext(ctx, "SELECT id FROM pve_runs WHERE status IN ('running','loading') ORDER BY updated_at,id LIMIT 100")
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
		if err := app.Match.LoadingExpired(ctx, id); err != nil {
			return err
		}
		if err := app.Runs.Tick(ctx, id); err != nil {
			return err
		}
	}
	if app.Social.RetentionDays > 0 {
		if _, err := app.DB.ExecContext(ctx, "DELETE FROM pve_social_messages WHERE created_at<? LIMIT 1000", time.Now().UTC().Add(-time.Duration(app.Social.RetentionDays)*24*time.Hour)); err != nil {
			return err
		}
	}
	if err := app.Retention.Tick(ctx); err != nil {
		return err
	}
	return app.Projection.Rebuild(ctx)
}
func (app *App) Publish(ctx context.Context) error {
	if err := control.Initialize(ctx, app.DB); err != nil {
		return err
	}
	if err := app.publishRules(ctx, app.Runs.Rules); err != nil {
		return err
	}
	products, err := task.LoadProducts()
	if err != nil {
		return err
	}
	return app.publishRules(ctx, products)
}
func (app *App) publishRules(ctx context.Context, rules task.Rules) error {
	return store.Transaction(ctx, app.DB, func(transaction *sql.Tx) error {
		hash := store.Hash(store.JSON(rules))
		var prior string
		err := transaction.QueryRowContext(ctx, "SELECT content_hash FROM pve_rule_versions WHERE rule_key=? AND version=?", rules.Operation, rules.Version).Scan(&prior)
		if err == nil {
			if prior != hash {
				return store.Conflict
			}
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		_, err = transaction.ExecContext(ctx, "INSERT INTO pve_rule_versions(rule_key,version,content_hash,content) VALUES(?,?,?,?)", rules.Operation, rules.Version, hash, store.JSON(rules))
		return err
	})
}
func (app *App) Recover(ctx context.Context) error {
	if err := app.Publish(ctx); err != nil {
		return err
	}
	if _, err := app.DB.ExecContext(ctx, "UPDATE pve_party_members SET ready=0 WHERE status='active'"); err != nil {
		return err
	}
	if _, err := app.DB.ExecContext(ctx, "UPDATE pve_recruitment_preferences SET ready=0"); err != nil {
		return err
	}
	if _, err := app.DB.ExecContext(ctx, "UPDATE pve_regroup_proposals SET status='aborted' WHERE status='pending'"); err != nil {
		return err
	}
	if err := store.Transaction(ctx, app.DB, func(transaction *sql.Tx) error {
		if err := matchmaking.Lane(ctx, transaction); err != nil {
			return err
		}
		if _, err := transaction.ExecContext(ctx, "DELETE FROM pve_player_activity_locks WHERE activity_type='ticket'"); err != nil {
			return err
		}
		if _, err := transaction.ExecContext(ctx, "UPDATE pve_match_tickets SET status='canceled' WHERE status IN ('queued','proposed')"); err != nil {
			return err
		}
		if _, err := transaction.ExecContext(ctx, "UPDATE pve_match_proposals SET status='aborted' WHERE status='pending'"); err != nil {
			return err
		}
		_, err := transaction.ExecContext(ctx, "UPDATE pve_parties SET status='open' WHERE status='queued'")
		return err
	}); err != nil {
		return err
	}
	rows, err := app.DB.QueryContext(ctx, "SELECT id FROM pve_runs WHERE status IN ('running','loading')")
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
		if err := store.Transaction(ctx, app.DB, func(transaction *sql.Tx) error {
			state, err := run.Load(ctx, transaction, id)
			if err != nil {
				return err
			}
			now := time.Now().UTC()
			state.Status = "ending"
			state.EndReason = "server_restart"
			state.EndedAt = &now
			state.FinalSequence = state.AppliedSequence
			if err := run.Save(ctx, transaction, state); err != nil {
				return err
			}
			return run.QueueSettlement(ctx, transaction, state)
		}); err != nil {
			return err
		}
	}
	return nil
}
