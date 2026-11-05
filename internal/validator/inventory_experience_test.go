package validator

import (
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
