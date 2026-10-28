package api

import (
	"encoding/json"
	"net/http"
	"testing"

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
