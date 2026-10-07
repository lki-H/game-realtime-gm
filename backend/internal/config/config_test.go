package config

import "testing"

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
