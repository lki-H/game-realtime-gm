package control

import (
	"context"
	"sort"

	"game-realtime-gm/backend/internal/pve/store"
	"github.com/redis/go-redis/v9"
)

func LegacyCacheKeys(ctx context.Context, cache *redis.Client) ([]string, error) {
	seen := map[string]bool{}
	for _, pattern := range []string{"matchmaking:queue:*", "matchmaking:ticket:*", "matchmaking:player:*", "matchmaking:timeouts"} {
		var cursor uint64
		for {
			keys, next, err := cache.Scan(ctx, cursor, pattern, 100).Result()
			if err != nil {
				return nil, err
			}
			for _, key := range keys {
				seen[key] = true
			}
			if len(seen) > 10000 {
				return nil, store.Conflict
			}
			cursor = next
			if cursor == 0 {
				break
			}
		}
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys, nil
}

func RemoveLegacyCache(ctx context.Context, cache *redis.Client, keys []string) (int64, error) {
	for _, key := range keys {
		allowed := key == "matchmaking:timeouts"
		for _, prefix := range []string{"matchmaking:queue:", "matchmaking:ticket:", "matchmaking:player:"} {
			allowed = allowed || len(key) > len(prefix) && key[:len(prefix)] == prefix
		}
		if !allowed {
			return 0, store.Invalid
		}
	}
	var removed int64
	for start := 0; start < len(keys); start += 100 {
		end := start + 100
		if end > len(keys) {
			end = len(keys)
		}
		count, err := cache.Unlink(ctx, keys[start:end]...).Result()
		if err != nil {
			return removed, err
		}
		removed += count
	}
	return removed, nil
}
