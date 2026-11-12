package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"strings"
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
		t.Errorf("irHash does not identify the delivered IR's compatibility key: got %q", pkg.IRHash)
	}
	if len(pkg.IRSHA256) != 64 || pkg.IRSHA256 != irSHA256Of(pkg.IR) {
		t.Errorf("irSha256 does not bind the complete delivered IR: got %q", pkg.IRSHA256)
	}
	if len(pkg.IRSHA256) == 64 && pkg.IRHash != pkg.IRSHA256[:12] {
		t.Errorf("legacy short irHash must remain a prefix of full irSha256")
	}
	if pkg.Assets == nil {
		t.Error("assets must be an array, not null, even when the package has no assets")
	}
	for _, asset := range pkg.Assets {
		if len(asset.SHA256) != 64 {
			t.Errorf("asset %q has no SHA-256 digest in the package envelope: %q", asset.ID, asset.SHA256)
		}
	}
}

func TestPackageAssetSHA256ComputesLegacyDigestAndRejectsMismatch(t *testing.T) {
	data := []byte("verified sprite bytes")
	sum := sha256.Sum256(data)
	want := hex.EncodeToString(sum[:])

	got, err := packageAssetSHA256("", data)
	if err != nil || got != want {
		t.Fatalf("computed digest = %q, %v; want %q", got, err, want)
	}
	got, err = packageAssetSHA256(strings.ToUpper(want), data)
	if err != nil || got != want {
		t.Fatalf("matching declared digest = %q, %v; want %q", got, err, want)
	}
	if _, err := packageAssetSHA256(strings.Repeat("0", 64), data); err == nil {
		t.Fatal("mismatched declared digest must fail closed")
	}
}


// TestExportCanonicalPackageForSwift is an opt-in bridge fixture producer for
// GitHub Actions. The JSON is emitted by the actual Go HTTP handler, with the
// real Go struct serialization and digest semantics, rather than being a
// handcrafted Swift test package. Normal Go test runs do not modify files.
func TestExportCanonicalPackageForSwift(t *testing.T) {
	path := os.Getenv("EXPORT_SWIFT_PACKAGE_FIXTURE")
	if path == "" {
	    t.Skip("set EXPORT_SWIFT_PACKAGE_FIXTURE to validate Go->Swift delivery compatibility")
	}
	st := store.NewMemoryStore()
	srv := New(st, pipeline.NewRunner(st))
	game := seedReadyGame(st, "runtime-contract-game")
	game.ModerationStatus = "approved"
	st.UpdateGame(game)

	rec := getAs(t, srv, "/v1/games/runtime-contract-game/package", "")
	if rec.Code != http.StatusOK {
	    t.Fatalf("fixture package HTTP %d: %s", rec.Code, rec.Body.String())
	}
	if err := os.WriteFile(path, rec.Body.Bytes(), 0600); err != nil {
	    t.Fatalf("write Go-emitted package fixture: %v", err)
	}
}
