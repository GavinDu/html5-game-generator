package attestation

import (
	"encoding/json"
	"os"
	"testing"

	"doodlequest/internal/gameir"
)

const startScoreFixtureDigest = "9751d51614b4eb1b9bf8f03d155b8526c4f21f116b38d9af90b7ef6346af855d"

func loadStartScoreFixture(t *testing.T) *gameir.Document {
	t.Helper()
	raw, err := os.ReadFile("testdata/start_score_ir_v1.json")
	if err != nil {
		t.Fatalf("read shared JS/Go start-score IR: %v", err)
	}
	doc, err := gameir.DecodeAndValidate(raw)
	if err != nil {
		t.Fatalf("strict decode fixture: %v", err)
	}
	return doc
}

func TestStartScoreDiagnosticOrderedPreTickReduction(t *testing.T) {
	doc := loadStartScoreFixture(t)
	before, _ := json.Marshal(doc)
	got, err := reduceStartScoreDiagnostic(doc)
	if err != nil {
		t.Fatalf("diagnostic score reducer: %v", err)
	}
	if got.Profile != replayStartScoreProfileV1 ||
		got.Score != 1 || got.AppliedActions != 2 ||
		got.TraceSHA256 != startScoreFixtureDigest {
		t.Fatalf("shared exact JS pre-tick golden mismatch: %+v", got)
	}
	after, _ := json.Marshal(doc)
	if string(before) != string(after) {
		t.Fatalf("diagnostic reducer mutated published Game IR")
	}
	restarted, err := reduceStartScoreDiagnostic(doc)
	if err != nil || restarted != got {
		t.Fatalf("fresh independent run should replay identically: %+v %v", restarted, err)
	}
}

func TestStartScoreDiagnosticFingerprintCommitsToAuthoredOrder(t *testing.T) {
	doc := loadStartScoreFixture(t)
	normal, err := reduceStartScoreDiagnostic(doc)
	if err != nil {
		t.Fatal(err)
	}
	doc.Rules[0], doc.Rules[1] = doc.Rules[1], doc.Rules[0]
	swapped, err := reduceStartScoreDiagnostic(doc)
	if err != nil {
		t.Fatal(err)
	}
	if swapped.Score != normal.Score || swapped.TraceSHA256 == normal.TraceSHA256 {
		t.Fatalf("same total score must not hide changed per-action order: normal=%+v swapped=%+v", normal, swapped)
	}
}

func TestStartScoreDiagnosticBoundedSignedAndZeroActions(t *testing.T) {
	doc := loadStartScoreFixture(t)
	doc.Rules[0].Actions[0].Value = float64(0)
	doc.Rules[1].Actions[0].Value = float64(-1_000_000)
	result, err := reduceStartScoreDiagnostic(doc)
	if err != nil {
		t.Fatal(err)
	}
	if result.Score != -1_000_000 || result.AppliedActions != 2 {
		t.Fatalf("negative signed exact score not applied: %+v", result)
	}
}

func TestStartScoreDiagnosticRejectsUnsupportedAndInvalidIRWithoutPartialResult(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*gameir.Document)
	}{
		{"unknown_event", func(d *gameir.Document) { d.Rules[0].When.Event = "not_installed" }},
		{"collision", func(d *gameir.Document) { d.Rules[0].When.Event = "collision" }},
		{"timer", func(d *gameir.Document) { d.Rules[0].When.Event = "timer_elapsed" }},
		{"pointer", func(d *gameir.Document) {
			d.InputBindings = []gameir.InputBinding{{Action: "tap", Input: "tap"}}
		}},
		{"fractional_js_truncation", func(d *gameir.Document) { d.Rules[0].Actions[0].Value = float64(2.9) }},
		{"numeric_string_js_coercion", func(d *gameir.Document) { d.Rules[0].Actions[0].Value = "3" }},
		{"nil_default_score", func(d *gameir.Document) { d.Rules[0].Actions[0].Value = nil }},
		{"terminal", func(d *gameir.Document) {
			d.Rules[0].Actions = []gameir.Action{{Type: "end_game", Value: true}}
		}},
		{"scene_world", func(d *gameir.Document) { d.Scenes[0].WorldSize = []float64{320, 480} }},
		{"extra_rules", func(d *gameir.Document) { d.Rules = nil }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := loadStartScoreFixture(t)
			tc.mutate(doc)
			result, err := reduceStartScoreDiagnostic(doc)
			if err == nil {
				t.Fatalf("unsupported/invalid Game IR admitted: %+v", result)
			}
			if result != (startScoreDiagnostic{}) {
				t.Fatalf("denied IR leaked a partial diagnostic result: %+v", result)
			}
		})
	}
	result, err := reduceStartScoreDiagnostic(nil)
	if err == nil || result != (startScoreDiagnostic{}) {
		t.Fatalf("nil Game IR must fail without score: %+v %v", result, err)
	}
}
