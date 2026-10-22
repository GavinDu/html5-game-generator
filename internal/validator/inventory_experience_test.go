package validator

import (
	"path/filepath"
	"testing"
)

// The shipped sample games are the standard's calibration set: they
// must pass the experience checks (one documented exception: Escape
// declares milestones but has no milestone art, so
// experience.milestones stays a warning there until art exists — the
// check doing its job, recorded here so it can't silently spread).
func TestInventoryGamesExperience(t *testing.T) {
	games := map[string]map[string]string{ // dir -> checkID -> want status (default pass)
		"13ui1k": {},
		"u46ex5": {},
		"2uowg9": {"experience.milestones": StatusFail},
	}
	for dir, exceptions := range games {
		doc, err := readGameIR(filepath.Join("..", "..", "inventory", dir))
		if err != nil {
			t.Fatalf("%s: IR must validate structurally: %v", dir, err)
		}
		if doc == nil {
			t.Fatalf("%s: no game-ir.json", dir)
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
