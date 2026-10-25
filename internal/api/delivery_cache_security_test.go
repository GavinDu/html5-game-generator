package api

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"

	"doodlequest/internal/gameir"
	"doodlequest/internal/model"
	"doodlequest/internal/pipeline"
	"doodlequest/internal/store"
)

func TestAssetDeliveryRechecksVisibilityBeforeConditionalResponse(t *testing.T) {
	st := store.NewMemoryStore()
	srv := New(st, pipeline.NewRunner(st))
	game := seedReadyGame(st, "asset-visibility-game")
	game.ModerationStatus = "approved"

	assetBytes := []byte("safe public asset")
	sum := sha256.Sum256(assetBytes)
	game.GameIR.Assets = append(game.GameIR.Assets, gameir.Asset{
		ID: "sprite", Kind: "image", URI: "assets/sprite.png",
		MIMEType: "image/png", SHA256: hex.EncodeToString(sum[:]),
	})
	st.UpdateGame(game)
	st.PutArtifact(game.ID, "assets/sprite.png", assetBytes, "image/png")

	request := func(user, etag string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/v1/games/asset-visibility-game/assets/sprite", nil)
		if user != "" {
			req.Header.Set("X-User-ID", user)
		}
		if etag != "" {
			req.Header.Set("If-None-Match", etag)
		}
		rec := httptest.NewRecorder()
		srv.Routes().ServeHTTP(rec, req)
		return rec
	}

	public := request("", "")
	if public.Code != http.StatusOK {
		t.Fatalf("initial public asset: got %d, want 200: %s", public.Code, public.Body.String())
	}
	etag := public.Header().Get("ETag")
	if etag == "" {
		t.Fatal("public asset response did not include ETag")
	}

	updated, ok := st.GetGame(game.ID)
	if !ok {
		t.Fatal("game disappeared")
	}
	updated.Visibility = model.VisibilityPrivate
	st.UpdateGame(updated)

	denied := request("stranger", etag)
	if denied.Code != http.StatusNotFound {
		t.Fatalf("private asset with stale ETag: got %d, want 404", denied.Code)
	}
	if got := denied.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("denied response Cache-Control = %q, want private, no-store", got)
	}

	owner := request("parent-1", etag)
	if owner.Code != http.StatusOK {
		t.Fatalf("owner preview with stale ETag: got %d, want 200", owner.Code)
	}
	if got := owner.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("owner response Cache-Control = %q, want private, no-store", got)
	}
}
