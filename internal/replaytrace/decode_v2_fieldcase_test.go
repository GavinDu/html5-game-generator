package replaytrace

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestStrictV2RejectsCaseInsensitiveFieldAliasesAtEveryNestingLevel(t *testing.T) {
	original, err := json.Marshal(validTrace())
	if err != nil {
		t.Fatal(err)
	}
	good := string(original)
	replaced := func(old, next string) []byte {
		t.Helper()
		if !strings.Contains(good, old) {
			t.Fatalf("golden semantic input fixture lost %q", old)
		}
		return []byte(strings.Replace(good, old, next, 1))
	}
	// encoding/json accepts case-insensitive struct-field spellings even
	// with DisallowUnknownFields. A later "Events":[] can overwrite a valid
	// full trace, yielding a completely different canonical input digest.
	if !strings.HasSuffix(good, "}") {
		t.Fatal("golden trace must be a JSON object")
	}
	shadowEvents := []byte(strings.TrimSuffix(good, "}") + `,"Events":[]}`)
	tests := []struct {
		name string
		wire []byte
	}{
		{"shadow-all-events-with-empty-alias", shadowEvents},
		{"duplicate-schema-by-case", replaced(`"schema":`, `"schema":"semantic-input-trace-v2","Schema":`)},
		{"duplicate-duration-by-case", replaced(`"durationTicks":120,`, `"durationTicks":120,"DurationTicks":120,`)},
		{"duplicate-event-action-by-case", replaced(`"action":"jump",`, `"action":"jump","Action":"jump",`)},
		{"duplicate-event-tick-by-case", replaced(`"tick":1,`, `"tick":1,"Tick":1,`)},
		{"duplicate-event-phase-by-case", replaced(`"phase":"down",`, `"phase":"down","Phase":"down",`)},
		{"duplicate-event-position-by-case", replaced(
			`"position":{"x":1000,"y":2000}`,
			`"position":{"x":1000,"y":2000},"Position":{"x":1000,"y":2000}`)},
		{"duplicate-coordinate-x-by-case", replaced(`"x":1000,`, `"x":1000,"X":1000,`)},
		{"duplicate-coordinate-y-by-case", replaced(`"y":2000}`, `"y":2000,"Y":2000}`)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if decoded, err := DecodeStrictV2(sampleIR(), tc.wire); err == nil {
				t.Fatalf("case-alias replay evidence admitted: %+v", decoded)
			}
		})
	}
}

func TestStrictV2PreservesCanonicalEscapedKeysAndCaseSensitiveActionValues(t *testing.T) {
	original, err := json.Marshal(validTrace())
	if err != nil {
		t.Fatal(err)
	}
	good := string(original)
	for _, tc := range []struct {
		name string
		old string
		next string
	}{
		{"escaped-canonical-schema", `"schema":`, `"sch\u0065ma":`},
		{"escaped-canonical-position", `"position":`, `"pos\u0069tion":`},
		{"escaped-canonical-coordinate", `"x":1000`, `"\u0078":1000`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// JSON member escapes are legitimate only when their decoded
			// spelling is exactly the canonical protocol field.
			if !strings.Contains(good, tc.old) {
				t.Fatalf("golden trace lost %q", tc.old)
			}
			wire := []byte(strings.Replace(good, tc.old, tc.next, 1))
			_, err := DecodeStrictV2(sampleIR(), wire)
			if err != nil {
				t.Fatalf("legal escaped canonical JSON key rejected: %v", err)
			}
		})
	}
	ir := sampleIR()
	ir.InputBindings[0].Action = "JuMp"
	trace := validTrace()
	trace.Events[0].Action = "JuMp"
	trace.Events[1].Action = "JuMp"
	wire, err := json.Marshal(trace)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeStrictV2(ir, wire)
	if err != nil || decoded.Events[0].Action != "JuMp" {
		t.Fatalf("action VALUES remain IR-defined and case-sensitive: %v", err)
	}
}
