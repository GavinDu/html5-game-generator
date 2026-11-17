package attestation

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"testing"

	"doodlequest/internal/gameir"
)

// A checked-in, explicitly NON-AUTHORITATIVE Go ledger expectation shared by
// real unmodified Chromium/WebKit Pixi bootstrap and the preinstalled pure
// JS semantic shadow. Neither probe grants a verified user score.
type crossLanguageStartStep struct {
	Ordinal     int   `json:"ordinal"`
	RuleIndex   int   `json:"ruleIndex"`
	RuleID      string `json:"ruleId"`
	ActionIndex int   `json:"actionIndex"`
	Delta       int64 `json:"delta"`
	Score       int64 `json:"score"`
}

type crossLanguageStartGolden struct {
	Schema         string                   `json:"schema"`
	FixtureSHA256  string                   `json:"fixtureSha256"`
	GoProfile      string                   `json:"goProfile"`
	GoTraceSHA256  string                   `json:"goTraceSha256"`
	AppliedActions int                      `json:"appliedActions"`
	FinalScore     int64                    `json:"finalScore"`
	Steps          []crossLanguageStartStep `json:"steps"`
	Attested       bool                     `json:"attested"`
}

func TestGoStartupDiagnosticBindsExactRealPixiGoldenArtifact(t *testing.T) {
	sourceBytes, err := os.ReadFile("testdata/start_score_ir_v1.json")
	if err != nil {
		t.Fatalf("read exact browser-intercepted Game IR source: %v", err)
	}
	ir, err := gameir.DecodeAndValidate(sourceBytes)
	if err != nil {
		t.Fatalf("validate exact Go/Pixi source Game IR: %v", err)
	}
	diagnostic, err := reduceStartScoreDiagnostic(ir)
	if err != nil {
		t.Fatalf("produce Go startup diagnostic: %v", err)
	}
	steps := make([]crossLanguageStartStep, 0, len(diagnostic.Steps))
	for _, step := range diagnostic.Steps {
		steps = append(steps, crossLanguageStartStep{
			Ordinal: step.Ordinal, RuleIndex: step.RuleIndex,
			RuleID: step.RuleID, ActionIndex: step.ActionIndex,
			Delta: step.Delta, Score: step.Score,
		})
	}
	bound := crossLanguageStartGolden{
		Schema:         "runtime-start-score-go-pixi-golden-v1",
		FixtureSHA256:  fmt.Sprintf("%x", sha256.Sum256(sourceBytes)),
		GoProfile:      diagnostic.Profile,
		GoTraceSHA256:  diagnostic.TraceSHA256,
		AppliedActions: diagnostic.AppliedActions,
		FinalScore:     diagnostic.Score,
		Steps:          steps,
		Attested:       false,
	}
	goldenBytes, err := os.ReadFile("../../tools/playtest/fixtures/runtime-start-score-go-pixi-golden-v1.json")
	if err != nil {
		t.Fatalf("read source-controlled cross-language golden: %v", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(goldenBytes))
	decoder.DisallowUnknownFields()
	var expected crossLanguageStartGolden
	if err := decoder.Decode(&expected); err != nil {
		t.Fatalf("invalid immutable Go/Pixi golden schema: %v", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatalf("cross-language golden must contain exactly one JSON document: %v", err)
	}
	if !reflect.DeepEqual(expected, bound) {
		t.Fatalf("source-controlled browser golden drifted from independent Go typed reducer: expected=%+v, recomputed=%+v", expected, bound)
	}
	if expected.Attested || expected.FinalScore != 1 || len(expected.Steps) != 2 ||
		expected.Steps[0].Score != 3 || expected.Steps[1].Score != 1 {
		t.Fatalf("real Pixi golden cannot claim a trusted verdict or lose exact action order: %+v", expected)
	}

	// Same final (commutative) score cannot hide altered authored rule order.
	reversed, err := gameir.DecodeAndValidate(sourceBytes)
	if err != nil {
		t.Fatalf("decode independent reversed Game IR: %v", err)
	}
	reversed.Rules[0], reversed.Rules[1] = reversed.Rules[1], reversed.Rules[0]
	reversedDiagnostic, err := reduceStartScoreDiagnostic(reversed)
	if err != nil {
		t.Fatalf("reduce reversed Go authored actions: %v", err)
	}
	if reversedDiagnostic.Score != expected.FinalScore ||
		reversedDiagnostic.TraceSHA256 == expected.GoTraceSHA256 ||
		reversedDiagnostic.Steps[0].Score != -2 ||
		reversedDiagnostic.Steps[1].Score != 1 {
		t.Fatalf("Go comparator lost ordinal-sensitive intermediate state: %+v", reversedDiagnostic)
	}
}
