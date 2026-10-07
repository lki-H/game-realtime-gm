package projection

import (
	"context"
	"database/sql"
	"strconv"

	"github.com/redis/go-redis/v9"
)

type Service struct {
	DB    *sql.DB
	Redis *redis.Client
}

func (s *Service) Rebuild(ctx context.Context) error {
	if s.Redis == nil {
		return nil
	}
	rows, err := s.DB.QueryContext(ctx, "SELECT id,queue_priority_since FROM pve_match_tickets WHERE status='queued'")
	if err != nil {
		return err
	}
	members := []redis.Z{}
	for rows.Next() {
		var id string
		var stamp sql.NullTime
		if err := rows.Scan(&id, &stamp); err != nil {
			rows.Close()
			return err
		}
		members = append(members, redis.Z{Member: id, Score: float64(stamp.Time.UnixMilli())})
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	stage := "v2:queue:training_ground:staging"
	if err := s.Redis.Del(ctx, stage).Err(); err != nil {
		return err
	}
	if len(members) > 0 {
		if err := s.Redis.ZAdd(ctx, stage, members...).Err(); err != nil {
			return err
		}
		if err := s.Redis.Rename(ctx, stage, "v2:queue:training_ground").Err(); err != nil {
			return err
		}
	} else {
		if err := s.Redis.Del(ctx, "v2:queue:training_ground").Err(); err != nil {
			return err
		}
	}
	rows, err = s.DB.QueryContext(ctx, "SELECT player_id,SUM(amount) FROM pve_reward_grants WHERE status='granted' GROUP BY player_id")
	if err != nil {
		return err
	}
	defer rows.Close()
	scores := []redis.Z{}
	for rows.Next() {
		var player, score int64
		if err := rows.Scan(&player, &score); err != nil {
			return err
		}
		scores = append(scores, redis.Z{Member: strconv.FormatInt(player, 10), Score: float64(score)})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := s.Redis.Del(ctx, "v2:rewards").Err(); err != nil {
		return err
	}
	if len(scores) > 0 {
		return s.Redis.ZAdd(ctx, "v2:rewards", scores...).Err()
	}
	return nil
}
