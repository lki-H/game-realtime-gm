package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"game-realtime-gm/backend/internal/cache"
	"game-realtime-gm/backend/internal/config"
	"game-realtime-gm/backend/internal/database"
	"game-realtime-gm/backend/internal/pve/control"
)

func main() {
	apply := flag.Bool("apply", false, "remove only enumerated legacy matchmaking cache after drain; default is inspect")
	flag.Parse()
	configuration := config.Load()
	if configuration.GameplayMode != "v2" {
		log.Fatal("retirement tool requires v2 mode")
	}
	if *apply && os.Getenv("PVE_RETIRE_CONFIRM") != "I_UNDERSTAND_LEGACY_CACHE_REMOVAL" {
		log.Fatal("set PVE_RETIRE_CONFIRM=I_UNDERSTAND_LEGACY_CACHE_REMOVAL before -apply")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := database.NewMySQLDB(ctx, configuration.Database)
	if err != nil {
		log.Fatal("connect retirement database failed")
	}
	defer db.Close()
	owner, err := db.Conn(ctx)
	if err != nil {
		log.Fatal("claim database connection failed")
	}
	defer owner.Close()
	var acquired int
	if err := owner.QueryRowContext(ctx, "SELECT GET_LOCK(CONCAT('gm-gameplay:',DATABASE()),0)").Scan(&acquired); err != nil || acquired != 1 {
		log.Fatal("stop the drained gameplay service before offline inspection or cache cleanup")
	}
	defer func() {
		_, _ = owner.ExecContext(context.Background(), "SELECT RELEASE_LOCK(CONCAT('gm-gameplay:',DATABASE()))")
	}()
	snapshot, err := control.Observe(ctx, db)
	if err != nil {
		log.Fatal("retirement state unavailable; apply stage r4 first")
	}
	cacheClient, err := cache.NewRedisClient(ctx, configuration.Redis)
	if err != nil {
		log.Fatal("connect retirement cache failed")
	}
	defer cacheClient.Close()
	keys, err := control.LegacyCacheKeys(ctx, cacheClient)
	if err != nil {
		log.Fatal("legacy cache scan failed or exceeds 10000 keys")
	}
	var removed int64
	if *apply {
		if snapshot.AdmissionState != "closed" && !snapshot.ReadyToStop {
			log.Fatal("cache removal requires closed or fully drained admission")
		}
		if err := control.RequireLegacyIdle(ctx, db); err != nil || snapshot.PendingOutbox != 0 {
			log.Fatal("cache removal requires no V2 activity or pending work")
		}
		removed, err = control.RemoveLegacyCache(ctx, cacheClient, keys)
		if err != nil {
			log.Fatal("legacy cache removal incomplete; rerun inspect and reconcile")
		}
	}
	result := struct {
		Database string           `json:"database"`
		RedisDB  int              `json:"redis_db"`
		State    control.Snapshot `json:"state"`
		KeyCount int              `json:"legacy_key_count"`
		Removed  int64            `json:"removed"`
		Applied  bool             `json:"applied"`
	}{configuration.Database.Name, configuration.Redis.DB, snapshot, len(keys), removed, *apply}
	data, err := json.Marshal(result)
	if err != nil {
		log.Fatal("serialize retirement result failed")
	}
	fmt.Println(string(data))
}
