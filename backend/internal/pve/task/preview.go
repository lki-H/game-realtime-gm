package task

import (
	"context"
	"database/sql"
	"encoding/json"
	"sort"
	"time"

	"game-realtime-gm/backend/internal/pve/store"
)

func Operations(ctx context.Context, db *sql.DB) ([]map[string]any, error) {
	rows, err := db.QueryContext(ctx, "SELECT rule_key,version,status,content FROM pve_rule_versions ORDER BY rule_key,version LIMIT 20")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []map[string]any{}
	for rows.Next() {
		var key, version, status string
		var content []byte
		if err := rows.Scan(&key, &version, &status, &content); err != nil {
			return nil, err
		}
		var rules Rules
		if err := json.Unmarshal(content, &rules); err != nil {
			return nil, err
		}
		result = append(result, map[string]any{"operation": key, "difficulty": rules.Difficulty, "rule_version": version, "status": status, "title": "训练场协同作战", "duration_seconds": rules.Duration, "reinforcements": rules.Reinforcements, "success_reward": rules.SuccessReward, "failure_reward": rules.FailureReward, "objectives": rules.Objectives, "tags": []string{"newcomer", "tasks", "experienced"}})
	}
	return result, rows.Err()
}
func Preview(ctx context.Context, db *sql.DB, player int64, version, key string) (map[string]any, error) {
	transaction, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer transaction.Rollback()
	var content []byte
	var status string
	if err := transaction.QueryRowContext(ctx, "SELECT content,status FROM pve_rule_versions WHERE rule_key='training_ground' AND version=?", version).Scan(&content, &status); err != nil {
		return nil, err
	}
	var rules Rules
	if err := json.Unmarshal(content, &rules); err != nil {
		return nil, err
	}
	value := map[string]any{"operation": rules.Operation, "difficulty": rules.Difficulty, "rule_version": rules.Version, "admission_allowed": status == "published", "personal_task_optional": true, "compatibility": "none", "base_reward_affected": false, "shared_objectives": rules.Objectives}
	if key != "" {
		definition, exists := rules.Task(key)
		if !exists {
			return nil, store.Invalid
		}
		compatibility := "compatible"
		reason := "selected_operation_matches"
		if definition.Operation != rules.Operation {
			compatibility = "incompatible"
			reason = "requires_other_operation"
		}
		progress, err := LoadProgress(ctx, transaction, player, definition, rules, definition.Period(time.Now().UTC()))
		if err != nil {
			return nil, err
		}
		value["compatibility"] = compatibility
		value["reason"] = reason
		value["task_key"] = key
		value["task_version"] = definition.LogicalVersion(rules)
		value["progress"] = progress
		value["confirmation"] = map[bool]string{true: "immediate", false: "run_end"}[definition.Immediate()]
		value["requires_success"] = definition.RequiresSuccess
		value["no_deaths"] = definition.NoDeaths
		value["reward"] = definition.Reward
		value["next"] = definition.Next
		objectives := []map[string]any{}
		for _, objective := range definition.Objectives {
			opportunity := objective.Opportunity
			if opportunity == "" {
				opportunity = "guaranteed"
			}
			if compatibility == "incompatible" {
				opportunity = "unavailable"
			}
			objectives = append(objectives, map[string]any{"objective": objective, "opportunity": opportunity, "active": Active(definition.Objectives, progress.Counts)[objective.Key], "can_progress": compatibility == "compatible", "persistence": map[bool]string{true: "across_runs", false: "same_run"}[objective.Persistent]})
		}
		value["objectives"] = objectives
	}
	return value, transaction.Commit()
}
func Recommendations(ctx context.Context, db *sql.DB, player int64, key string) ([]map[string]any, error) {
	operations, err := Operations(ctx, db)
	if err != nil {
		return nil, err
	}
	result := []map[string]any{}
	for _, operation := range operations {
		if operation["status"] != "published" {
			continue
		}
		preview, err := Preview(ctx, db, player, operation["rule_version"].(string), key)
		if err == store.Invalid {
			continue
		}
		if err != nil {
			return nil, err
		}
		if preview["compatibility"] == "incompatible" {
			continue
		}
		operation["reason"] = "fixed_operation_compatible"
		result = append(result, operation)
	}
	sort.SliceStable(result, func(first, second int) bool {
		return result[first]["rule_version"].(string) > result[second]["rule_version"].(string)
	})
	return result, nil
}
