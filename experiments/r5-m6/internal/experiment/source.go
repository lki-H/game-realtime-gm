package experiment

import (
	"context"
	"database/sql"
	"errors"
	"time"

	reportingv1 "game-realtime-gm/experiments/r5-m6/gen/reporting/v1"
	"github.com/go-sql-driver/mysql"
)

func OpenDatabase(cfg DatabaseConfig) (*sql.DB, error) {
	driver := mysql.NewConfig()
	driver.User = cfg.User
	driver.Passwd = cfg.Password
	driver.Net = "tcp"
	driver.Addr = cfg.Address
	driver.DBName = cfg.Name
	driver.ParseTime = true
	driver.Loc = time.UTC
	driver.Timeout = 3 * time.Second
	driver.ReadTimeout = 5 * time.Second
	driver.WriteTimeout = 5 * time.Second
	driver.Params = map[string]string{"time_zone": "'+00:00'"}
	db, err := sql.Open("mysql", driver.FormatDSN())
	if err != nil {
		return nil, errors.New("database configuration rejected")
	}
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(4)
	db.SetConnMaxLifetime(time.Minute)
	return db, nil
}

type Reader interface {
	Leaderboard(context.Context, uint32) (*reportingv1.GetLeaderboardResponse, error)
	Result(context.Context, string) (*reportingv1.GetRunResultResponse, error)
}
type Source struct{ DB *sql.DB }

func (source Source) Leaderboard(ctx context.Context, limit uint32) (*reportingv1.GetLeaderboardResponse, error) {
	rows, err := source.DB.QueryContext(ctx, "SELECT player_id,score FROM r5_leaderboard ORDER BY score DESC,player_id ASC LIMIT ?", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := &reportingv1.GetLeaderboardResponse{}
	for rows.Next() {
		entry := &reportingv1.LeaderboardEntry{Rank: uint32(len(result.Entries) + 1)}
		if err := rows.Scan(&entry.PlayerId, &entry.Score); err != nil {
			return nil, err
		}
		result.Entries = append(result.Entries, entry)
	}
	return result, rows.Err()
}
func (source Source) Result(ctx context.Context, runID string) (*reportingv1.GetRunResultResponse, error) {
	result := &reportingv1.GetRunResultResponse{RunId: runID}
	if err := source.DB.QueryRowContext(ctx, "SELECT operation_name,difficulty,end_reason FROM r5_runs WHERE id=?", runID).Scan(&result.Operation, &result.Difficulty, &result.EndReason); err != nil {
		return nil, err
	}
	rows, err := source.DB.QueryContext(ctx, "SELECT player_id,contribution_qualified,task_completed,reward,settled_at FROM r5_results WHERE run_id=? ORDER BY player_id", runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		member := &reportingv1.ParticipantResult{}
		var settled time.Time
		if err := rows.Scan(&member.PlayerId, &member.ContributionQualified, &member.TaskCompleted, &member.Reward, &settled); err != nil {
			return nil, err
		}
		member.SettledAt = settled.UTC().Format(time.RFC3339Nano)
		result.Participants = append(result.Participants, member)
	}
	if len(result.Participants) == 0 || len(result.Participants) > 4 {
		return nil, errors.New("invalid settled participant count")
	}
	return result, rows.Err()
}
