package api

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"

	"doodlequest/internal/inventory"
	"doodlequest/internal/model"
)

func TestCreateRemixUnsupportedParentReturnsConflictBeforeQueueing(t *testing.T) {
	srv := newTestServer()
	srv.store.CreateGame(&model.Game{
		ID:       "sample-ribbon-sky",
		Title:    "Ribbon Sky",
		Status:   model.GameReady,
		Manifest: &model.BundleManifest{GameID: "sample-ribbon-sky"},
		Elements: []model.Element{{ID: "el_1", Label: "ribbon", Kind: "character"}},
	})

	rec := do(t, srv, http.MethodPost, "/v1/games", map[string]any{
		"parent_game_id": "sample-ribbon-sky",
	}, "remix-user")
	if rec.Code != http.StatusConflict {
		t.Fatalf("unsupported remix status = %d, want 409: %s", rec.Code, rec.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if body["error"] != "remix_unsupported" {
		t.Fatalf("error code = %q, want remix_unsupported", body["error"])
	}
	if body["message"] == "" {
		t.Fatal("expected a user-readable explanation")
	}
}

func TestCreateRemixWithValidatedInventoryGameIsAccepted(t *testing.T) {
	srv := newTestServer()
	if err := inventory.Load(srv.store, filepath.Join("..", "..", "inventory")); err != nil {
		t.Fatalf("load shipped inventory: %v", err)
	}
	parent, ok := srv.store.GetGame("sample-ribbon-sky")
	if !ok {
		t.Fatal("sample-ribbon-sky inventory game missing")
	}
	if parent.GameIR == nil || parent.Manifest == nil {
		t.Fatal("sample-ribbon-sky must have a canonical Game IR and manifest")
	}

	rec := do(t, srv, http.MethodPost, "/v1/games", map[string]any{
		"parent_game_id": parent.ID,
		"intent":         "Make the ribbon adventure a little brighter",
	}, "remix-user")
	if rec.Code != http.StatusAccepted {
		t.Fatalf("supported inventory remix status = %d, want 202: %s", rec.Code, rec.Body.String())
	}
}
