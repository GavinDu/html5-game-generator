package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"doodlequest/internal/gameir"
	"doodlequest/internal/pipeline"
	"doodlequest/internal/store"
)

func TestPackageAssetBudgetLimitsAreFailClosed(t *testing.T) {
	cases := []struct {
		name  string
		size  int
		total int64
		want  bool
	}{
		{"empty", 0, 0, true},
		{"single_at_limit", maxPackageAssetBytes, 0, true},
		{"single_oversized", maxPackageAssetBytes + 1, 0, false},
		{"negative_size", -1, 0, false},
		{"negative_total", 1, -1, false},
		{"aggregate_at_limit", maxPackageAssetBytes, maxPackageAggregateBytes - maxPackageAssetBytes, true},
		{"aggregate_over_limit", maxPackageAssetBytes, maxPackageAggregateBytes - maxPackageAssetBytes + 1, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := packageAssetBudgetAllows(tc.size, tc.total); got != tc.want {
				t.Errorf("allows(%d, %d) = %t, want %t", tc.size, tc.total, got, tc.want)
			}
		})
	}
}

func TestPackageAdvertisesByteLengthAndRejectsOversizedStoredImage(t *testing.T) {
	st := store.NewMemoryStore()
	srv := New(st, pipeline.NewRunner(st))
	game := seedReadyGame(st, "budget-fixture-game")
	game.ModerationStatus = "approved"
	const uri = "assets/runtime-budget-image.png"
	const id = "runtime-budget-image"
	game.GameIR.Assets = append(game.GameIR.Assets, gameir.Asset{
		ID: id, Kind: "image", URI: uri, MIMEType: "image/png",
	})
	st.UpdateGame(game)
	st.PutArtifact(game.ID, uri, []byte("small confirmed content"), "image/png")

	rec := getAs(t, srv, "/v1/games/budget-fixture-game/package", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("valid package HTTP %d: %s", rec.Code, rec.Body.String())
	}
	var pkg GamePackage
	if err := json.Unmarshal(rec.Body.Bytes(), &pkg); err != nil {
		t.Fatalf("decode package: %v", err)
	}
	found := false
	for _, asset := range pkg.Assets {
		if asset.ID == id {
			found = true
			if asset.ByteLength != len("small confirmed content") {
				t.Errorf("delivered byteLength = %d, want exact stored byte length", asset.ByteLength)
			}
		}
	}
	if !found {
		t.Fatal("test asset missing from package delivery")
	}

	st.PutArtifact(game.ID, uri, make([]byte, maxPackageAssetBytes+1), "image/png")
	oversized := getAs(t, srv, "/v1/games/budget-fixture-game/package", "")
	if oversized.Code != http.StatusUnprocessableEntity {
		t.Fatalf("oversized package HTTP %d, want 422: %s", oversized.Code, oversized.Body.String())
	}
	if oversized.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("oversized package must never be shared cached")
	}
	asset := getAs(t, srv, "/v1/games/budget-fixture-game/assets/runtime-budget-image", "")
	if asset.Code != http.StatusUnprocessableEntity {
		t.Fatalf("oversized asset HTTP %d, want 422: %s", asset.Code, asset.Body.String())
	}
}
