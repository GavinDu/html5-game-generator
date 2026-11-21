package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"testing"

	"doodlequest/internal/inventory"
	"doodlequest/internal/pipeline"
	"doodlequest/internal/store"
)

// This measures every *checked-in* Game IR's actual HTTP package envelope,
// rather than treating one synthetic fixture as the largest published game.
// A checked-in inventory fixture is NOT a deployed publisher database audit.
func TestEveryCheckedInGameIRSerializedPackageHTTPByteBaseline(t *testing.T) {
	st := store.NewMemoryStore()
	if err := inventory.Load(st, "../../inventory"); err != nil {
		t.Fatalf("load actual checked-in game inventory: %v", err)
	}
	srv := New(st, pipeline.NewRunner(st))
	games := st.ListGames()
	sort.Slice(games, func(i, j int) bool {
		return games[i].ID < games[j].ID
	})

	var measured, maximumBytes int
	for _, game := range games {
		if game.GameIR == nil {
			continue // No Game IR is published from this fixture.
		}
		// This is explicitly an in-memory diagnostic public package fixture,
		// not evidence that the actual deployment's moderation is approved.
		game.ModerationStatus = "approved"
		st.UpdateGame(game)
		t.Run(game.ID, func(t *testing.T) {
			path := "/v1/games/" + url.PathEscape(game.ID) + "/package"
			rec := getAs(t, srv, path, "")
			if rec.Code != http.StatusOK {
				t.Fatalf("checked-in Game IR HTTP package status=%d; want 200", rec.Code)
			}
			var pkg GamePackage
			if err := json.Unmarshal(rec.Body.Bytes(), &pkg); err != nil {
				t.Fatalf("decode actual package HTTP body: %v", err)
			}
			if pkg.GameID != game.ID ||
				pkg.PackageFormatVersion != PackageFormatVersion ||
				pkg.MinRuntimeVersion != MinRuntimeVersion ||
				pkg.IRVersion != game.GameIR.IRVersion {
				t.Fatalf("actual package identity/version contract changed for checked-in inventory")
			}
			if pkg.IR == nil ||
				len(pkg.IRSHA256) != 64 ||
				pkg.IRSHA256 != irSHA256Of(pkg.IR) ||
				pkg.IRHash != pkg.IRSHA256[:12] {
				t.Fatal("actual HTTP package no longer binds a complete canonical IR digest")
			}
			if len(pkg.Assets) != len(game.GameIR.Assets) {
				t.Fatalf("asset manifest mismatch got=%d want=%d",
					len(pkg.Assets), len(game.GameIR.Assets))
			}
			marshalled, err := json.Marshal(pkg)
			if err != nil {
				t.Fatalf("remarshal actual Go HTTP package envelope: %v", err)
			}
			if !bytes.Equal(rec.Body.Bytes(), marshalled) {
				t.Fatal("actual 200 body is not exact Go GamePackage serialized JSON")
			}
			size := len(rec.Body.Bytes())
			if size == 0 {
				t.Fatal("empty 200 GamePackage")
			}
			measured++
			if size > maximumBytes {
				maximumBytes = size
			}
			t.Logf("checked-in public GamePackage HTTP JSON bytes=%d assets=%d irVersion=%s",
				size, len(pkg.Assets), pkg.IRVersion)
		})
	}
	if measured == 0 {
		t.Fatal("no checked-in inventory Game IR packages were measured")
	}
	t.Logf("checked-in inventory actual Go HTTP package count=%d maximum serialized JSON bytes=%d; NOT a deployed inventory limit",
		measured, maximumBytes)
	_ = fmt.Sprintf // keep compiler's format import audit explicit
}
