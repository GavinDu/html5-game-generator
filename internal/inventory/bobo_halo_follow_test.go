package inventory

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"doodlequest/internal/gameir"
)

func TestBoboGroundHaloFollowsPhysicalBearWithoutChangingAuthoredCollider(t *testing.T) {
	raw, err := os.ReadFile("../../inventory/13ui1k/game-ir.json")
	if err != nil { t.Fatal(err) }
	doc, err := gameir.DecodeAndValidate(raw)
	if err != nil { t.Fatalf("canonical Bobo IR must be valid: %v", err) }
	var halo, bear *gameir.Entity
	for i := range doc.Scenes {
		for j := range doc.Scenes[i].Entities {
			e := &doc.Scenes[i].Entities[j]
			if e.ID == "halo-bear" { halo = e }
			if e.ID == "player" { bear = e }
		}
	}
	if halo == nil || halo.Behavior == nil || halo.Behavior.Type != "follow" ||
		halo.Behavior.Target != "player" || halo.Physics != nil {
		t.Fatalf("halo must be a physics-free visual attachment: %+v", halo)
	}
	if bear == nil || len(bear.Size) != 2 || bear.Size[0] != 92 || bear.Size[1] != 104 ||
		bear.Render == nil || len(bear.Render.Scale) != 2 ||
		bear.Render.Scale[0] != 1.28 {
		t.Fatalf("creator's original bear collider/scale must remain unchanged: %+v", bear)
	}
	manifestBytes, err := os.ReadFile("../../inventory/13ui1k/manifest.json")
	if err != nil { t.Fatal(err) }
	var manifest struct { Version int `json:"version"` }
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil { t.Fatal(err) }
	if manifest.Version < 5 {
		t.Fatalf("new visual follow behavior must invalidate old preview version: got %d", manifest.Version)
	}
}

func TestFollowBehaviorRejectsMissingAndInvalidTargets(t *testing.T) {
	raw, err := os.ReadFile("../../inventory/13ui1k/game-ir.json")
	if err != nil { t.Fatal(err) }
	var valid map[string]any
	if err := json.Unmarshal(raw, &valid); err != nil { t.Fatal(err) }
	scenes := valid["scenes"].([]any)
	entities := scenes[0].(map[string]any)["entities"].([]any)
	var halo map[string]any
	for _, anyEntity := range entities {
		e := anyEntity.(map[string]any)
		if e["id"] == "halo-bear" { halo = e; break }
	}
	if halo == nil { t.Fatal("missing Bobo halo fixture") }
	for _, tc := range []struct { target string; want string }{
		{"missing-character", "follows missing entity"},
		{"halo-bear", "needs a different valid target"},
		{"", "needs a different valid target"},
	} {
		t.Run(tc.target, func(t *testing.T) {
			halo["behavior"] = map[string]any{"type":"follow","target":tc.target}
			bytes, err := json.Marshal(valid)
			if err != nil { t.Fatal(err) }
			_, err = gameir.DecodeAndValidate(bytes)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("target %q: got %v, want %q", tc.target, err, tc.want)
			}
		})
	}
}
