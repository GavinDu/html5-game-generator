package attestation

import (
	"encoding/json"
	"os"
	"reflect"
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
	if err != nil || !reflect.DeepEqual(restarted, got) {
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
			if !reflect.DeepEqual(result, startScoreDiagnostic{}) {
				t.Fatalf("denied IR leaked a partial diagnostic result: %+v", result)
			}
		})
	}
	result, err := reduceStartScoreDiagnostic(nil)
	if err == nil || !reflect.DeepEqual(result, startScoreDiagnostic{}) {
		t.Fatalf("nil Game IR must fail without score: %+v %v", result, err)
	}
}

func TestStartScoreDiagnosticTypedLedgerPreservesExactIntermediateOrder(t *testing.T) {
	doc := loadStartScoreFixture(t)
	got, err := reduceStartScoreDiagnostic(doc)
	if err != nil {
		t.Fatalf("diagnostic ledger: %v", err)
	}
	want := []startScoreStep{
		{Ordinal: 0, RuleIndex: 0, RuleID: "start_plus", ActionIndex: 0, Delta: 3, Score: 3},
		{Ordinal: 1, RuleIndex: 1, RuleID: "start_minus", ActionIndex: 0, Delta: -2, Score: 1},
	}
	if !reflect.DeepEqual(got.Steps, want) {
		t.Fatalf("unexpected authored-step ledger: got %+v, want %+v", got.Steps, want)
	}
	if got.AppliedActions != len(want) || got.Score != got.Steps[len(got.Steps)-1].Score {
		t.Fatalf("steps and final diagnostic score must agree: %+v", got)
	}

	doc.Rules[0], doc.Rules[1] = doc.Rules[1], doc.Rules[0]
	reordered, err := reduceStartScoreDiagnostic(doc)
	if err != nil {
		t.Fatalf("diagnostic reordered ledger: %v", err)
	}
	if reordered.Score != got.Score ||
		reflect.DeepEqual(reordered.Steps, want) ||
		reordered.Steps[0].Score != -2 ||
		reordered.Steps[1].Score != 1 ||
		reordered.TraceSHA256 == got.TraceSHA256 {
		t.Fatalf("authored order must affect intermediate states and hash: %+v", reordered)
	}
}

func TestStartScoreDiagnosticLedgerIsBoundedAndFailClosed(t *testing.T) {
	doc := loadStartScoreFixture(t)
	for len(doc.Rules) < 8 {
		doc.Rules = append(doc.Rules, gameir.Rule{
			ID:      "extra_" + string(rune('a'+len(doc.Rules))),
			When:    gameir.Condition{Event: "game_start"},
			Actions: []gameir.Action{{Type: "add_score", Value: float64(1_000_000)}},
		})
	}
	got, err := reduceStartScoreDiagnostic(doc)
	if err != nil || got.AppliedActions != 8 || len(got.Steps) != 8 {
		t.Fatalf("8 authored start-score actions must remain bounded: %+v %v", got, err)
	}
	// The ninth rule is unsupported even though its action is otherwise benign.
	doc.Rules = append(doc.Rules, gameir.Rule{
		ID: "ninth", When: gameir.Condition{Event: "game_start"},
		Actions: []gameir.Action{{Type: "add_score", Value: float64(1)}},
	})
	got, err = reduceStartScoreDiagnostic(doc)
	if err == nil || !reflect.DeepEqual(got, startScoreDiagnostic{}) {
		t.Fatalf("overbudget diagnostic must not expose partial ledger: %+v %v", got, err)
	}
}

func TestSharedGoPixiDiagnosticSourceFixtureIdentity(t *testing.T) {
	// The real Chromium/WebKit bootstrap test reads THIS SAME on-disk JSON.
	// It must never drift into a second, independently hand-authored game.
	doc := loadStartScoreFixture(t)
	if doc.Game.ID != "escape-from-injection" ||
		doc.IRVersion != "1.0" || len(doc.Capabilities) != 0 ||
		len(doc.Assets) != 0 || len(doc.InputBindings) != 0 ||
		len(doc.Scenes) != 1 || doc.Scenes[0].ID != "scene" ||
		len(doc.Scenes[0].Entities) != 0 || len(doc.Rules) != 2 {
		t.Fatalf("shared real Pixi/Go diagnostic package drift: game=%q ir=%q",
			doc.Game.ID, doc.IRVersion)
	}
	if doc.Rules[0].ID != "start_plus" || doc.Rules[1].ID != "start_minus" ||
		doc.Rules[0].When.Event != "game_start" ||
		doc.Rules[1].When.Event != "game_start" ||
		len(doc.Rules[0].Actions) != 1 || len(doc.Rules[1].Actions) != 1 ||
		doc.Rules[0].Actions[0].Type != "add_score" ||
		doc.Rules[1].Actions[0].Type != "add_score" {
		t.Fatal("shared authored event/action order was silently changed")
	}
	got, err := reduceStartScoreDiagnostic(doc)
	if err != nil || got.Score != 1 || got.TraceSHA256 != startScoreFixtureDigest ||
		len(got.Steps) != 2 || got.Steps[0].Score != 3 ||
		got.Steps[1].Score != 1 {
		t.Fatalf("shared Go/Pixi source order or diagnostic changed: %+v %v", got, err)
	}
}


func TestStartScoreDiagnostic64FreshDocumentRerunsPreserveOrderAndIsolation(t *testing.T) {
	// This scoped diagnostic has no server-granted seed or input queue yet.
	// Fresh decode on every iteration catches shared mutable reducer state,
	// accidental rule reordering, and a fingerprint based only on final score.
	var baseline, reversedBaseline startScoreDiagnostic
	for attempt := 0; attempt < 64; attempt++ {
		originalDoc := loadStartScoreFixture(t)
		original, err := reduceStartScoreDiagnostic(originalDoc)
		if err != nil {
			t.Fatalf("fresh diagnostic %d: %v", attempt, err)
		}
		if original.Score != 1 || original.AppliedActions != 2 ||
			len(original.Steps) != 2 || original.Steps[0].Score != 3 ||
			original.Steps[1].Score != 1 || original.TraceSHA256 != startScoreFixtureDigest {
			t.Fatalf("fresh diagnostic drift on run %d: %+v", attempt, original)
		}

		reversedDoc := loadStartScoreFixture(t)
		reversedDoc.Rules[0], reversedDoc.Rules[1] = reversedDoc.Rules[1], reversedDoc.Rules[0]
		reversed, err := reduceStartScoreDiagnostic(reversedDoc)
		if err != nil {
			t.Fatalf("reversed diagnostic %d: %v", attempt, err)
		}
		if reversed.Score != original.Score ||
			reversed.TraceSHA256 == original.TraceSHA256 ||
			len(reversed.Steps) != 2 || reversed.Steps[0].Score != -2 ||
			reversed.Steps[1].Score != 1 {
			t.Fatalf("authored-order fingerprint lost on run %d: %+v", attempt, reversed)
		}
		if attempt == 0 {
			baseline, reversedBaseline = original, reversed
		} else if !reflect.DeepEqual(original, baseline) ||
			!reflect.DeepEqual(reversed, reversedBaseline) {
			t.Fatalf("fresh decoded replay changed across attempts: %d", attempt)
		}

		// A mutation to one in-memory document cannot poison a future
		// independent decode or turn a later run into an earlier outcome.
		originalDoc.Rules[0].Actions[0].Value = float64(-1)
		changed, err := reduceStartScoreDiagnostic(originalDoc)
		if err != nil || changed.Score != -3 ||
			changed.TraceSHA256 == baseline.TraceSHA256 {
			t.Fatalf("document mutation not isolated on run %d: %+v %v", attempt, changed, err)
		}
	}
}
