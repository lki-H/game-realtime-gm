package main

import (
	"context"
	"errors"
	"log"
	"net/http"

	"game-realtime-gm/backend/internal/cache"
	"game-realtime-gm/backend/internal/config"
	"game-realtime-gm/backend/internal/database"
	"game-realtime-gm/backend/internal/diagnostics"
	"game-realtime-gm/backend/internal/router"
)

func main() {
	cfg := config.Load()
	ctx := context.Background()

	db, err := database.NewMySQLDB(ctx, cfg.Database)
	if err != nil {
		log.Fatal("connect database failed: ", err)
	}
	defer db.Close()
	log.Println("database connected")

	redisClient, err := cache.NewRedisClient(ctx, cfg.Redis)
	if err != nil {
		log.Fatal("connect redis failed: ", err)
	}
	defer redisClient.Close()
	log.Println("redis connected")

	if cfg.Pprof.Enabled {
		pprofServer := diagnostics.NewPprofServer(cfg.Pprof.Addr)
		go func() {
			log.Printf("pprof listening on http://%s/debug/pprof/", cfg.Pprof.Addr)
			if err := pprofServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Printf("pprof server stopped unexpectedly: %v", err)
			}
		}()
	}

	r := router.New(ctx, db, redisClient, cfg)

	srv := &http.Server{
		Addr:    ":" + cfg.AppPort,
		Handler: r,
	}

	log.Println("server listening on :" + cfg.AppPort)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
