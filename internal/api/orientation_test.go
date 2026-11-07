package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"doodlequest/internal/inventory"
	"doodlequest/internal/pipeline"
	"doodlequest/internal/store"
)

func TestFeedAndPackageExposeCanonicalOrientation(t *testing.T) {
	st := store.NewMemoryStore()
	if err := inventory.Load(st, "../../inventory"); err != nil {
		t.Fatal(err)
	}
	srv := New(st, pipeline.NewRunner(st))
	game, _ := st.GetGame("penguin-heartlight-rescue")
	for _, orientation := range []string{"portrait", "landscape", "any"} {
		game.GameIR.Game.Orientation = orientation
		st.UpdateGame(game)
		rec := do(t, srv, http.MethodGet, "/v1/games/"+game.ID+"/package", nil, "")
		if rec.Code != http.StatusOK {
			t.Fatal(rec.Body.String())
		}
		var pkg GamePackage
		if err := json.Unmarshal(rec.Body.Bytes(), &pkg); err != nil {
			t.Fatal(err)
		}
		if pkg.Orientation != orientation || pkg.IR.Game.Orientation != orientation {
			t.Fatalf("package lost canonical %s orientation", orientation)
		}
		rec = do(t, srv, http.MethodGet, "/v1/feed?scope=for_you&limit=20", nil, "")
		var feed struct {
			Cards []feedCard `json:"cards"`
		}
		if rec.Code != http.StatusOK {
			t.Fatal(rec.Body.String())
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &feed); err != nil {
			t.Fatal(err)
		}
		found := false
		for _, card := range feed.Cards {
			if card.GameID == game.ID {
				found = true
				if card.Orientation != orientation {
					t.Fatalf("feed orientation = %s, want %s", card.Orientation, orientation)
				}
			}
		}
		if !found {
			t.Fatal("penguin card missing")
		}
	}
}
