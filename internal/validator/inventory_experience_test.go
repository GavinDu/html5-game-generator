package validator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// The shipped inventory games are the standard's calibration set: they
// must pass the experience checks. (Historical note: Escape once
// declared milestones with no milestone art and pinned a warning
// here; Phase E gave it hallway scenery visuals and the exception
// was retired — the standardized inventory games pass; Penguin now declares its IR 1.2 experience contract.)
func TestInventoryGamesExperience(t *testing.T) {
	games := map[string]map[string]string{ // dir -> checkID -> want status (default pass)
		"13ui1k": {},
		"u46ex5": {},
		"2uowg9": {},
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

type generatedExperienceContract struct {
	Archetype      string `json:"archetype"`
	ControlsScheme string `json:"controlsScheme"`
	Objective      struct {
		Kind   string `json:"kind"`
		Target int    `json:"target"`
		Label  string `json:"label"`
	} `json:"objective"`
	Failure struct {
		Mode           string `json:"mode"`
		Health         *int   `json:"health"`
		InvulnerableMs *int   `json:"invulnerableMs"`
	} `json:"failure"`
	TargetSessionSec     *int `json:"targetSessionSec"`
	FirstRewardWithinSec *int `json:"firstRewardWithinSec"`
	Milestones           int  `json:"milestones"`
	Scenes               int  `json:"scenes"`
}

func optionalPositiveInt(value int) *int {
	if value <= 0 {
		return nil
	}
	return &value
}

// Quality contracts are a derived view of the canonical Game IR. A
// questionnaire or control-scheme change must not leave a stale quality
// profile that tells the evaluator to judge a different game.
func TestInventoryQualityContractsMatchGameIR(t *testing.T) {
	for _, dir := range []string{"13ui1k", "u46ex5", "2uowg9", "p3ngui"} {
		doc, err := readGameIR(filepath.Join("..", "..", "inventory", dir))
		if err != nil {
			t.Fatalf("%s: IR must validate structurally: %v", dir, err)
		}
		if doc == nil || doc.Experience == nil {
			t.Fatalf("%s: canonical Game IR must declare experience", dir)
		}
		raw, err := os.ReadFile(filepath.Join("..", "..", "inventory", dir, "quality-rules.json"))
		if err != nil {
			t.Fatalf("%s: read quality rules: %v", dir, err)
		}
		var rules struct {
			GameplayContract struct {
				Generated generatedExperienceContract `json:"generated"`
			} `json:"gameplayContract"`
		}
		if err := json.Unmarshal(raw, &rules); err != nil {
			t.Fatalf("%s: decode quality rules: %v", dir, err)
		}
		exp := doc.Experience
		want := generatedExperienceContract{
			Archetype:      exp.Archetype,
			ControlsScheme: exp.Controls.Scheme,
			Failure: struct {
				Mode           string `json:"mode"`
				Health         *int   `json:"health"`
				InvulnerableMs *int   `json:"invulnerableMs"`
			}{Mode: exp.Failure.Mode, Health: optionalPositiveInt(exp.Failure.Health), InvulnerableMs: optionalPositiveInt(exp.Failure.InvulnerableMs)},
			TargetSessionSec:     optionalPositiveInt(exp.Pacing.TargetSessionSec),
			FirstRewardWithinSec: optionalPositiveInt(exp.Pacing.FirstRewardWithinSec),
			Milestones:           len(exp.Milestones),
			Scenes:               len(doc.Scenes),
		}
		want.Objective.Kind = exp.Objective.Kind
		want.Objective.Target = exp.Objective.Target
		want.Objective.Label = exp.Objective.Label
		if !reflect.DeepEqual(rules.GameplayContract.Generated, want) {
			t.Errorf("%s: generated quality contract drifted from Game IR\n got: %#v\nwant: %#v", dir, rules.GameplayContract.Generated, want)
		}
	}
}

// The three uploaded penguin drawings are three progression skins, not
// decorative unused assets: the canonical IR must start with the rainbow
// scarf and unlock the holiday and classic-blue designs at the authored
// four-heart and eight-heart milestones.
func TestPenguinUsesAllThreeSuppliedDrawingVariants(t *testing.T) {
	doc, err := readGameIR(filepath.Join("..", "..", "inventory", "p3ngui"))
	if err != nil {
		t.Fatalf("Penguin IR must validate structurally: %v", err)
	}
	if doc == nil {
		t.Fatal("Penguin game-ir.json is missing")
	}
	assets := map[string]bool{}
	for _, asset := range doc.Assets {
		assets[asset.ID] = true
	}
	for _, id := range []string{"penguin_rainbow", "penguin_holiday", "penguin_classic"} {
		if !assets[id] {
			t.Errorf("supplied drawing variant asset %q is missing", id)
		}
	}

	playerUsesRainbow := false
	for _, scene := range doc.Scenes {
		for _, entity := range scene.Entities {
			if entity.ID == "penguin" && entity.Kind == "player" && entity.AssetID == "penguin_rainbow" {
				playerUsesRainbow = true
			}
		}
	}
	if !playerUsesRainbow {
		t.Fatal("player must begin with the rainbow-scarf drawing")
	}

	unlocks := map[int]string{}
	for _, rule := range doc.Rules {
		if rule.When.Event != "score_reached" || rule.When.Value == nil {
			continue
		}
		for _, action := range rule.Actions {
			if action.Type == "set_asset" && action.Target == "penguin" {
				unlocks[int(*rule.When.Value)] = action.AssetID
			}
		}
	}
	for score, want := range map[int]string{4: "penguin_holiday", 8: "penguin_classic"} {
		if got := unlocks[score]; got != want {
			t.Errorf("penguin skin unlock at %d hearts = %q, want %q", score, got, want)
		}
	}
}
