package replaytrace

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestDecodeStrictV2AcceptsDeclaredTraceAndEmptyEvents(t *testing.T) {
	encoded, err := json.Marshal(validTrace())
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeStrictV2(sampleIR(), encoded)
	if err != nil {
		t.Fatalf("valid bounded tap/drag/key trace rejected: %v", err)
	}
	if len(decoded.Events) != len(validTrace().Events) ||
		decoded.Events[0].Action != "jump" ||
		decoded.Events[3].Phase != "move" {
		t.Fatalf("semantic input edges lost in decode: %+v", decoded.Events)
	}

	// A wire-level empty list is distinct from an omitted or null member;
	// the canonical fingerprint helper may subsequently normalize it.
	idle := Trace{Schema: SchemaV2, DurationTicks: 10, Events: []Event{}}
	encoded, err = json.Marshal(idle)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err = DecodeStrictV2(sampleIR(), encoded)
	if err != nil || len(decoded.Events) != 0 {
		t.Fatalf("explicit idle input trace must decode: %+v %v", decoded, err)
	}
}

func TestDecodeStrictV2RejectsAmbiguousUnknownAndTruncatedJSON(t *testing.T) {
	encoded, err := json.Marshal(validTrace())
	if err != nil {
		t.Fatal(err)
	}
	good := string(encoded)
	if !strings.Contains(good, "\"position\":{\"x\":1000,\"y\":2000}") {
		t.Fatal("golden fixture coordinate serialization changed")
	}
	replaced := func(old, next string) []byte {
		t.Helper()
		if !strings.Contains(good, old) {
			t.Fatalf("golden fixture missing %q", old)
		}
		return []byte(strings.Replace(good, old, next, 1))
	}
	tests := []struct {
		name    string
		payload []byte
	}{
		{"duplicate-envelope-key", replaced("\"schema\":", "\"schema\":\"semantic-input-trace-v2\",\"schema\":")},
		{"duplicate-escaped-envelope-key", replaced("\"schema\":", "\"sch\\u0065ma\":\"semantic-input-trace-v2\",\"schema\":")},
		{"duplicate-event-tick", replaced("\"tick\":1", "\"tick\":1,\"tick\":1")},
		{"duplicate-nested-x", replaced("\"x\":1000", "\"x\":1000,\"x\":1000")},
		{"unknown-envelope-member", replaced("\"schema\":", "\"surprise\":true,\"schema\":")},
		{"unknown-event-member", replaced("\"ordinal\":0", "\"unexpected\":true,\"ordinal\":0")},
		{"unknown-position-member", replaced("\"x\":1000", "\"z\":1,\"x\":1000")},
		{"missing-schema", replaced("\"schema\":\"semantic-input-trace-v2\",", "")},
		{"missing-duration", replaced("\"durationTicks\":120,", "")},
		{"missing-events", replaced(",\"events\":", ",\"removedEvents\":")},
		{"missing-event-ordinal", replaced("\"ordinal\":0,", "")},
		{"missing-position-coordinate", replaced("\"x\":1000,", "")},
		{"null-duration", replaced("\"durationTicks\":120", "\"durationTicks\":null")},
		{"null-events", []byte("{\"schema\":\"semantic-input-trace-v2\",\"durationTicks\":10,\"events\":null}")},
		{"null-event", replaced("\"events\":[", "\"events\":[null,")},
		{"null-position", replaced("\"position\":{\"x\":1000,\"y\":2000}", "\"position\":null")},
		{"null-coordinate", replaced("\"x\":1000", "\"x\":null")},
		{"nonintegral-tick", replaced("\"tick\":1", "\"tick\":1.5")},
		{"undeclared-action", replaced("\"action\":\"jump\"", "\"action\":\"teleport\"")},
		{"unbalanced-release", replaced("\"phase\":\"up\"", "\"phase\":\"down\"")},
		{"wrong-top-level-type", []byte("[]")},
		{"null-top-level", []byte("null")},
		{"malformed-json", []byte("{\"schema\":")},
		{"trailing-object", append(append([]byte(nil), encoded...), []byte("{}")...)},
		{"trailing-scalar", append(append([]byte(nil), encoded...), []byte("42")...)},
		{"invalid-utf8", append(append([]byte(nil), encoded...), 0xff)},
		{"too-many-bytes", bytes.Repeat([]byte(" "), MaxEncodedTraceBytes+1)},
		{"excess-nesting", replaced("\"schema\":", "\"ignored\":[[[[[[[0]]]]]]],\"schema\":")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if trace, err := DecodeStrictV2(sampleIR(), tc.payload); err == nil {
				t.Fatalf("malformed/ambiguous semantic trace admitted: %+v", trace)
			}
		})
	}
}

func TestDecodeStrictV2RejectsOversizedEventArrayBeforeSemanticReplay(t *testing.T) {
	trace := Trace{
		Schema: SchemaV2, DurationTicks: 1,
		Events: make([]Event, MaxEvents+1),
	}
	raw, err := json.Marshal(trace)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) > MaxEncodedTraceBytes {
		t.Fatalf("test must exercise event-count bound, not byte budget: %d", len(raw))
	}
	if _, err := DecodeStrictV2(sampleIR(), raw); err == nil {
		t.Fatal("event-count overflow should reject before game simulation")
	}
}
