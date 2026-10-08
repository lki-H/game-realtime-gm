package config

import "testing"

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
