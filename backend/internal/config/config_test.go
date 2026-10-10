package config

import "testing"

func TestAppHostCanRestrictListenerToLoopback(t *testing.T) {
	t.Setenv("APP_HOST", "")
	if Load().AppHost != "" {
		t.Fatal("default listener must retain existing behavior")
	}
	t.Setenv("APP_HOST", "127.0.0.1")
	if Load().AppHost != "127.0.0.1" {
		t.Fatal("explicit listener host was ignored")
	}
}

func TestDefaultGameplayModeIsV2AndLegacyRequiresExplicitSelection(t *testing.T) {
	t.Setenv("GAMEPLAY_MODE", "")
	configuration := Load()
	if configuration.GameplayMode != "v2" || !configuration.Database.UTC {
		t.Fatal("default gameplay must use v2 and UTC")
	}
	t.Setenv("GAMEPLAY_MODE", "legacy")
	configuration = Load()
	if configuration.GameplayMode != "legacy" || configuration.Database.UTC {
		t.Fatal("explicit legacy must retain legacy time semantics")
	}
}

func TestGetEnvBool(t *testing.T) {
	t.Setenv("DAY34_BOOL", "true")
	if !getEnvBool("DAY34_BOOL", false) {
		t.Fatal("true should be parsed")
	}

	t.Setenv("DAY34_BOOL", "invalid")
	if !getEnvBool("DAY34_BOOL", true) {
		t.Fatal("invalid value should use default")
	}
}

func TestLoadPVEConfig(t *testing.T) {
	t.Setenv("GAMEPLAY_MODE", "v2")
	t.Setenv("PVE_TEST_EVENTS_ENABLED", "true")
	t.Setenv("PVE_RULES_PATH", "rules.fixture.json")
	config := Load()
	if config.GameplayMode != "v2" || !config.PVE.TestEventsEnabled || config.PVE.RulesPath != "rules.fixture.json" {
		t.Fatal("unexpected v2 config")
	}
}

func TestConnectionBudgetsAreExplicitAndConfigurable(t *testing.T) {
	for _, name := range []string{"DB_MAX_OPEN_CONNS", "DB_MAX_IDLE_CONNS", "REDIS_POOL_SIZE", "REDIS_MAX_ACTIVE_CONNS", "REDIS_POOL_TIMEOUT_MS"} {
		t.Setenv(name, "")
	}
	configuration := Load()
	if configuration.Database.MaxOpenConns != 20 || configuration.Database.MaxIdleConns != 10 || configuration.Redis.PoolSize != 10 || configuration.Redis.MaxActiveConns != 20 || configuration.Redis.PoolTimeoutMS != 1000 {
		t.Fatal("connection budgets must have bounded defaults")
	}
	t.Setenv("DB_MAX_OPEN_CONNS", "4")
	t.Setenv("DB_MAX_IDLE_CONNS", "1")
	t.Setenv("REDIS_POOL_SIZE", "2")
	t.Setenv("REDIS_MAX_ACTIVE_CONNS", "3")
	t.Setenv("REDIS_POOL_TIMEOUT_MS", "200")
	configuration = Load()
	if configuration.Database.MaxOpenConns != 4 || configuration.Database.MaxIdleConns != 1 || configuration.Redis.PoolSize != 2 || configuration.Redis.MaxActiveConns != 3 || configuration.Redis.PoolTimeoutMS != 200 {
		t.Fatal("configured connection budgets were ignored")
	}
}
