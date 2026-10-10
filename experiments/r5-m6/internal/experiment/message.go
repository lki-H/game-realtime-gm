package experiment

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"time"

	reportingv1 "game-realtime-gm/experiments/r5-m6/gen/reporting/v1"
)

var ErrInvalidMessage = errors.New("invalid_message")
var ErrConflict = errors.New("message_conflict")
var ErrExpired = errors.New("expired_message")

var messageIdentifier = regexp.MustCompile(`^[A-Za-z0-9_:-]{1,128}$`)

type Participant struct {
	PlayerID      int64  `json:"player_id"`
	Qualified     bool   `json:"qualified"`
	TaskCompleted bool   `json:"task_completed"`
	Reward        int64  `json:"reward"`
	SettledAt     string `json:"settled_at"`
}
type Report struct {
	RunID        string        `json:"run_id"`
	Operation    string        `json:"operation"`
	Difficulty   string        `json:"difficulty"`
	EndReason    string        `json:"end_reason"`
	Participants []Participant `json:"participants"`
}
type Envelope struct {
	MessageID     string          `json:"message_id"`
	SchemaVersion int             `json:"schema_version"`
	Source        string          `json:"source"`
	PublishedAt   time.Time       `json:"published_at"`
	Type          string          `json:"type"`
	PayloadHash   string          `json:"payload_hash"`
	Payload       json.RawMessage `json:"payload"`
}

func Hash(data []byte) string { digest := sha256.Sum256(data); return hex.EncodeToString(digest[:]) }
func ReportFrom(result *reportingv1.GetRunResultResponse) Report {
	report := Report{RunID: result.RunId, Operation: result.Operation, Difficulty: result.Difficulty, EndReason: result.EndReason}
	for _, participant := range result.Participants {
		report.Participants = append(report.Participants, Participant{participant.PlayerId, participant.ContributionQualified, participant.TaskCompleted, participant.Reward, participant.SettledAt})
	}
	return report
}
func NewEnvelope(id, source string, stamp time.Time, report Report) Envelope {
	payload, _ := json.Marshal(report)
	return Envelope{id, 1, source, stamp.UTC(), "run.settled", Hash(payload), payload}
}
func strictJSON(data []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(value) != nil {
		return ErrInvalidMessage
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return ErrInvalidMessage
	}
	return nil
}
func DecodeEnvelope(data []byte, source string, now time.Time, maxAge time.Duration) (Envelope, Report, error) {
	var envelope Envelope
	var report Report
	if len(data) > 65536 || strictJSON(data, &envelope) != nil {
		return envelope, report, ErrInvalidMessage
	}
	if !messageIdentifier.MatchString(envelope.MessageID) || envelope.Source != source || !identifier.MatchString(source) || envelope.SchemaVersion != 1 || envelope.Type != "run.settled" || strictJSON(envelope.Payload, &report) != nil {
		return envelope, report, ErrInvalidMessage
	}
	canonical, _ := json.Marshal(report)
	if Hash(canonical) != envelope.PayloadHash {
		return envelope, report, ErrInvalidMessage
	}
	envelope.Payload = canonical
	if envelope.PublishedAt.IsZero() || envelope.PublishedAt.After(now.Add(time.Minute)) {
		return envelope, report, ErrInvalidMessage
	}
	if !identifier.MatchString(report.RunID) || !identifier.MatchString(report.Operation) || !identifier.MatchString(report.Difficulty) || !identifier.MatchString(report.EndReason) || len(report.Participants) < 1 || len(report.Participants) > 4 {
		return envelope, report, ErrInvalidMessage
	}
	players := make(map[int64]bool)
	for _, participant := range report.Participants {
		stamp, err := time.Parse(time.RFC3339Nano, participant.SettledAt)
		if participant.PlayerID <= 0 || players[participant.PlayerID] || participant.Reward < 0 || err != nil || stamp.After(now.Add(time.Minute)) {
			return envelope, report, ErrInvalidMessage
		}
		players[participant.PlayerID] = true
	}
	if now.Sub(envelope.PublishedAt) > maxAge {
		return envelope, report, ErrExpired
	}
	return envelope, report, nil
}

func (envelope Envelope) Fingerprint() string {
	encoded, _ := json.Marshal(envelope)
	return Hash(encoded)
}
