package task

import (
	"embed"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

//go:embed rules*.json
var bundled embed.FS

type Objective struct {
	Key         string   `json:"key"`
	Type        string   `json:"type"`
	Required    int64    `json:"required"`
	Scope       string   `json:"scope"`
	Persistent  bool     `json:"persistent,omitempty"`
	DependsOn   []string `json:"depends_on,omitempty"`
	TargetID    string   `json:"target_id,omitempty"`
	Opportunity string   `json:"opportunity,omitempty"`
}
type Definition struct {
	Key             string      `json:"key"`
	Version         string      `json:"version,omitempty"`
	RepeatSeconds   int         `json:"repeat_seconds,omitempty"`
	Operation       string      `json:"operation"`
	Reward          int64       `json:"reward"`
	Next            string      `json:"next,omitempty"`
	Locked          bool        `json:"locked,omitempty"`
	RequiresSuccess bool        `json:"requires_success,omitempty"`
	NoDeaths        bool        `json:"no_deaths,omitempty"`
	Objectives      []Objective `json:"objectives"`
}
type Rules struct {
	Version        string       `json:"version"`
	Operation      string       `json:"operation"`
	Difficulty     string       `json:"difficulty"`
	Duration       int          `json:"duration_seconds"`
	Reinforcements int          `json:"reinforcements"`
	SuccessReward  int64        `json:"success_reward"`
	FailureReward  int64        `json:"failure_reward"`
	AbortReward    int64        `json:"abort_reward"`
	FillWait       int          `json:"fill_wait_seconds"`
	Confirmation   int          `json:"confirmation_seconds"`
	Loading        int          `json:"loading_seconds"`
	Reconnect      int          `json:"reconnect_seconds"`
	Gap            int          `json:"gap_seconds"`
	Spawn          int          `json:"spawn_seconds"`
	Objectives     []Objective  `json:"objectives"`
	Tasks          []Definition `json:"tasks"`
}

func Load(path string) (Rules, error) {
	var reader io.ReadCloser
	var err error
	if path == "" {
		reader, err = bundled.Open("rules.json")
	} else {
		reader, err = os.Open(path)
	}
	if err != nil {
		return Rules{}, err
	}
	defer reader.Close()
	return Decode(reader)
}
func LoadProducts() (Rules, error) {
	reader, err := bundled.Open("rules-v2.json")
	if err != nil {
		return Rules{}, err
	}
	defer reader.Close()
	return Decode(reader)
}
func (definition Definition) LogicalVersion(rules Rules) string {
	if definition.Version != "" {
		return definition.Version
	}
	return rules.Version
}
func (definition Definition) Period(now time.Time) string {
	if definition.RepeatSeconds == 0 {
		return "once"
	}
	return strconv.FormatInt(now.UTC().Unix()/int64(definition.RepeatSeconds), 10)
}
func (definition Definition) Immediate() bool {
	return !definition.RequiresSuccess && !definition.NoDeaths
}
func Decode(reader io.Reader) (Rules, error) {
	var rules Rules
	decoder := json.NewDecoder(io.LimitReader(reader, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&rules); err != nil {
		return rules, err
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return rules, errors.New("trailing rule content")
	}
	return rules, rules.Validate()
}
func (rules Rules) Validate() error {
	if rules.Version == "" || rules.Operation == "" || rules.Difficulty == "" || rules.Duration <= 0 || rules.Reinforcements < 0 || rules.SuccessReward < 0 || rules.FailureReward < 0 || rules.AbortReward < 0 || rules.FillWait < 0 || rules.Confirmation <= 0 || rules.Loading <= 0 || rules.Reconnect <= 0 || rules.Gap <= 0 || rules.Spawn <= 0 || len(rules.Objectives) == 0 {
		return errors.New("invalid operation rules")
	}
	validate := func(objectives []Objective) bool {
		seen := map[string]bool{}
		for _, objective := range objectives {
			if strings.TrimSpace(objective.Key) == "" || len(objective.Key) > 64 || seen[objective.Key] || objective.Required <= 0 || !ValidObjectiveType(objective.Type) || (objective.Scope != "self" && objective.Scope != "team" && objective.Scope != "eligible") || (objective.Opportunity != "" && objective.Opportunity != "guaranteed" && objective.Opportunity != "conditional" && objective.Opportunity != "random") {
				return false
			}
			seen[objective.Key] = true
		}
		byKey := map[string]Objective{}
		for _, objective := range objectives {
			byKey[objective.Key] = objective
		}
		visiting := map[string]bool{}
		visited := map[string]bool{}
		var visit func(string) bool
		visit = func(key string) bool {
			if visiting[key] {
				return false
			}
			if visited[key] {
				return true
			}
			objective, exists := byKey[key]
			if !exists {
				return false
			}
			visiting[key] = true
			for _, dependency := range objective.DependsOn {
				if !visit(dependency) {
					return false
				}
			}
			visiting[key] = false
			visited[key] = true
			return true
		}
		for key := range byKey {
			if !visit(key) {
				return false
			}
		}
		return len(objectives) > 0 && len(objectives) <= 16
	}
	if !validate(rules.Objectives) {
		return errors.New("invalid shared objectives")
	}
	definitions := map[string]Definition{}
	for _, definition := range rules.Tasks {
		if definition.Key == "" || len(definition.Key) > 59 || len(definition.Version) > 64 || definition.RepeatSeconds < 0 || definition.RepeatSeconds > 31536000 || definition.Reward < 0 || !validate(definition.Objectives) {
			return errors.New("invalid task")
		}
		if _, exists := definitions[definition.Key]; exists {
			return errors.New("duplicate task")
		}
		definitions[definition.Key] = definition
	}
	for _, definition := range rules.Tasks {
		seen := map[string]bool{}
		for definition.Next != "" {
			if seen[definition.Key] {
				return errors.New("task cycle")
			}
			seen[definition.Key] = true
			next, exists := definitions[definition.Next]
			if !exists {
				return errors.New("missing next task")
			}
			definition = next
		}
	}
	return nil
}
func ValidObjectiveType(kind string) bool {
	return kind == "kill" || kind == "interact" || kind == "reach" || kind == "damage" || kind == "heal" || kind == "rescue"
}
func (rules Rules) Task(key string) (Definition, bool) {
	for _, definition := range rules.Tasks {
		if definition.Key == key {
			return definition, true
		}
	}
	return Definition{}, false
}

func Active(objectives []Objective, progress map[string]int64) map[string]bool {
	required := map[string]int64{}
	for _, objective := range objectives {
		required[objective.Key] = objective.Required
	}
	active := map[string]bool{}
	for _, objective := range objectives {
		eligible := true
		for _, dependency := range objective.DependsOn {
			eligible = eligible && progress[dependency] >= required[dependency]
		}
		active[objective.Key] = eligible
	}
	return active
}
