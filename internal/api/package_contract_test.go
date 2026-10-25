package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"doodlequest/internal/pipeline"
	"doodlequest/internal/store"
)

func TestPackageEnvelopeAdvertisesRuntimeCompatibilityContract(t *testing.T) {
	st := store.NewMemoryStore()
	srv := New(st, pipeline.NewRunner(st))
	game := seedReadyGame(st, "runtime-contract-game")
	game.ModerationStatus = "approved"
	st.UpdateGame(game)

	rec := getAs(t, srv, "/v1/games/runtime-contract-game/package", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("package response: got %d, want 200: %s", rec.Code, rec.Body.String())
	}

	var pkg GamePackage
	if err := json.Unmarshal(rec.Body.Bytes(), &pkg); err != nil {
		t.Fatalf("decode package envelope: %v", err)
	}
	if pkg.PackageFormatVersion != PackageFormatVersion {
		t.Errorf("packageFormatVersion = %q, want %q", pkg.PackageFormatVersion, PackageFormatVersion)
	}
	if pkg.MinRuntimeVersion != MinRuntimeVersion {
		t.Errorf("minRuntimeVersion = %q, want %q", pkg.MinRuntimeVersion, MinRuntimeVersion)
	}
	if pkg.GameID != game.ID {
		t.Errorf("gameId = %q, want %q", pkg.GameID, game.ID)
	}
	if pkg.IR == nil {
		t.Fatal("package envelope omitted IR")
	}
	if pkg.IRHash == "" || pkg.IRHash != irHashOf(pkg.IR) {
		t.Errorf("irHash does not identify the exact delivered IR: got %q", pkg.IRHash)
	}
	if pkg.Assets == nil {
		t.Error("assets must be an array, not null, even when the package has no assets")
	}
}
