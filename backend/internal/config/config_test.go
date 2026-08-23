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
