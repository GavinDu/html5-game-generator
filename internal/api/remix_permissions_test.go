package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"doodlequest/internal/model"
	"doodlequest/internal/pipeline"
	"doodlequest/internal/store"
)

func TestRemixSourceRightsDenyPrivateCrossAccountAndRequirePublicOptIn(t *testing.T) {
	t.Setenv("DQ_ALLOW_INSECURE_USER_ID", "true")
	t.Setenv("DQ_AUTH_HMAC_SECRET", "")
	t.Setenv("DQ_SOCIAL_STATE_FILE", filepath.Join(t.TempDir(), "social.json"))
	st := store.NewMemoryStore()
	srv := New(st, pipeline.NewRunner(st))
	now := time.Now().UTC()
	source := &model.Game{
		ID: "sample-rainbow-bear", OwnerID: "creator-one",
		Title: "Owner-only game", Status: model.GameReady,
		Visibility:  model.VisibilityPrivate,
		Elements:    []model.Element{{ID: "hero", Label: "hero", Kind: "character"}},
		Manifest:    &model.BundleManifest{GameID: "sample-rainbow-bear", Version: 1},
		PublishedAt: &now, PublishedVersion: 1, ModerationStatus: "approved",
		CreatedAt: now, UpdatedAt: now,
	}
	st.CreateGame(source)

	count := len(st.ListGames())
	denied := do(t, srv, http.MethodPost, "/v1/games",
		map[string]any{"parent_game_id": source.ID}, "creator-two")
	if denied.Code != http.StatusNotFound {
		t.Fatalf("private remix by another account got %d, want 404: %s", denied.Code, denied.Body.String())
	}
	if len(st.ListGames()) != count {
		t.Fatal("unauthorized remix created an orphan child game")
	}
	if !canRemixGame(source, "creator-one") || canRemixGame(source, "creator-two") {
		t.Fatal("private owner remix policy is incorrect")
	}

	source.Visibility = model.VisibilityPublic
	source.RemixAllowed = false
	st.UpdateGame(source)
	rejected := do(t, srv, http.MethodPost, "/v1/games",
		map[string]any{"parent_game_id": source.ID}, "creator-two")
	if rejected.Code != http.StatusForbidden {
		t.Fatalf("public but no-opt-in remix got %d, want 403: %s", rejected.Code, rejected.Body.String())
	}
	if len(st.ListGames()) != count {
		t.Fatal("non-opted-in remix queued a child")
	}

	source.RemixAllowed = true
	st.UpdateGame(source)
	if !canRemixGame(source, "creator-two") {
		t.Fatal("approved published opt-in should allow eligible remix")
	}
	accepted := do(t, srv, http.MethodPost, "/v1/games",
		map[string]any{"parent_game_id": source.ID}, "creator-two")
	if accepted.Code != http.StatusAccepted {
		t.Fatalf("approved opt-in remix got %d, want 202: %s", accepted.Code, accepted.Body.String())
	}
	if len(st.ListGames()) != count+1 {
		t.Fatalf("allowed remix created %d new game(s), want 1", len(st.ListGames())-count)
	}

	source.Quarantined = true
	if canRemixGame(source, "creator-two") {
		t.Fatal("quarantined source cannot be remixed regardless of consent")
	}
}

func TestRemixEligibilityRequiresSignedBearerAndHidesPrivateSource(t *testing.T) {
	const secret = "test-remix-eligibility-secret-long-enough"
	t.Setenv("DQ_AUTH_HMAC_SECRET", secret)
	t.Setenv("DQ_ALLOW_INSECURE_USER_ID", "false")
	t.Setenv("DQ_SOCIAL_STATE_FILE", filepath.Join(t.TempDir(), "social.json"))
	st := store.NewMemoryStore()
	srv := New(st, pipeline.NewRunner(st))
	now := time.Now().UTC()
	parent := &model.Game{
		ID: "owner-secret-source", OwnerID: "creator-one",
		Title: "Private drawing", Status: model.GameReady,
		Visibility: model.VisibilityPrivate,
		Manifest:   &model.BundleManifest{GameID: "owner-secret-source", Version: 1},
		CreatedAt:  now, UpdatedAt: now,
	}
	st.CreateGame(parent)
	request := func(user, gameID string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/v1/games/"+gameID+"/remix-eligibility", nil)
		if user != "" {
			req.Header.Set("Authorization", "Bearer "+
				authTokenForTest(user, now.Add(time.Hour), secret))
		}
		req.Header.Set("X-User-ID", "creator-one")
		rec := httptest.NewRecorder()
		srv.Routes().ServeHTTP(rec, req)
		return rec
	}
	if got := request("", parent.ID); got.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous eligibility got %d want 401: %s", got.Code, got.Body.String())
	}
	if got := request("creator-two", parent.ID); got.Code != http.StatusNotFound {
		t.Fatalf("cross-account private source got %d want 404: %s", got.Code, got.Body.String())
	}
	owner := request("creator-one", parent.ID)
	if owner.Code != http.StatusOK {
		t.Fatalf("owner eligibility got %d: %s", owner.Code, owner.Body.String())
	}
	var value map[string]any
	if err := json.Unmarshal(owner.Body.Bytes(), &value); err != nil {
		t.Fatal(err)
	}
	if value["eligible"] != true {
		t.Fatalf("owner's own ready source should be eligible: %#v", value)
	}
	if owner.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("eligibility must never be publicly cacheable")
	}
}

func TestPublishedRemixOptInIsPersistentAndNotAnApprovalShortcut(t *testing.T) {
	t.Setenv("DQ_ALLOW_INSECURE_USER_ID", "true")
	t.Setenv("DQ_AUTH_HMAC_SECRET", "")
	t.Setenv("DQ_SOCIAL_STATE_FILE", filepath.Join(t.TempDir(), "social.json"))
	st := store.NewMemoryStore()
	srv := New(st, pipeline.NewRunner(st))
	game := &model.Game{
		ID: "creator-opt-in", OwnerID: "creator-a", Title: "A game",
		Status: model.GameReady, Visibility: model.VisibilityPrivate,
		Manifest:  &model.BundleManifest{GameID: "creator-opt-in", Version: 1},
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	st.CreateGame(game)
	rec := do(t, srv, http.MethodPost, "/v1/games/"+game.ID+"/publish",
		map[string]any{"caption": "hello", "topics": []string{}, "allow_remixes": true}, "creator-a")
	if rec.Code != http.StatusAccepted {
		t.Fatalf("owner publish opt-in got %d: %s", rec.Code, rec.Body.String())
	}
	got, ok := st.GetGame(game.ID)
	if !ok || !got.RemixAllowed || got.ModerationStatus != "pending" {
		t.Fatalf("opt-in must persist without automatically bypassing moderation: %#v", got)
	}
	if canRemixGame(got, "creator-b") {
		t.Fatal("creator opt-in must not make a pending moderation game eligible")
	}
}
