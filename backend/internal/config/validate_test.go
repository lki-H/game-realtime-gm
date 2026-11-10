package config

import (
	"strings"
	"testing"
)

func validServiceFixture() Config {
	return Config{AppHost: "127.0.0.1", AppPort: "18080", GameplayMode: "v2", JWTSecret: strings.Repeat("j", 64),
		Database: DatabaseConfig{Host: "127.0.0.1", Port: "33306", User: "gm_local", Name: "game_realtime_local", Password: strings.Repeat("d", 64), MaxOpenConns: 20, MaxIdleConns: 10},
		Redis:    RedisConfig{Addr: "127.0.0.1:36379", Password: strings.Repeat("r", 64), PoolSize: 10, MaxActiveConns: 20, PoolTimeoutMS: 1000}}
}

func TestServiceDefaultsDoNotSupplyCredentials(t *testing.T) {
	for _, name := range []string{"DB_PASSWORD", "JWT_SECRET", "APP_HOST"} {
		t.Setenv(name, "")
	}
	cfg := Load()
	if cfg.Database.Password != "" || cfg.JWTSecret != "" || cfg.AppHost != "127.0.0.1" {
		t.Fatal("unsafe implicit service identity or listener")
	}
	if ValidateService(cfg) == nil {
		t.Fatal("missing credentials must fail before connecting")
	}
}

func TestServiceConfigurationRejectsMissingAndReusedSecrets(t *testing.T) {
	for name, mutate := range map[string]func(*Config){
		"database_password": func(cfg *Config) { cfg.Database.Password = "" },
		"jwt":               func(cfg *Config) { cfg.JWTSecret = "" },
		"short_jwt":         func(cfg *Config) { cfg.JWTSecret = "short" },
		"jwt_database":      func(cfg *Config) { cfg.JWTSecret = cfg.Database.Password },
		"jwt_redis":         func(cfg *Config) { cfg.JWTSecret = cfg.Redis.Password },
		"host":              func(cfg *Config) { cfg.AppHost = "" },
		"port":              func(cfg *Config) { cfg.AppPort = "65536" },
	} {
		t.Run(name, func(t *testing.T) {
			cfg := validServiceFixture()
			mutate(&cfg)
			if ValidateService(cfg) == nil {
				t.Fatal("invalid configuration accepted")
			}
		})
	}
}

func TestExplicitExistingStorageAndContainerListenersRemainSupported(t *testing.T) {
	cfg := validServiceFixture()
	cfg.AppHost = "0.0.0.0"
	cfg.Redis.Password = ""
	if err := ValidateService(cfg); err != nil {
		t.Fatal(err)
	}
	cfg.JWTSecret = ""
	if err := ValidateDatabase(cfg.Database); err != nil {
		t.Fatal("database-only maintenance must not require JWT identity", err)
	}
}
