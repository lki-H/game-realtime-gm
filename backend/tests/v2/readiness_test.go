package v2_test

import (
	"context"
	"database/sql"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"game-realtime-gm/backend/internal/config"
	"game-realtime-gm/backend/internal/handler"
	"game-realtime-gm/backend/internal/router"
	"github.com/gin-gonic/gin"
	"github.com/go-sql-driver/mysql"
	"github.com/redis/go-redis/v9"
)

func TestReadinessDependencyFailureAndShutdown(t *testing.T) {
	if os.Getenv("PVE_INTEGRATION") != "1" {
		t.Skip("set PVE_INTEGRATION=1 for isolated MySQL/Redis readiness")
	}
	password := os.Getenv("PVE_TEST_PASSWORD")
	if password == "" {
		t.Fatal("temporary test password required")
	}
	dbAddress, cacheAddress := "127.0.0.1:23306", "127.0.0.1:26379"
	if os.Getenv("PVE_TEST_NETWORK") == "compose" {
		dbAddress, cacheAddress = "gm-v2-test-mysql-1:3306", "gm-v2-test-redis-1:6379"
	}
	connection := mysql.Config{User: "pve_test", Passwd: password, Net: "tcp", Addr: dbAddress, DBName: "game_realtime_v2_test"}
	db, err := sql.Open("mysql", connection.FormatDSN())
	if err != nil {
		t.Fatal("isolated readiness database open failed")
	}
	defer db.Close()
	cache := redis.NewClient(&redis.Options{Addr: cacheAddress, DB: 14})
	defer cache.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	check := func(lifecycle context.Context, db *sql.DB, cache *redis.Client, expected int) {
		t.Helper()
		engine := gin.New()
		engine.GET("/ready", handler.Readiness(lifecycle, db, cache))
		recorder := httptest.NewRecorder()
		started := time.Now()
		engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/ready", nil))
		if recorder.Code != expected || time.Since(started) > 2*time.Second {
			t.Fatalf("readiness status=%d elapsed=%s", recorder.Code, time.Since(started))
		}
		if expected == http.StatusServiceUnavailable && !strings.Contains(recorder.Body.String(), "service not ready") {
			t.Fatal("dependency details must not be exposed")
		}
	}
	check(ctx, db, cache, http.StatusOK)
	downCache := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", MaxRetries: -1, DialTimeout: 100 * time.Millisecond})
	defer downCache.Close()
	check(ctx, db, downCache, http.StatusServiceUnavailable)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	done := make(chan struct{})
	defer close(done)
	go func() {
		for {
			connection, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer connection.Close()
				<-done
			}()
		}
	}()
	stalledCache := redis.NewClient(&redis.Options{Addr: listener.Addr().String(), MaxRetries: -1, ReadTimeout: 5 * time.Second})
	defer stalledCache.Close()
	check(ctx, db, stalledCache, http.StatusServiceUnavailable)
	closedDB, err := sql.Open("mysql", "")
	if err != nil {
		t.Fatal(err)
	}
	_ = closedDB.Close()
	check(ctx, closedDB, cache, http.StatusServiceUnavailable)
	lifecycle, stop := context.WithCancel(context.Background())
	stop()
	check(lifecycle, db, cache, http.StatusServiceUnavailable)
	recorder := httptest.NewRecorder()
	configuration := config.Load()
	configuration.GameplayMode = "v2"
	router.New(ctx, db, cache, configuration).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if recorder.Code != http.StatusOK {
		t.Fatal("readiness route is not wired to the real router")
	}
}
