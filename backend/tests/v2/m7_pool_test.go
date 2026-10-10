package v2_test

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"game-realtime-gm/backend/internal/cache"
	"game-realtime-gm/backend/internal/config"
	"game-realtime-gm/backend/internal/database"
	"github.com/redis/go-redis/v9"
)

func TestM7ConnectionBudgetsBoundConcurrentDependencyCalls(t *testing.T) {
	if os.Getenv("PVE_INTEGRATION") != "1" {
		t.Skip("set PVE_INTEGRATION=1 for isolated connection budgets")
	}
	password := os.Getenv("PVE_TEST_PASSWORD")
	if password == "" {
		t.Fatal("temporary test password required")
	}
	configuration := config.Load()
	configuration.Database = config.DatabaseConfig{Host: "127.0.0.1", Port: "23306", Name: "game_realtime_v2_test", User: "pve_test", Password: password, UTC: true, MaxOpenConns: 2, MaxIdleConns: 1}
	configuration.Redis = config.RedisConfig{Addr: "127.0.0.1:26379", DB: 14, PoolSize: 2, MaxActiveConns: 2, PoolTimeoutMS: 1000}
	if os.Getenv("PVE_TEST_NETWORK") == "compose" {
		configuration.Database.Host, configuration.Database.Port = "gm-v2-test-mysql-1", "3306"
		configuration.Redis.Addr = "gm-v2-test-redis-1:6379"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	db, err := database.NewMySQLDB(ctx, configuration.Database)
	if err != nil {
		t.Fatal("isolated budget database connection failed")
	}
	defer db.Close()
	client, err := cache.NewRedisClient(ctx, configuration.Redis)
	if err != nil {
		t.Fatal("isolated budget Redis connection failed")
	}
	defer client.Close()
	gate := make(chan struct{})
	var wait sync.WaitGroup
	failures := make(chan error, 24)
	for index := 0; index < 12; index++ {
		wait.Add(2)
		go func() {
			defer wait.Done()
			<-gate
			var result int
			if err := db.QueryRowContext(ctx, "SELECT SLEEP(0.05)").Scan(&result); err != nil {
				failures <- err
			}
		}()
		go func() {
			defer wait.Done()
			<-gate
			if _, err := client.Do(ctx, "BLPOP", "v2:m7:missing-budget-queue", "0.05").Result(); !errors.Is(err, redis.Nil) {
				failures <- err
			}
		}()
	}
	close(gate)
	wait.Wait()
	close(failures)
	for failure := range failures {
		t.Fatalf("bounded dependency call failed: %T", failure)
	}
	dbStats, redisStats := db.Stats(), client.PoolStats()
	if dbStats.MaxOpenConnections != 2 || dbStats.OpenConnections > 2 || dbStats.WaitCount == 0 {
		t.Fatal("MySQL budget did not bound concurrent calls")
	}
	if redisStats.TotalConns > 2 || redisStats.WaitCount == 0 {
		t.Fatal("Redis hard budget did not bound concurrent calls")
	}
	t.Logf("12 concurrent calls each: mysql_max=%d mysql_waits=%d redis_connections=%d redis_waits=%d", dbStats.MaxOpenConnections, dbStats.WaitCount, redisStats.TotalConns, redisStats.WaitCount)
}

func TestM7InvalidConnectionBudgetsFailBeforeDial(t *testing.T) {
	for _, configuration := range []config.DatabaseConfig{{MaxOpenConns: 0}, {MaxOpenConns: 2, MaxIdleConns: 3}, {MaxOpenConns: 2, MaxIdleConns: -1}} {
		if client, err := database.NewMySQLDB(context.Background(), configuration); err == nil || client != nil {
			t.Fatal("invalid MySQL budget accepted")
		}
	}
	for _, configuration := range []config.RedisConfig{{PoolSize: 0}, {PoolSize: 3, MaxActiveConns: 2, PoolTimeoutMS: 100}, {PoolSize: 2, MaxActiveConns: 2, PoolTimeoutMS: 0}} {
		if client, err := cache.NewRedisClient(context.Background(), configuration); err == nil || client != nil {
			t.Fatal("invalid Redis budget accepted")
		}
	}
}
