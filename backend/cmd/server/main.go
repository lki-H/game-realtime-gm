package main

import (
	"context"
	"errors"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"game-realtime-gm/backend/internal/cache"
	"game-realtime-gm/backend/internal/config"
	"game-realtime-gm/backend/internal/database"
	"game-realtime-gm/backend/internal/diagnostics"
	"game-realtime-gm/backend/internal/pve"
	"game-realtime-gm/backend/internal/pve/control"
	"game-realtime-gm/backend/internal/pve/task"
	"game-realtime-gm/backend/internal/router"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	cfg := config.Load()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if cfg.GameplayMode != "legacy" && cfg.GameplayMode != "v2" {
		log.Fatal("GAMEPLAY_MODE must be legacy or v2")
	}
	slog.Info("gameplay mode selected", "gameplay_mode", cfg.GameplayMode)

	db, err := database.NewMySQLDB(ctx, cfg.Database)
	if err != nil {
		log.Fatal("connect database failed: ", err)
	}
	defer db.Close()
	log.Println("database connected")
	owner, err := db.Conn(ctx)
	if err != nil {
		log.Fatal("claim single-instance ownership failed")
	}
	var owned int
	if err := owner.QueryRowContext(ctx, "SELECT GET_LOCK(CONCAT('gm-gameplay:',DATABASE()),0)").Scan(&owned); err != nil || owned != 1 {
		owner.Close()
		log.Fatal("database already has an active gameplay service")
	}
	defer owner.Close()
	if cfg.GameplayMode == "legacy" {
		if err := control.RequireLegacyIdle(ctx, db); err != nil {
			log.Fatal("legacy regression requires idle V2 facts: ", err)
		}
	}

	redisClient, err := cache.NewRedisClient(ctx, cfg.Redis)
	if err != nil {
		log.Fatal("connect redis failed: ", err)
	}
	defer redisClient.Close()
	log.Println("redis connected")
	var listenerShutdown sync.WaitGroup
	listenerShutdown.Add(1)
	go func() {
		defer listenerShutdown.Done()
		database.WatchGameplayOwner(ctx, owner, time.Second, func() {
			slog.Error("gameplay database ownership lost; stopping service")
			stop()
		})
	}()

	if cfg.Pprof.Enabled {
		if !diagnostics.LoopbackAddress(cfg.Pprof.Addr) {
			log.Fatal("pprof requires an explicit loopback listener")
		}
		pprofServer := diagnostics.NewPprofServer(cfg.Pprof.Addr)
		listenerShutdown.Add(1)
		go func() {
			defer listenerShutdown.Done()
			<-ctx.Done()
			shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = pprofServer.Shutdown(shutdown)
		}()
		go func() {
			log.Printf("pprof listening on http://%s/debug/pprof/", cfg.Pprof.Addr)
			if err := pprofServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Printf("pprof server stopped unexpectedly: %v", err)
			}
		}()
	}

	var app *pve.App
	if cfg.GameplayMode == "v2" {
		rules, err := task.Load(cfg.PVE.RulesPath)
		if err != nil {
			log.Fatal("load pve rules failed: ", err)
		}
		app = pve.New(db, redisClient, rules)
		if cfg.PVE.RegroupTTLSeconds <= 0 {
			log.Fatal("PVE_REGROUP_TTL_SECONDS must be positive")
		}
		app.Party.RegroupTTL = time.Duration(cfg.PVE.RegroupTTLSeconds) * time.Second
		if cfg.PVE.MaxProposalRounds <= 0 || cfg.HTTP.MaxBodyBytes <= 0 || cfg.HTTP.AuthRateLimit <= 0 || cfg.HTTP.WebSocketRateLimit <= 0 || cfg.HTTP.WebSocketCommandRateLimit <= 0 {
			log.Fatal("resource limits must be positive")
		}
		app.Match.MaxProposalRounds = cfg.PVE.MaxProposalRounds
		if cfg.PVE.MessageRetentionDays <= 0 {
			log.Fatal("PVE_MESSAGE_RETENTION_DAYS must be positive")
		}
		app.Social.RetentionDays = cfg.PVE.MessageRetentionDays
		if cfg.PVE.ArchiveDays <= 0 || cfg.PVE.ArchiveBatchSize <= 0 || cfg.PVE.ArchiveBatchSize > 1000 || cfg.HTTP.WebSocketMaxConnections <= 0 || cfg.HTTP.WebSocketMaxPerIP <= 0 {
			log.Fatal("invalid archive or connection limits")
		}
		app.Retention.Enabled = cfg.PVE.ArchiveEnabled
		app.Retention.Days = cfg.PVE.ArchiveDays
		app.Retention.BatchSize = cfg.PVE.ArchiveBatchSize
		if err := app.Recover(ctx); err != nil {
			log.Fatal("recover pve facts failed: ", err)
		}
	}
	var r http.Handler
	if app == nil {
		r = router.New(ctx, db, redisClient, cfg)
	} else {
		r = router.New(ctx, db, redisClient, cfg, app)
	}
	workerDone := make(chan struct{})
	if app != nil {
		go func() { defer close(workerDone); app.Worker.Run(ctx) }()
	} else {
		close(workerDone)
	}

	srv := &http.Server{
		Addr:              ":" + cfg.AppPort,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	if cfg.PVE.TestEventsEnabled {
		host, _, err := net.SplitHostPort(cfg.PVE.TestEventsAddr)
		if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() || app == nil || len(cfg.PVE.TestEventsToken) < 24 || cfg.PVE.TestEventsToken == cfg.JWTSecret {
			log.Fatal("test events require v2 mode, loopback address and at least 24 characters of independent token")
		}
		internalServer := &http.Server{
			Addr:              cfg.PVE.TestEventsAddr,
			Handler:           router.NewPVEInternalHandler(db, cfg, app),
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       10 * time.Second,
			WriteTimeout:      10 * time.Second,
			IdleTimeout:       30 * time.Second,
			MaxHeaderBytes:    1 << 20,
		}
		listenerShutdown.Add(1)
		go func() {
			log.Printf("pve test event server listening on http://%s", cfg.PVE.TestEventsAddr)
			if err := internalServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Printf("pve test event server stopped unexpectedly: %v", err)
				stop()
			}
		}()
		go func() {
			defer listenerShutdown.Done()
			<-ctx.Done()
			shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = internalServer.Shutdown(shutdown)
		}()
	}
	if cfg.PVE.MetricsEnabled {
		host, _, err := net.SplitHostPort(cfg.PVE.MetricsAddr)
		if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() || app == nil || len(cfg.PVE.MetricsToken) < 24 || cfg.PVE.MetricsToken == cfg.PVE.TestEventsToken || cfg.PVE.MetricsToken == cfg.JWTSecret {
			log.Fatal("metrics require v2, loopback address and independent service token")
		}
		metricsServer := &http.Server{Addr: cfg.PVE.MetricsAddr, Handler: router.NewPVEMetricsHandler(db, cfg.PVE.MetricsToken), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16384}
		listenerShutdown.Add(1)
		go func() {
			if err := metricsServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				slog.Error("pve metrics listener stopped", "error", err.Error())
				stop()
			}
		}()
		go func() {
			defer listenerShutdown.Done()
			<-ctx.Done()
			shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = metricsServer.Shutdown(shutdown)
		}()
	}
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()

	log.Println("server listening on :" + cfg.AppPort)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
	<-workerDone
	<-shutdownDone
	listenerShutdown.Wait()
}
