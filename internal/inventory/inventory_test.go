package inventory

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"doodlequest/internal/store"
)

func writeGame(t *testing.T, root, dir, manifest string, files map[string]string) {
	t.Helper()
	gd := filepath.Join(root, dir)
	if err := os.MkdirAll(filepath.Join(gd, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gd, "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(gd, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

const testManifest = `{
  "game_id": "test-game", "title": "Test", "owner": "tester",
  "intent": "testing", "visibility": "public", "version": 1,
  "shim": "gameshim/v1", "entry": "game.js", "controls": ["tap"],
  "elements": [{"id":"el_1","label":"x","kind":"prop","description":"x"}],
  "music": {"src": "audio/theme.wav", "mood": "calm", "source": "library"},
  "sprites": [
    {"id":"s1","src":"sprites/s1.png","mime":"image/png","source":"test","label":"s1","file":"assets/s1.png.b64"},
    {"id":"heart","src":"sprites/heart.png","mime":"image/png","source":"procedural","label":"h","procedural":"heart:#EC70A8"},
    {"id":"band3","src":"sprites/band3.png","mime":"image/png","source":"procedural","label":"b","procedural":"band:3"}
  ]
}`

func TestLoad(t *testing.T) {
	root := t.TempDir()
	b64 := base64.StdEncoding.EncodeToString([]byte("fakepng"))
	writeGame(t, root, "abc123", testManifest, map[string]string{
		"game.js":           "var x = 1;",
		"assets/s1.png.b64": b64,
	})
	// Non-conforming dir names are ignored.
	if err := os.Mkdir(filepath.Join(root, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}

	st := store.NewMemoryStore()
	if err := Load(st, root); err != nil {
		t.Fatalf("Load: %v", err)
	}
	g, ok := st.GetGame("test-game")
	if !ok {
		t.Fatal("game not seeded")
	}
	if g.Title != "Test" || g.Manifest == nil || len(g.Manifest.Assets.Sprites) != 3 {
		t.Fatalf("bad game record: %+v", g)
	}
	for _, want := range []string{"game.js", "sprites/s1.png", "sprites/heart.png", "sprites/band3.png", "audio/theme.wav"} {
		if _, _, ok := st.GetArtifact("test-game", want); !ok {
			t.Fatalf("missing artifact %s", want)
		}
	}
	if data, _, _ := st.GetArtifact("test-game", "sprites/s1.png"); string(data) != "fakepng" {
		t.Fatal("b64 sprite not decoded")
	}

	// Idempotent: second load keeps the game, no duplicates.
	if err := Load(st, root); err != nil {
		t.Fatalf("second Load: %v", err)
	}
}

func TestLoadDuplicateGameID(t *testing.T) {
	root := t.TempDir()
	writeGame(t, root, "abc123", testManifest, map[string]string{"game.js": "1"})
	writeGame(t, root, "def456", testManifest, map[string]string{"game.js": "2"})
	if err := Load(storet(t), root); err == nil {
		t.Fatal("expected duplicate game_id error")
	}
}

func TestLoadBadProcedural(t *testing.T) {
	root := t.TempDir()
	bad := `{"game_id":"bad","title":"b","owner":"o","intent":"i","visibility":"public",
	  "version":1,"shim":"s","entry":"game.js","controls":["tap"],"elements":[],
	  "music":{"src":"audio/theme.wav","mood":"calm","source":"library"},
	  "sprites":[{"id":"x","src":"sprites/x.png","mime":"image/png","source":"p","label":"x","procedural":"nope"}]}`
	writeGame(t, root, "abc123", bad, map[string]string{"game.js": "1"})
	if err := Load(storet(t), root); err == nil {
		t.Fatal("expected unknown procedural error")
	}
}

func TestLoadNoFolders(t *testing.T) {
	if err := Load(storet(t), t.TempDir()); err == nil {
		t.Fatal("expected no-game-folders error")
	}
}

const testIR = `{
  "irVersion": "1.0",
  "game": {"id": "test-game", "title": "Test", "entryScene": "main", "orientation": "portrait"},
  "assets": [{"id": "s1", "kind": "image", "uri": "sprites/s1.png", "source": "generated"}],
  "scenes": [{"id": "main", "entities": [
    {"id": "player", "kind": "player", "assetId": "s1", "position": [10, 10], "components": ["movement"]}
  ]}],
  "inputBindings": [{"action": "move", "input": "tap"}],
  "rules": [{"id": "start", "when": {"event": "game_start"},
    "actions": [{"type": "show_text", "value": "go"}]}]
}`

// A game folder carrying game-ir.json seeds the canonical IR onto the game
// record, so the package endpoints can serve it.
func TestLoadSeedsGameIR(t *testing.T) {
	root := t.TempDir()
	b64 := base64.StdEncoding.EncodeToString([]byte("fakepng"))
	writeGame(t, root, "abc123", testManifest, map[string]string{
		"game.js":           "var x = 1;",
		"assets/s1.png.b64": b64,
		"game-ir.json":      testIR,
	})
	st := store.NewMemoryStore()
	if err := Load(st, root); err != nil {
		t.Fatalf("Load: %v", err)
	}
	g, ok := st.GetGame("test-game")
	if !ok || g.GameIR == nil {
		t.Fatal("game-ir.json was not seeded onto the game record")
	}
	if g.GameIR.Game.ID != "test-game" || len(g.GameIR.Assets) != 1 {
		t.Fatalf("bad seeded IR: %+v", g.GameIR.Game)
	}
}

// A present-but-broken game-ir.json fails the load: an invalid IR must not
// silently degrade to a runtime 409.
func TestLoadRejectsBadGameIR(t *testing.T) {
	for name, ir := range map[string]string{
		"not json":      `{`,
		"wrong game id": strings.Replace(testIR, `"test-game"`, `"other-game"`, 1),
	} {
		root := t.TempDir()
		b64 := base64.StdEncoding.EncodeToString([]byte("fakepng"))
		writeGame(t, root, "abc123", testManifest, map[string]string{
			"game.js":           "var x = 1;",
			"assets/s1.png.b64": b64,
			"game-ir.json":      ir,
		})
		if err := Load(store.NewMemoryStore(), root); err == nil {
			t.Fatalf("%s: expected load error", name)
		}
	}
}

func storet(t *testing.T) *store.MemoryStore {
	t.Helper()
	return store.NewMemoryStore()
}
