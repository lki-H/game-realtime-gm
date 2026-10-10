package main

import (
	"testing"

	"game-realtime-gm/backend/internal/config"
)

func TestGameplayConnectionBudgetCannotBeConsumedOnlyByOwnership(t *testing.T) {
	for _, maximum := range []int{-1, 0, 1} {
		if validateGameplayConnectionBudget(config.Config{Database: config.DatabaseConfig{MaxOpenConns: maximum}}) == nil {
			t.Fatal("gameplay accepted a pool without application capacity")
		}
	}
	for _, maximum := range []int{2, 20} {
		if err := validateGameplayConnectionBudget(config.Config{Database: config.DatabaseConfig{MaxOpenConns: maximum}}); err != nil {
			t.Fatal(err)
		}
	}
}
