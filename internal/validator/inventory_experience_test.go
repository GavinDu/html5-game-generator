package validator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// The shipped sample games are the standard's calibration set. Every
// applicable experience check must pass; the balance model intentionally
// skips non-collect objectives because their economics need game-aware probes.
func TestInventoryGamesExperience(t *testing.T) {
	games := map[string]map[string]string{ // dir -> checkID -> expected non-default status
		"13ui1k": {},
		"u46ex5": {"experience.balance": StatusSkip},
		"2uowg9": {"experience.balance": StatusSkip},
		"p3ngui": {},
	}
	for dir, exceptions := range games {
		doc, err := readGameIR(filepath.Join("..", "..", "inventory", dir))
		if err != nil {
			t.Fatalf("%s: IR must validate structurally: %v", dir, err)
		}
		if doc == nil {
			t.Fatalf("%s: no game-ir.json", dir)
		}
		if doc.Experience == nil {
			t.Fatalf("%s: missing canonical experience block", dir)
		}
		qualityData, err := os.ReadFile(filepath.Join("..", "..", "inventory", dir, "quality-rules.json"))
		if err != nil {
			t.Fatalf("%s: read quality rules: %v", dir, err)
		}
		var quality struct {
			GameplayContract struct {
				Generated struct {
					ControlsScheme   string `json:"controlsScheme"`
					TargetSessionSec int    `json:"targetSessionSec"`
					Objective      struct {
						Kind   string `json:"kind"`
						Target int    `json:"target"`
						Label  string `json:"label"`
					} `json:"objective"`
					Failure struct {
						Mode           string `json:"mode"`
						Health         int    `json:"health"`
						InvulnerableMs int    `json:"invulnerableMs"`
					} `json:"failure"`
				} `json:"generated"`
			} `json:"gameplayContract"`
		}
		if err := json.Unmarshal(qualityData, &quality); err != nil {
			t.Fatalf("%s: decode quality rules: %v", dir, err)
		}
		generated := quality.GameplayContract.Generated
		exp := doc.Experience
		if generated.ControlsScheme != exp.Controls.Scheme ||
			generated.TargetSessionSec != exp.Pacing.TargetSessionSec ||
			generated.Objective.Kind != exp.Objective.Kind ||
			generated.Objective.Target != exp.Objective.Target ||
			generated.Objective.Label != exp.Objective.Label ||
			generated.Failure.Mode != exp.Failure.Mode ||
			generated.Failure.Health != exp.Failure.Health ||
			generated.Failure.InvulnerableMs != exp.Failure.InvulnerableMs {
			t.Errorf("%s: questionnaire/quality rules diverge from canonical Game IR: generated=%+v experience=%+v", dir, generated, exp)
		}
		for _, c := range experienceChecks(doc, nil) {
			want := StatusPass
			if w, ok := exceptions[c.CheckID]; ok {
				want = w
			}
			if c.Status != want {
				t.Errorf("%s %s = %s, want %s (evidence: %s)", dir, c.CheckID, c.Status, want, c.Evidence)
			}
		}
	}
}
