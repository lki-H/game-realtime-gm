package main

import "testing"

func TestReadOnlyCheckerRejectsUnknownModeAndMismatchedIdentityBeforeConnecting(t *testing.T) {
	if err := check("initialize", "game_realtime_local", "unused"); err == nil {
		t.Fatal("read-only tool accepted a writing mode")
	}
	t.Setenv("DB_HOST", "127.0.0.1")
	t.Setenv("DB_PORT", "33306")
	t.Setenv("DB_USER", "gm_local")
	t.Setenv("DB_NAME", "another_database")
	t.Setenv("DB_PASSWORD", "fixture-value-not-a-secret")
	if err := check("schema", "game_realtime_local", "unused"); err == nil || err.Error() != "configured database identity differs from the expected database" {
		t.Fatal("mismatched identity was not rejected before a connection")
	}
}
