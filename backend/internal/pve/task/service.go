package task

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"game-realtime-gm/backend/internal/pve/store"
)

type Progress struct {
	Counts    map[string]int64 `json:"counts"`
	Completed bool             `json:"completed"`
	Unlocked  bool             `json:"unlocked"`
	PeriodID  string           `json:"period_id"`
	Paused    bool             `json:"paused"`
}

func LoadProgress(ctx context.Context, transaction *sql.Tx, player int64, definition Definition, rules Rules, period string) (Progress, error) {
	result := Progress{Counts: map[string]int64{}, Unlocked: !definition.Locked, PeriodID: period}
	if err := transaction.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM pve_task_pauses WHERE player_id=? AND task_key=? AND task_version=? AND paused=1)", player, definition.Key, definition.LogicalVersion(rules)).Scan(&result.Paused); err != nil {
		return result, err
	}
	var content []byte
	var err error
	if definition.RepeatSeconds > 0 {
		err = transaction.QueryRowContext(ctx, "SELECT progress,completed FROM pve_task_periods WHERE player_id=? AND task_key=? AND task_version=? AND period_id=?", player, definition.Key, definition.LogicalVersion(rules), period).Scan(&content, &result.Completed)
	} else {
		err = transaction.QueryRowContext(ctx, "SELECT progress,completed,unlocked FROM pve_player_tasks WHERE player_id=? AND task_key=? AND version=?", player, definition.Key, definition.LogicalVersion(rules)).Scan(&content, &result.Completed, &result.Unlocked)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	err = json.Unmarshal(content, &result.Counts)
	return result, err
}
func SaveProgress(ctx context.Context, transaction *sql.Tx, player int64, definition Definition, rules Rules, period string, counts map[string]int64, completed bool) error {
	if definition.RepeatSeconds > 0 {
		_, err := transaction.ExecContext(ctx, "INSERT INTO pve_task_periods(player_id,task_key,task_version,period_id,progress,completed) VALUES(?,?,?,?,?,?) ON DUPLICATE KEY UPDATE progress=VALUES(progress),completed=GREATEST(completed,VALUES(completed))", player, definition.Key, definition.LogicalVersion(rules), period, store.JSON(counts), completed)
		return err
	}
	_, err := transaction.ExecContext(ctx, "INSERT INTO pve_player_tasks(player_id,task_key,version,progress,completed,unlocked) VALUES(?,?,?,?,?,1) ON DUPLICATE KEY UPDATE progress=VALUES(progress),completed=GREATEST(completed,VALUES(completed))", player, definition.Key, definition.LogicalVersion(rules), store.JSON(counts), completed)
	return err
}
func RuleTx(ctx context.Context, transaction *sql.Tx, operation, version string) (Rules, error) {
	var content []byte
	var status, hash string
	if err := transaction.QueryRowContext(ctx, "SELECT content,status,content_hash FROM pve_rule_versions WHERE rule_key=? AND version=? FOR SHARE", operation, version).Scan(&content, &status, &hash); err != nil {
		return Rules{}, err
	}
	var rules Rules
	if err := json.Unmarshal(content, &rules); err != nil {
		return rules, err
	}
	if rules.Validate() != nil || store.Hash(store.JSON(rules)) != hash || status != "published" {
		return rules, store.Conflict
	}
	return rules, nil
}
func Catalogue(ctx context.Context, db *sql.DB, player int64, version string) ([]map[string]any, error) {
	transaction, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer transaction.Rollback()
	var content []byte
	if err := transaction.QueryRowContext(ctx, "SELECT content FROM pve_rule_versions WHERE rule_key='training_ground' AND version=?", version).Scan(&content); err != nil {
		return nil, err
	}
	var rules Rules
	if err := json.Unmarshal(content, &rules); err != nil {
		return nil, err
	}
	result := []map[string]any{}
	for _, definition := range rules.Tasks {
		progress, err := LoadProgress(ctx, transaction, player, definition, rules, definition.Period(time.Now().UTC()))
		if err != nil {
			return nil, err
		}
		result = append(result, map[string]any{"task": definition, "task_version": definition.LogicalVersion(rules), "progress": progress, "paused": progress.Paused, "confirmation": map[bool]string{true: "immediate", false: "run_end"}[definition.Immediate()]})
	}
	return result, transaction.Commit()
}
