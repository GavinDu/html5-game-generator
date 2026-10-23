package validator

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// The shipped games' quality contracts were regenerated from their
// IRs (scorecard fusion, Phase D): the check must pass on all three.
func TestQualityContractMatchesShippedGames(t *testing.T) {
	for _, dir := range []string{"13ui1k", "u46ex5", "2uowg9"} {
		gameDir := filepath.Join("..", "..", "inventory", dir)
		doc, err := readGameIR(gameDir)
		if err != nil {
			t.Fatalf("%s: %v", dir, err)
		}
		got := checkQualityContract(gameDir, doc, nil)
		if got.Status != StatusPass {
			t.Errorf("%s: quality contract = %s (%s), want pass", dir, got.Status, got.Evidence)
		}
	}
}

func TestQualityContractSkipsWithoutRules(t *testing.T) {
	dir := t.TempDir()
	doc, _ := readGameIR(filepath.Join("..", "..", "inventory", "13ui1k"))
	got := checkQualityContract(dir, doc, nil)
	if got.Status != StatusSkip {
		t.Fatalf("no rules file: got %s, want skip", got.Status)
	}
}

func TestQualityContractFailsOnDrift(t *testing.T) {
	src := filepath.Join("..", "..", "inventory", "13ui1k")
	dir := t.TempDir()
	raw, err := os.ReadFile(filepath.Join(src, "quality-rules.json"))
	if err != nil {
		t.Fatal(err)
	}
	// Gentle-mode game whose prose claims a miss limit: contradiction.
	tampered := []byte(string(raw))
	needle := []byte(`"missed hearts simply fall away — no penalty (gentle mode)"`)
	if !bytes.Contains(tampered, needle) {
		t.Skip("bobo coreLoop wording changed; update the tamper needle")
	}
	out := bytes.Replace(tampered, needle, []byte(`"survive the declared miss limit"`), 1)
	if err := os.WriteFile(filepath.Join(dir, "quality-rules.json"), out, 0o644); err != nil {
		t.Fatal(err)
	}
	doc, _ := readGameIR(src)
	got := checkQualityContract(dir, doc, nil)
	if got.Status != StatusFail {
		t.Fatalf("contradictory prose: got %s (%s), want fail", got.Status, got.Evidence)
	}
}
