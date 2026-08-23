package observation

import (
	"strings"
	"testing"
)

func TestSettlementWhere(t *testing.T) {
	whereSQL, args := settlementWhere("training_ground", 9)
	if !strings.Contains(whereSQL, "m.status = 'settled'") ||
		!strings.Contains(whereSQL, "m.mission_id = ?") ||
		!strings.Contains(whereSQL, "participant_reward.player_id = ?") {
		t.Fatalf("unexpected where SQL: %s", whereSQL)
	}
	if len(args) != 2 || args[0] != "training_ground" || args[1] != int64(9) {
		t.Fatalf("unexpected args: %#v", args)
	}
}

func TestSettlementWhereWithoutFilters(t *testing.T) {
	whereSQL, args := settlementWhere("", 0)
	if whereSQL != "WHERE m.status = 'settled'" {
		t.Fatalf("unexpected where SQL: %s", whereSQL)
	}
	if len(args) != 0 {
		t.Fatalf("unexpected args: %#v", args)
	}
}
