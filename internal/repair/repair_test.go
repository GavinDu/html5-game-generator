package repair

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"strings"
	"testing"

	"doodlequest/internal/gameir"
)

func f64(v float64) *float64 { return &v }

func baseDoc() *gameir.Document {
	return &gameir.Document{
		IRVersion: "1.1",
		Game:      gameir.Game{ID: "g1", Title: "Test"},
		Assets: []gameir.Asset{
			{ID: "hero", Kind: "image", URI: "hero"},
			{ID: "theme", Kind: "audio", URI: "theme"},
		},
		Scenes: []gameir.Scene{{
			ID:        "s1",
			WorldSize: []float64{390, 844},
			Gravity:   []float64{0, 99999}, // out of range on purpose
			Entities: []gameir.Entity{
				{ID: "player", Kind: "player", AssetID: "hero", Position: []float64{100, 100},
					Physics: &gameir.Physics{Body: "dynamic", Density: f64(500), Friction: f64(-3), Restitution: f64(7)}},
				{ID: "coin", Kind: "pickup", AssetID: "ghost-asset", Position: []float64{5000, -400}},
			},
		}},
		Rules: []gameir.Rule{{
			ID:   "coin-caught",
			When: gameir.Condition{Event: "collected", Source: "player", Target: "coin"},
			Actions: []gameir.Action{
				{Type: "add_score", Value: float64(3)},
				{Type: "play_sound", AssetID: "missing-sound"},
			},
		}},
	}
}

func TestClampPhysics(t *testing.T) {
	res := RepairDeterministic(baseDoc(), nil)
	sc := res.Doc.Scenes[0]
	if sc.Gravity[1] != 5000 {
		t.Errorf("gravity: want clamped 5000, got %v", sc.Gravity[1])
	}
	ph := sc.Entities[0].Physics
	if *ph.Density != 50 || *ph.Friction != 0 || *ph.Restitution != 1 {
		t.Errorf("physics not clamped: density=%v friction=%v restitution=%v", *ph.Density, *ph.Friction, *ph.Restitution)
	}
}

func TestDropDanglingAsset(t *testing.T) {
	res := RepairDeterministic(baseDoc(), nil)
	coin := res.Doc.Scenes[0].Entities[1]
	if coin.AssetID != "" {
		t.Errorf("dangling entity asset should be cleared, got %q", coin.AssetID)
	}
	// The play_sound action with the missing sound is dropped; add_score stays.
	acts := res.Doc.Rules[0].Actions
	if len(acts) != 1 || acts[0].Type != "add_score" {
		t.Errorf("rule actions after repair: %+v", acts)
	}
	// The source document must be untouched (repair works on a copy).
	if baseDoc().Scenes[0].Entities[1].AssetID != "ghost-asset" {
		t.Error("original doc mutated")
	}
}

func TestClampPosition(t *testing.T) {
	res := RepairDeterministic(baseDoc(), nil)
	pos := res.Doc.Scenes[0].Entities[1].Position
	if pos[0] != 490 || pos[1] != -100 { // world 390x844 + margin 100
		t.Errorf("position: want [490 -100], got %v", pos)
	}
}

func TestAddWinRule(t *testing.T) {
	res := RepairDeterministic(baseDoc(), nil)
	var win *gameir.Rule
	for i := range res.Doc.Rules {
		if res.Doc.Rules[i].ID == "auto-repair-win" {
			win = &res.Doc.Rules[i]
		}
	}
	if win == nil {
		t.Fatal("no win rule added")
	}
	if win.When.Value == nil || *win.When.Value != 3 {
		t.Errorf("win threshold: want 3 (the coin's add_score), got %v", win.When.Value)
	}
	// A doc that already has a win rule is left alone.
	doc := baseDoc()
	doc.Rules = append(doc.Rules, gameir.Rule{ID: "w", Actions: []gameir.Action{{Type: "end_game", Value: true}}})
	res2 := RepairDeterministic(doc, nil)
	for _, r := range res2.Changes {
		if r.Rule == "add-win-rule" {
			t.Error("win rule added despite an existing one")
		}
	}
}

func TestReencodeImage(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	img.Set(0, 0, color.RGBA{255, 0, 0, 255})
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	res := RepairDeterministic(baseDoc(), map[string][]byte{"hero": buf.Bytes()})
	if len(res.FixedAssets["hero"]) == 0 {
		t.Fatal("jpeg hero should have been re-encoded to PNG")
	}
	if !bytes.HasPrefix(res.FixedAssets["hero"], []byte("\x89PNG")) {
		t.Error("re-encoded bytes are not PNG")
	}
	// Undecodable bytes: unfixable, never faked.
	res2 := RepairDeterministic(baseDoc(), map[string][]byte{"hero": []byte("not an image")})
	found := false
	for _, u := range res2.Unfixable {
		if strings.Contains(u, "undecodable") {
			found = true
		}
	}
	if !found {
		t.Error("undecodable image must be reported unfixable")
	}
}

func TestRunLoopStopsWhenNothingChanges(t *testing.T) {
	gate := func(doc *gameir.Document) []string { return []string{"ir.schema_valid"} }
	loop := RunLoop(baseDoc(), nil, gate)
	if loop.Attempts > MaxAttempts {
		t.Fatalf("attempts unbounded: %d", loop.Attempts)
	}
	if len(loop.Remaining) == 0 {
		t.Fatal("persistent failure must remain reported")
	}
}

func TestRunLoopCleanGate(t *testing.T) {
	// A doc with nothing repairable and a clean gate: no attempts spent.
	clean := &gameir.Document{
		IRVersion: "1.1",
		Game:      gameir.Game{ID: "g0", Title: "Clean"},
		Scenes: []gameir.Scene{{ID: "s1", Entities: []gameir.Entity{
			{ID: "player", Kind: "player", Position: []float64{10, 10}},
		}}},
	}
	gate := func(doc *gameir.Document) []string { return nil }
	loop := RunLoop(clean, nil, gate)
	if loop.Attempts != 0 || len(loop.Remaining) != 0 {
		t.Fatalf("clean gate: attempts=%d remaining=%v", loop.Attempts, loop.Remaining)
	}
	// But a schema-clean doc with repairable breakage still gets its round:
	// the gate being clean must not skip the fixes it cannot measure.
	loop2 := RunLoop(baseDoc(), nil, gate)
	if len(loop2.Result.Changes) == 0 {
		t.Fatal("clean-gate doc with repairable issues received no repair round")
	}
}
