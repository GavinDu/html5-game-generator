package validator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The shipped sample games are the standard's calibration set. Every
// applicable experience check must pass; the balance model intentionally
// skips non-collect objectives because their economics need game-aware probes.
func TestInventoryGamesExperience(t *testing.T) {
	games := map[string]map[string]string{ // dir -> checkID -> expected non-default status
		"13ui1k": {},
		"u46ex5": {},
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

func TestRibbonSixUniqueHolesDriveVictoryIndependentlyOfBonusScore(t *testing.T) {
	dir := filepath.Join("..", "..", "inventory", "u46ex5")
	doc, err := readGameIR(dir)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Experience.Objective.ProgressVariable != "holes_stitched" || doc.Experience.Objective.Target != 6 {
		t.Fatal("Ribbon's objective must be six repaired holes, not shard score")
	}
	raw, err := os.ReadFile(filepath.Join(dir, "game-ir.json"))
	if err != nil {
		t.Fatal(err)
	}
	var ir struct {
		Scenes []struct {
			Entities []struct {
				ID         string   `json:"id"`
				Kind       string   `json:"kind"`
				Components []string `json:"components"`
				Physics    struct {
					Sensor bool `json:"sensor"`
				} `json:"physics"`
				Properties map[string]any `json:"properties"`
			} `json:"entities"`
		} `json:"scenes"`
		Rules []struct {
			ID   string `json:"id"`
			When struct {
				Event  string `json:"event"`
				Target string `json:"target"`
			} `json:"when"`
			Conditions []struct {
				Variable string `json:"variable"`
				Operator string `json:"operator"`
				Value    any    `json:"value"`
			} `json:"conditions"`
			Actions []struct {
				Type     string `json:"type"`
				Target   string `json:"target"`
				Variable string `json:"variable"`
				AssetID  string `json:"assetId"`
				Value    any    `json:"value"`
			} `json:"actions"`
		} `json:"rules"`
	}
	if err := json.Unmarshal(raw, &ir); err != nil {
		t.Fatal(err)
	}
	holes := map[string]bool{}
	for _, e := range ir.Scenes[0].Entities {
		if strings.HasPrefix(e.ID, "hole") {
			collectible := false
			for _, c := range e.Components {
				if c == "collectible" {
					collectible = true
				}
			}
			if !collectible || !e.Physics.Sensor || e.Kind != "pickup" || e.Properties["closeOnCollect"] != true {
				t.Errorf("%s must have a contactable non-blocking sensor", e.ID)
			}
			holes[e.ID] = true
		}
	}
	if len(holes) != 6 {
		t.Errorf("expected six unique playable sky holes, got %d", len(holes))
	}
	stitches := map[string]bool{}
	hasWin := false
	for _, rule := range ir.Rules {
		increment := 0
		closed := false
		ends := false
		for _, a := range rule.Actions {
			if a.Type == "add_variable" && a.Variable == "holes_stitched" && a.Value == float64(1) {
				increment++
			}
			if a.Type == "set_asset" && a.Target == rule.When.Target && a.AssetID == "hole_closed" {
				closed = true
			}
			if a.Type == "end_game" && a.Value == true {
				ends = true
			}
		}
		if rule.When.Event == "collected" && holes[rule.When.Target] {
			if increment != 1 || !closed {
				t.Errorf("%s must stitch exactly one hole and retain its closed-patch art", rule.ID)
			}
			stitches[rule.When.Target] = true
		}
		if ends {
			if rule.When.Event != "variable_changed" {
				t.Errorf("%s: score must never end Ribbon", rule.ID)
			}
			six := false
			for _, c := range rule.Conditions {
				if c.Variable == "holes_stitched" && c.Operator == "eq" && c.Value == float64(6) {
					six = true
				}
			}
			if !six {
				t.Errorf("%s: victory must require six distinct repairs", rule.ID)
			}
			hasWin = true
		}
		if strings.Contains(rule.ID, "shard") && increment != 0 {
			t.Errorf("%s must never advance repair progress", rule.ID)
		}
	}
	if len(stitches) != 6 || !hasWin {
		t.Errorf("six unique stitch rules and a six-repair win rule are required; stitched=%d win=%v", len(stitches), hasWin)
	}
}

// Bobo's questionnaire contract keeps regular-heart progress separate from bonus score.
func TestBoboRegularHeartsAloneRestoreRainbow(t *testing.T) {
	dir := filepath.Join("..", "..", "inventory", "13ui1k")
	doc, err := readGameIR(dir)
	if err != nil {
		t.Fatalf("Bobo IR must validate structurally: %v", err)
	}
	for _, entity := range doc.Scenes[0].Entities {
		if entity.ID == "player" && entity.Character != nil {
			t.Fatal("Bobo is a horizontal catcher; a platformer character controller blocks its destination movement")
		}
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
		ID   string `json:"id"`
		When struct {
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
	goldBonus := false
	for _, a := range gold.Actions {
		if a.Type == "add_score" {
			if value, ok := a.Value.(float64); ok && value == 50 {
				goldBonus = true
			}
		}
	}
	if !goldBonus {
		t.Error("golden hearts must retain their optional +50 bonus score")
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
	if win.When.Event != "variable_changed" || !winAtTwelve || !endGame {
		t.Error("Bobo must win only when hearts_collected reaches exactly 12")
	}
}

// All three supplied penguin drawings must be used as progression skins:
// rainbow at start, holiday at four hearts, and classic blue at eight.
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
		if rule.When.Event != "variable_changed" {
			continue
		}
		threshold := 0
		for _, condition := range rule.Conditions {
			if condition.Variable == "hearts_collected" && condition.Operator == "eq" {
				if value, ok := condition.Value.(float64); ok {
					threshold = int(value)
				}
			}
		}
		for _, action := range rule.Actions {
			if action.Type == "set_asset" && action.Target == "penguin" {
				unlocks[threshold] = action.AssetID
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

func TestPenguinPortraitLoopAndVisibleAurora(t *testing.T) {
	doc, err := readGameIR(filepath.Join("..", "..", "inventory", "p3ngui"))
	if err != nil {
		t.Fatal(err)
	}
	if doc.Game.Orientation != "portrait" || doc.Experience.Objective.Target != 12 {
		t.Fatal("Penguin must remain a portrait twelve-heart quest")
	}
	pickups := 0
	for _, entity := range doc.Scenes[0].Entities {
		if entity.ID == "penguin" && entity.Character != nil {
			t.Fatal("a platformer controller must not block the horizontal catcher")
		}
		if entity.Kind == "pickup" {
			pickups++
			if entity.Properties["recycle"] != true || entity.Physics == nil || !entity.Physics.Sensor {
				t.Fatalf("%s must recycle after misses and remain a non-blocking collectible", entity.ID)
			}
		}
	}
	if pickups != 12 {
		t.Fatalf("expected twelve authored heart pickups, got %d", pickups)
	}
	restored := map[int]bool{}
	for _, rule := range doc.Rules {
		if rule.When.Event != "variable_changed" {
			continue
		}
		threshold := 0
		for _, condition := range rule.Conditions {
			if condition.Variable == "hearts_collected" && condition.Operator == "eq" {
				if value, ok := condition.Value.(float64); ok {
					threshold = int(value)
				}
			}
		}
		for _, action := range rule.Actions {
			if action.Type == "set_alpha" && strings.HasPrefix(action.Target, "band") {
				if alpha, ok := action.Value.(float64); ok && alpha == 1 {
					restored[threshold] = true
				}
			}
		}
	}
	for _, score := range []int{2, 4, 6, 8, 10, 12} {
		if !restored[score] {
			t.Errorf("%d catches must restore a visibly bright aurora band", score)
		}
	}
}

func TestPenguinBonusScoringNeverShortCircuitsAuroraVictory(t *testing.T) {
	doc, err := readGameIR(filepath.Join("..", "..", "inventory", "p3ngui"))
	if err != nil {
		t.Fatal(err)
	}
	if doc.Experience.Objective.ProgressVariable != "hearts_collected" {
		t.Fatal("heart progress must not use bonus score")
	}
	goldHearts := map[string]bool{}
	for _, scene := range doc.Scenes {
		for _, entity := range scene.Entities {
			if entity.Kind == "pickup" && entity.Render != nil && entity.Render.Tint != "" {
				goldHearts[entity.ID] = true
			}
		}
	}
	if len(goldHearts) != 2 {
		t.Errorf("expected two visibly gold heart pickups, got %d", len(goldHearts))
	}
	collected := 0
	for _, rule := range doc.Rules {
		if rule.When.Event == "collected" && strings.HasPrefix(rule.When.Target, "heart") {
			collected++
			score := 0.0
			progress := 0.0
			for _, action := range rule.Actions {
				if action.Type == "add_score" {
					if v, ok := action.Value.(float64); ok {
						score = v
					}
				}
				if action.Type == "add_variable" && action.Variable == "hearts_collected" {
					if v, ok := action.Value.(float64); ok {
						progress = v
					}
				}
			}
			want := 10.0
			if goldHearts[rule.When.Target] {
				want = 30
			}
			if score != want || progress != 1 {
				t.Errorf("%s needs +%.0f score, exactly one heart, got %.0f score/%.0f hearts", rule.ID, want, score, progress)
			}
		}
		for _, action := range rule.Actions {
			if action.Type == "end_game" && action.Value == true {
				valid := false
				for _, condition := range rule.Conditions {
					if condition.Variable == "hearts_collected" && condition.Operator == "eq" && condition.Value == float64(12) {
						valid = true
					}
				}
				if !valid {
					t.Errorf("%s cannot win from raw score", rule.ID)
				}
			}
		}
	}
	if collected != 12 {
		t.Errorf("expected 12 collected-heart rules, got %d", collected)
	}
}

func TestEscapeQuestionnaireGapAndCaughtEnding(t *testing.T) {
	dir := filepath.Join("..", "..", "inventory", "2uowg9")
	doc, err := readGameIR(dir)
	if err != nil {
		t.Fatal(err)
	}
	if doc == nil || doc.Experience == nil {
		t.Fatal("Escape must have a canonical experience")
	}
	if doc.Experience.Controls.Scheme != "lane_tap" {
		t.Fatalf("Escape must use discrete lane taps, got %q", doc.Experience.Controls.Scheme)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "game-ir.json"))
	if err != nil {
		t.Fatal(err)
	}
	var ir map[string]any
	if err := json.Unmarshal(raw, &ir); err != nil {
		t.Fatal(err)
	}
	exp := ir["experience"].(map[string]any)
	gap := exp["chaserGap"].(map[string]any)
	for field, want := range map[string]float64{
		"start":             220,
		"max":               250,
		"caughtAt":          55,
		"recoveryPerSecond": 6,
	} {
		if got := gap[field].(float64); got != want {
			t.Errorf("chaser gap %s = %v, want %v", field, got, want)
		}
	}
	seenCaught := false
	seenInevitableCatch := false
	for _, item := range ir["rules"].([]any) {
		rule := item.(map[string]any)
		id := rule["id"].(string)
		if id == "inevitable-catch" {
			when := rule["when"].(map[string]any)
			if when["event"] != "timer_elapsed" || when["value"] != float64(1200) {
				t.Error("Escape must reach its inevitable caught chapter at 20 minutes")
			}
		}
		for _, actionItem := range rule["actions"].([]any) {
			action := actionItem.(map[string]any)
			if id == "inevitable-catch" && action["type"] == "set_variable" &&
				action["variable"] == "chaser_gap" && action["value"] == float64(55) {
				seenInevitableCatch = true
			}
			if action["type"] == "change_health" {
				t.Errorf("legacy health damage in %s, gap must be authoritative", id)
			}
			if action["type"] == "end_game" {
				if action["value"] == true {
					t.Errorf("rule %s invents a winning escape", id)
				}
				if id == "gap-caught" && action["value"] == false {
					seenCaught = true
				}
			}
		}
	}
	if !seenCaught {
		t.Error("Escape needs an explicit caught ending")
	}
	if !seenInevitableCatch {
		t.Error("Escape needs the bounded 20-minute terminal catch")
	}
}
