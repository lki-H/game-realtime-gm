package cache

import (
	"context"
	"errors"
	"time"

	"game-realtime-gm/backend/internal/config"

	"github.com/redis/go-redis/v9"
)

func NewRedisClient(ctx context.Context, cfg config.RedisConfig) (*redis.Client, error) {
	if cfg.PoolSize <= 0 || cfg.MaxActiveConns < cfg.PoolSize || cfg.PoolTimeoutMS <= 0 {
		return nil, errors.New("invalid Redis connection budget")
	}
	client := redis.NewClient(&redis.Options{
		PoolSize:       cfg.PoolSize,
		MaxActiveConns: cfg.MaxActiveConns,
		PoolTimeout:    time.Duration(cfg.PoolTimeoutMS) * time.Millisecond,
		Addr:           cfg.Addr,
		Password:       cfg.Password,
		DB:             cfg.DB,
	})

	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, err
	}

	return client, nil
}
