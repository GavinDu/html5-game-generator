package validator

import (
	"encoding/json"
	"fmt"
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
					Archetype            string `json:"archetype"`
					ControlsScheme       string `json:"controlsScheme"`
					TargetSessionSec     int    `json:"targetSessionSec"`
					FirstRewardWithinSec int    `json:"firstRewardWithinSec"`
					Milestones           int    `json:"milestones"`
					Scenes               int    `json:"scenes"`
					Objective            struct {
						Kind             string `json:"kind"`
						Target           int    `json:"target"`
						Label            string `json:"label"`
						ProgressVariable string `json:"progressVariable"`
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
		if generated.Archetype != exp.Archetype ||
			generated.ControlsScheme != exp.Controls.Scheme ||
			generated.TargetSessionSec != exp.Pacing.TargetSessionSec ||
			generated.FirstRewardWithinSec != exp.Pacing.FirstRewardWithinSec ||
			generated.Milestones != len(exp.Milestones) ||
			generated.Scenes != len(doc.Scenes) ||
			generated.Objective.Kind != exp.Objective.Kind ||
			generated.Objective.Target != exp.Objective.Target ||
			generated.Objective.Label != exp.Objective.Label ||
			generated.Objective.ProgressVariable != exp.Objective.ProgressVariable ||
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

// All three supplied penguin drawings must be used as progression skins:
// rainbow at start, holiday at four hearts, and classic blue at eight.
func TestBoboRegularHeartsAloneRestoreRainbow(t *testing.T) {
	dir := filepath.Join("..", "..", "inventory", "13ui1k")
	doc, err := readGameIR(dir)
	if err != nil {
		t.Fatalf("Bobo IR must validate structurally: %v", err)
	}
	if doc.Experience.Objective.ProgressVariable != "hearts_collected" {
		t.Fatalf("Bobo objective progress variable = %q, want hearts_collected", doc.Experience.Objective.ProgressVariable)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "game-ir.json"))
	if err != nil {
		t.Fatalf("read Bobo Game IR: %v", err)
	}
	type condition struct {
		Variable string `json:"variable"`
		Operator string `json:"operator"`
		Value    any    `json:"value"`
	}
	type action struct {
		Type     string `json:"type"`
		Variable string `json:"variable"`
		Value    any    `json:"value"`
	}
	type rule struct {
		ID         string `json:"id"`
		When       struct {
			Event string `json:"event"`
		} `json:"when"`
		Conditions []condition `json:"conditions"`
		Actions    []action    `json:"actions"`
	}
	var ir struct {
		Rules []rule `json:"rules"`
	}
	if err := json.Unmarshal(raw, &ir); err != nil {
		t.Fatalf("decode Bobo rules: %v", err)
	}
	byID := map[string]rule{}
	for _, r := range ir.Rules {
		byID[r.ID] = r
	}
	hasProgressIncrement := func(r rule) bool {
		for _, a := range r.Actions {
			if a.Type == "add_variable" && a.Variable == "hearts_collected" {
				if value, ok := a.Value.(float64); ok && value == 1 {
					return true
				}
			}
		}
		return false
	}
	for _, id := range []string{"heart-caught", "heart-blue-caught"} {
		r, ok := byID[id]
		if !ok || !hasProgressIncrement(r) {
			t.Errorf("%s must add exactly one regular heart to hearts_collected", id)
		}
	}
	gold, ok := byID["heart-gold-caught"]
	if !ok {
		t.Fatal("golden-heart rule is missing")
	}
	if hasProgressIncrement(gold) {
		t.Error("golden hearts must not advance rainbow restoration progress")
	}
	for band := 0; band < 6; band++ {
		id := fmt.Sprintf("band%d-reveal", band)
		r, ok := byID[id]
		if !ok || r.When.Event != "variable_changed" {
			t.Errorf("%s must react to hearts_collected changes", id)
			continue
		}
		want := float64((band + 1) * 2)
		found := false
		for _, c := range r.Conditions {
			if c.Variable == "hearts_collected" && c.Operator == "eq" {
				if value, ok := c.Value.(float64); ok && value == want {
					found = true
				}
			}
		}
		if !found {
			t.Errorf("%s must reveal at exactly %g regular hearts", id, want)
		}
	}
	win, ok := byID["rainbow-complete"]
	if !ok {
		t.Fatal("rainbow-complete rule is missing")
	}
	winAtTwelve := false
	for _, c := range win.Conditions {
		if c.Variable == "hearts_collected" && c.Operator == "eq" {
			if value, ok := c.Value.(float64); ok && value == 12 {
				winAtTwelve = true
			}
		}
	}
	endGame := false
	for _, a := range win.Actions {
		if a.Type == "end_game" {
			if value, ok := a.Value.(bool); ok && value {
				endGame = true
			}
		}
	}
	if !winAtTwelve || !endGame {
		t.Error("Bobo must win only when hearts_collected reaches exactly 12")
	}
}

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

func TestGameIRSchemaDeclaresExperienceV12(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "schemas", "game-ir-v1.schema.json"))
	if err != nil {
		t.Fatalf("read Game IR schema: %v", err)
	}
	var root struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(raw, &root); err != nil {
		t.Fatalf("decode Game IR schema: %v", err)
	}
	if _, ok := root.Properties["experience"]; !ok {
		t.Fatal("Game IR schema must declare the standardized experience block")
	}
	var version struct {
		Enum []string `json:"enum"`
	}
	if err := json.Unmarshal(root.Properties["irVersion"], &version); err != nil {
		t.Fatalf("decode irVersion schema: %v", err)
	}
	if !schemaEnumContains(version.Enum, "1.2") {
		t.Fatal("Game IR schema must accept irVersion 1.2")
	}
	var capabilities struct {
		Items struct {
			Enum []string `json:"enum"`
		} `json:"items"`
	}
	if err := json.Unmarshal(root.Properties["capabilities"], &capabilities); err != nil {
		t.Fatalf("decode capabilities schema: %v", err)
	}
	if !schemaEnumContains(capabilities.Items.Enum, "shell.v1") {
		t.Fatal("Game IR schema must declare shell.v1 capability")
	}
}

func schemaEnumContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestInventoryTextEntitiesClearNativeHeader(t *testing.T) {
	for _, dir := range []string{"13ui1k", "u46ex5", "2uowg9", "p3ngui"} {
		doc, err := readGameIR(filepath.Join("..", "..", "inventory", dir))
		if err != nil {
			t.Fatalf("%s: IR must validate structurally: %v", dir, err)
		}
		for _, scene := range doc.Scenes {
			for _, entity := range scene.Entities {
				if entity.Kind == "text" && len(entity.Position) > 1 && entity.Position[1] < 110 {
					t.Errorf("%s/%s: text entity y=%.1f overlaps the native player header; want y >= 110", dir, entity.ID, entity.Position[1])
				}
			}
		}
	}
}
