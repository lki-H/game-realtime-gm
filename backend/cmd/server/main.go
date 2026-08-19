package main

import (
	"context"
	"log"
	"net/http"

	"game-realtime-gm/backend/internal/cache"
	"game-realtime-gm/backend/internal/config"
	"game-realtime-gm/backend/internal/database"
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

	r := router.New(db, redisClient, cfg)

	srv := &http.Server{
		Addr:    ":" + cfg.AppPort,
		Handler: r,
	}

	log.Println("server listening on :" + cfg.AppPort)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
