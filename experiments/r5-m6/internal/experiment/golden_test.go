package experiment

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"

	reportingv1 "game-realtime-gm/experiments/r5-m6/gen/reporting/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func goldenMessage(name string) proto.Message {
	switch name {
	case "GetLeaderboardRequest":
		return &reportingv1.GetLeaderboardRequest{}
	case "GetLeaderboardResponse":
		return &reportingv1.GetLeaderboardResponse{}
	case "GetRunResultRequest":
		return &reportingv1.GetRunResultRequest{}
	case "GetRunResultResponse":
		return &reportingv1.GetRunResultResponse{}
	default:
		return nil
	}
}
func TestSharedGoCSharpGoldenBytes(t *testing.T) {
	data, err := os.ReadFile("../../fixtures/protobuf-golden.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		UnknownHex string `json:"unknown_hex"`
		Cases      []struct {
			Type     string          `json:"type"`
			Hex      string          `json:"hex"`
			Expected json.RawMessage `json:"expected"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if len(fixture.Cases) != 4 {
		t.Fatal("all request and response messages must be represented")
	}
	unknown, err := hex.DecodeString(fixture.UnknownHex)
	if err != nil || len(unknown) == 0 {
		t.Fatal("unknown field fixture missing")
	}
	for _, test := range fixture.Cases {
		t.Run(test.Type, func(t *testing.T) {
			want, got := goldenMessage(test.Type), goldenMessage(test.Type)
			if want == nil {
				t.Fatal("unsupported golden type")
			}
			if err := protojson.Unmarshal(test.Expected, want); err != nil {
				t.Fatal(err)
			}
			wire, err := hex.DecodeString(test.Hex)
			if err != nil {
				t.Fatal(err)
			}
			if err := proto.Unmarshal(wire, got); err != nil || !proto.Equal(want, got) {
				t.Fatal("golden bytes decoded to different fields")
			}
			encoded, err := (proto.MarshalOptions{Deterministic: true}).Marshal(want)
			if err != nil || !bytes.Equal(encoded, wire) {
				t.Fatal("Go encoding changed from reviewed shared golden")
			}
			future := append(append([]byte(nil), wire...), unknown...)
			if err := proto.Unmarshal(future, got); err != nil || len(got.ProtoReflect().GetUnknown()) == 0 {
				t.Fatal("unknown field not retained")
			}
			encoded, err = (proto.MarshalOptions{Deterministic: true}).Marshal(got)
			if err != nil || !bytes.Equal(encoded, future) {
				t.Fatal("unknown fields changed on roundtrip")
			}
			got.ProtoReflect().SetUnknown(nil)
			if !proto.Equal(want, got) {
				t.Fatal("unknown field changed known values")
			}
		})
	}
}
