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

func TestAuditRuleStorageAndDurationLimits(t *testing.T) {
	for _, mutation := range []struct {
		name   string
		change func(*Rules)
	}{
		{"version", func(rules *Rules) { rules.Version = strings.Repeat("a", 65) }},
		{"target", func(rules *Rules) { rules.Objectives[0].TargetID = strings.Repeat("a", 129) }},
		{"duration", func(rules *Rules) { rules.Duration = int(^uint(0) >> 1) }},
		{"task operation", func(rules *Rules) { rules.Tasks[0].Operation = "" }},
	} {
		t.Run(mutation.name, func(t *testing.T) {
			rules, err := Load("")
			if err != nil {
				t.Fatal(err)
			}
			mutation.change(&rules)
			if rules.Validate() == nil {
				t.Fatal("unusable operation configuration accepted")
			}
		})
	}
}
