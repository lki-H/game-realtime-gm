package experiment

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	reportingv1 "game-realtime-gm/experiments/r5-m6/gen/reporting/v1"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

func protoname(name string) protoreflect.Name { return protoreflect.Name(name) }

func TestContractBaselineAndUnknownFields(t *testing.T) {
	source, err := os.ReadFile("../../proto/reporting.proto")
	if err != nil {
		t.Fatal(err)
	}
	sourceHash, err := os.ReadFile("../../proto/source.sha256")
	if err != nil || Hash(source) != strings.TrimSpace(string(sourceHash)) {
		t.Fatal("proto source changed without regeneration")
	}
	encodedDescriptor, err := os.ReadFile("../../proto/reporting.pb")
	if err != nil {
		t.Fatal(err)
	}
	var descriptorSet descriptorpb.FileDescriptorSet
	if proto.Unmarshal(encodedDescriptor, &descriptorSet) != nil || len(descriptorSet.File) != 1 || !proto.Equal(descriptorSet.File[0], protodesc.ToFileDescriptorProto(reportingv1.File_reporting_proto)) {
		t.Fatal("generated descriptor and Go contract disagree")
	}
	data, err := os.ReadFile("../../proto/compatibility.json")
	if err != nil {
		t.Fatal(err)
	}
	var baseline struct {
		Package  string
		Service  []string
		Methods  map[string]string
		Messages map[string]map[string]string
	}
	if err := json.Unmarshal(data, &baseline); err != nil {
		t.Fatal(err)
	}
	descriptor := reportingv1.File_reporting_proto
	if string(descriptor.Package()) != baseline.Package {
		t.Fatal("protobuf package changed")
	}
	for _, method := range baseline.Service {
		if descriptor.Services().Get(0).Methods().ByName(protoname(method)) == nil {
			t.Fatal("RPC removed")
		}
	}
	for name, expected := range baseline.Methods {
		method := descriptor.Services().Get(0).Methods().ByName(protoname(name))
		if method == nil || string(method.Input().FullName())+":"+string(method.Output().FullName()) != expected || method.IsStreamingClient() || method.IsStreamingServer() {
			t.Fatalf("RPC signature changed: %s", name)
		}
	}
	for name, fields := range baseline.Messages {
		message := descriptor.Messages().ByName(protoname(name))
		if message == nil {
			t.Fatalf("message removed: %s", name)
		}
		for fieldName, expected := range fields {
			field := message.Fields().ByName(protoname(fieldName))
			if field == nil {
				t.Fatalf("field removed: %s.%s", name, fieldName)
			}
			actual := fmt.Sprintf("%d:%s", field.Number(), field.Kind())
			if field.Message() != nil {
				actual += ":" + string(field.Message().FullName())
			}
			if field.IsList() {
				actual += ":repeated"
			}
			if actual != expected {
				t.Fatalf("wire contract changed: %s.%s", name, fieldName)
			}
		}
	}
	encoded, err := proto.Marshal(&reportingv1.GetLeaderboardRequest{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	encoded = protowire.AppendTag(encoded, 99, protowire.BytesType)
	encoded = protowire.AppendString(encoded, "future-field")
	var request reportingv1.GetLeaderboardRequest
	if err := proto.Unmarshal(encoded, &request); err != nil || request.Limit != 10 || len(request.ProtoReflect().GetUnknown()) == 0 {
		t.Fatal("unknown field compatibility broken")
	}
}
func fixtureReport() Report {
	return Report{RunID: "run_fixture", Operation: "training_ground", Difficulty: "normal", EndReason: "success", Participants: []Participant{{PlayerID: 1, Qualified: true, Reward: 100, SettledAt: time.Now().UTC().Format(time.RFC3339Nano)}}}
}
func TestEnvelopeValidation(t *testing.T) {
	now := time.Now().UTC()
	valid := NewEnvelope("fixture_1", "test_source", now, fixtureReport())
	for _, scenario := range []struct {
		name     string
		mutate   func(*Envelope)
		expected error
	}{
		{"valid", func(envelope *Envelope) {}, nil},
		{"changed_hash", func(envelope *Envelope) { envelope.PayloadHash = "bad" }, ErrInvalidMessage},
		{"schema", func(envelope *Envelope) { envelope.SchemaVersion = 2 }, ErrInvalidMessage},
		{"source", func(envelope *Envelope) { envelope.Source = "other" }, ErrInvalidMessage},
		{"expired", func(envelope *Envelope) { envelope.PublishedAt = now.Add(-2 * time.Hour) }, ErrExpired},
		{"future", func(envelope *Envelope) { envelope.PublishedAt = now.Add(time.Hour) }, ErrInvalidMessage},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			envelope := valid
			scenario.mutate(&envelope)
			data, _ := json.Marshal(envelope)
			_, _, err := DecodeEnvelope(data, "test_source", now, time.Hour)
			if err != scenario.expected {
				t.Fatalf("got %v", err)
			}
		})
	}
	data, _ := json.Marshal(valid)
	var normalized any
	_ = json.Unmarshal(data, &normalized)
	normalizedData, _ := json.MarshalIndent(normalized, "", "  ")
	if _, _, err := DecodeEnvelope(normalizedData, "test_source", now, time.Hour); err != nil {
		t.Fatal("MySQL JSON formatting breaks hash")
	}
}
