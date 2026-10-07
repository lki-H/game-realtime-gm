package task

import (
	"strings"
	"testing"
)

func TestPublishedRulesAndCycleValidation(t *testing.T) {
	rules, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if rules.Duration != 1200 || rules.Reinforcements != 3 || rules.SuccessReward != 100 || rules.FailureReward != 20 || rules.AbortReward != 0 {
		t.Fatal("unexpected demo rule values")
	}
	rules.Tasks[1].Next = "hunter"
	if rules.Validate() == nil {
		t.Fatal("cyclic chain accepted")
	}
	if _, err := Decode(strings.NewReader(`{"unknown":true}`)); err == nil {
		t.Fatal("unknown fields accepted")
	}
}
