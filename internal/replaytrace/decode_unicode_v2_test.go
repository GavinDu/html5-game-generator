package replaytrace

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSemanticTraceV2RejectsUnpairedEscapedSurrogateBeforeIRBinding(t *testing.T) {
	original, err := json.Marshal(validTrace())
	if err != nil {
		t.Fatal(err)
	}
	const actionWire = "\"action\":\"jump\""
	if !strings.Contains(string(original), actionWire) {
		t.Fatal("golden trace lost expected action")
	}

	// Without the raw-wire preflight, encoding/json turns a lone \uD800
	// into U+FFFD, making malformed evidence equal a *valid* declared
	// action. This must be rejected before any canonical SHA is computed.
	published := sampleIR()
	published.InputBindings[0].Action = "ju\uFFFDmp"
	for _, escaped := range []string{
		"ju\\ud800mp", // high surrogate only
		"ju\\udfffmp", // low surrogate only
		"ju\\ud800\\ud800mp", // high followed by high
		"ju\\ud800\\u0061mp", // high followed by ordinary codepoint
		"ju\\ud800\\u1234mp", // high followed by non-low scalar
	} {
		t.Run(escaped, func(t *testing.T) {
			wire := strings.ReplaceAll(string(original), actionWire,
				"\"action\":\""+escaped+"\"")
			if _, err := DecodeStrictV2(published, []byte(wire)); err == nil {
				t.Fatalf("malformed UTF-16 escaped action admitted: %s", escaped)
			}
		})
	}
}

func TestSemanticTraceV2PreservesPairedSurrogateAndLiteralBackslash(t *testing.T) {
	original, err := json.Marshal(validTrace())
	if err != nil {
		t.Fatal(err)
	}

	published := sampleIR()
	published.InputBindings[0].Action = "ju🚀mp"
	wire := strings.ReplaceAll(string(original), "\"action\":\"jump\"",
		"\"action\":\"ju\\uD83D\\uDE80mp\"")
	decoded, err := DecodeStrictV2(published, []byte(wire))
	if err != nil {
		t.Fatalf("valid UTF-16 surrogate pair rejected: %v", err)
	}
	if decoded.Events[0].Action != "ju🚀mp" {
		t.Fatalf("valid surrogate pair changed semantic action: %q", decoded.Events[0].Action)
	}

	// A JSON escaped literal backslash followed by "uD800" must not be
	// mistaken for an actual Unicode escape and rejected.
	published.InputBindings[0].Action = "ju\\ud800mp"
	trace := validTrace()
	trace.Events[0].Action = published.InputBindings[0].Action
	trace.Events[1].Action = published.InputBindings[0].Action
	encoded, err := json.Marshal(trace)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err = DecodeStrictV2(published, encoded)
	if err != nil || decoded.Events[0].Action != published.InputBindings[0].Action {
		t.Fatalf("literal backslash Unicode text changed: %v", err)
	}
}
