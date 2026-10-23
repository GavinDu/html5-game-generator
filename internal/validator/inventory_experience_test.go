package validator

import (
	"path/filepath"
	"testing"
)

// The shipped sample games are the standard's calibration set: they
// must pass the experience checks. (Historical note: Escape once
// declared milestones with no milestone art and pinned a warning
// here; Phase E gave it hallway scenery visuals and the exception
// was retired — all three games now pass every experience check.)
func TestInventoryGamesExperience(t *testing.T) {
	games := map[string]map[string]string{ // dir -> checkID -> want status (default pass)
		"13ui1k": {},
		"u46ex5": {},
		"2uowg9": {},
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
