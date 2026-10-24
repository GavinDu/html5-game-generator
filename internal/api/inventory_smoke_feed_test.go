package api

import (
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"doodlequest/internal/inventory"
	"doodlequest/internal/model"
	"doodlequest/internal/pipeline"
	"doodlequest/internal/store"
)

const smokeInventoryGameID = "sample-rainbow-bear"

func TestInventorySmokeGameSurvivesRestartAndIsPlayableFromForYou(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "store.json")
	loadAppState := func() *store.MemoryStore {
		t.Helper()
		st := store.NewMemoryStore()
		if err := st.EnablePersistence(statePath); err != nil {
			t.Fatalf("enable persisted store: %v", err)
		}
		if err := inventory.Load(st, filepath.Join("..", "..", "inventory")); err != nil {
			t.Fatalf("load shipped inventory: %v", err)
		}
		return st
	}

	first := store.NewMemoryStore()
	if err := first.EnablePersistence(statePath); err != nil {
		t.Fatalf("enable persisted store: %v", err)
	}
	first.CreateGame(&model.Game{
		ID: "user-private-draft", Title: "Private family draft", OwnerID: "test-user",
		Status: model.GameReady, Visibility: model.VisibilityPrivate,
	})
	if err := inventory.Load(first, filepath.Join("..", "..", "inventory")); err != nil {
		t.Fatalf("load shipped inventory: %v", err)
	}

	// A new store restores the persisted snapshot, then follows app startup
	// order by reloading the shipped inventory over it.
	restarted := loadAppState()
	testGameCount := 0
	for _, game := range restarted.ListGames() {
		if game.ID == smokeInventoryGameID {
			testGameCount++
		}
	}
	if testGameCount != 1 {
		t.Fatalf("expected exactly one deterministic smoke game after restart; got %d", testGameCount)
	}
	userGame, ok := restarted.GetGame("user-private-draft")
	if !ok || userGame.OwnerID != "test-user" || userGame.Title != "Private family draft" ||
		userGame.Visibility != model.VisibilityPrivate {
		t.Fatalf("inventory seeding changed or lost private user content: %#v", userGame)
	}

	game, ok := restarted.GetGame(smokeInventoryGameID)
	if !ok || game.OwnerID != "doodlequest" || game.Status != model.GameReady ||
		game.Visibility != model.VisibilityPublic || game.GameIR == nil ||
		game.GameIR.Game.Audience != "family" {
		t.Fatalf("smoke game is not a public, validated family game: %#v", game)
	}

	srv := New(restarted, pipeline.NewRunner(restarted))
	feedRec := do(t, srv, http.MethodGet, "/v1/feed?scope=for_you", nil, "smoke-viewer")
	if feedRec.Code != http.StatusOK {
		t.Fatalf("For You feed: %d %s", feedRec.Code, feedRec.Body.String())
	}
	var feed struct {
		Games []model.Game `json:"games"`
	}
	decode(t, feedRec, &feed)
	found := false
	for _, candidate := range feed.Games {
		if candidate.ID == smokeInventoryGameID {
			found = true
		}
		if candidate.ID == "user-private-draft" {
			t.Fatal("private user content leaked into For You")
		}
	}
	if !found {
		t.Fatalf("For You feed does not contain the shipped smoke game: %s", feedRec.Body.String())
	}

	packageRec := do(t, srv, http.MethodGet, "/v1/games/"+smokeInventoryGameID+"/package", nil, "smoke-viewer")
	if packageRec.Code != http.StatusOK {
		t.Fatalf("game package: %d %s", packageRec.Code, packageRec.Body.String())
	}
	var pkg GamePackage
	decode(t, packageRec, &pkg)
	if pkg.GameID != smokeInventoryGameID || pkg.IR == nil || len(pkg.Assets) == 0 {
		t.Fatalf("invalid smoke-game package: %+v", pkg)
	}
	for _, asset := range pkg.Assets {
		assetRec := do(t, srv, http.MethodGet, asset.URL, nil, "smoke-viewer")
		if assetRec.Code != http.StatusOK || assetRec.Body.Len() == 0 {
			t.Fatalf("asset %q unavailable at %s: %d %s", asset.ID, asset.URL, assetRec.Code, assetRec.Body.String())
		}
	}

	playerRec := do(t, srv, http.MethodGet, "/player?game="+smokeInventoryGameID, nil, "smoke-viewer")
	if playerRec.Code != http.StatusOK || !strings.Contains(playerRec.Body.String(), "/web/pixi-runtime.js") {
		t.Fatalf("player shell: %d %s", playerRec.Code, playerRec.Body.String())
	}
	runtimeRec := do(t, srv, http.MethodGet, "/web/pixi-runtime.js", nil, "smoke-viewer")
	if runtimeRec.Code != http.StatusOK || !strings.Contains(runtimeRec.Body.String(), "/package") {
		t.Fatalf("player runtime: %d", runtimeRec.Code)
	}
}
