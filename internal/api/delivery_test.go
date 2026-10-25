package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"doodlequest/internal/model"
	"doodlequest/internal/pipeline"
	"doodlequest/internal/store"
)

// postOp performs a POST carrying the operator (moderation) credential
// the gated endpoints require; the matching env value is installed by
// opEnv. Added with the 2026-10-23 operator gate (architecture review
// follow-up): quarantine/unquarantine and repair/tuner apply used to
// accept anyone.
func postOp(t *testing.T, srv *Server, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(""))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Moderation-Token", "test-op-credential")
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	return rec
}

func opEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DQ_MODERATION_TOKEN", "test-op-credential")
}

func getAs(t *testing.T, srv *Server, path, user string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if user != "" {
		req.Header.Set("X-User-ID", user)
	}
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	return rec
}

// Delivery endpoints must enforce the same sharing + moderation rules
// the feed always had: before this gate, anyone with a game ID could
// download and play private, pending, rejected or quarantined games.
func TestDeliveryRespectsSharingAndModeration(t *testing.T) {
	st := store.NewMemoryStore()
	srv := New(st, pipeline.NewRunner(st))

	mutate := func(id string, fn func(*model.Game)) {
		g, ok := st.GetGame(id)
		if !ok {
			t.Fatalf("seed %s missing", id)
		}
		fn(g)
		st.UpdateGame(g)
	}
	seedReadyGame(st, "g-public")
	seedReadyGame(st, "g-private")
	mutate("g-private", func(g *model.Game) { g.Visibility = model.VisibilityPrivate })
	seedReadyGame(st, "g-quar")
	mutate("g-quar", func(g *model.Game) { g.Quarantined = true })
	seedReadyGame(st, "g-rejected")
	mutate("g-rejected", func(g *model.Game) { g.ModerationStatus = "rejected" })
	seedReadyGame(st, "g-pending")
	mutate("g-pending", func(g *model.Game) { g.ModerationStatus = "pending" })
	seedReadyGame(st, "g-group")
	st.CreateGroup(&model.Group{ID: "grp-1", Name: "Fam", OwnerID: "aunt-1", MemberIDs: []string{"cousin-1"}})
	mutate("g-group", func(g *model.Game) {
		g.Visibility = model.VisibilityGroups
		g.GroupIDs = []string{"grp-1"}
	})

	pkg := func(id, user string) int {
		return getAs(t, srv, "/v1/games/"+id+"/package", user).Code
	}
	// Public games are for everyone, signed in or not.
	if c := pkg("g-public", ""); c != http.StatusOK {
		t.Errorf("public/anon: %d, want 200", c)
	}
	if c := pkg("g-public", "stranger"); c != http.StatusOK {
		t.Errorf("public/stranger: %d, want 200", c)
	}
	// Restricted states: strangers get 404, the author keeps access.
	for _, id := range []string{"g-private", "g-quar", "g-rejected", "g-pending"} {
		if c := pkg(id, "stranger"); c != http.StatusNotFound {
			t.Errorf("%s/stranger: %d, want 404", id, c)
		}
		if c := pkg(id, "parent-1"); c != http.StatusOK {
			t.Errorf("%s/owner: %d, want 200", id, c)
		}
	}
	// Group games: members and the group owner get in, others don't.
	if c := pkg("g-group", "cousin-1"); c != http.StatusOK {
		t.Errorf("group/member: %d, want 200", c)
	}
	if c := pkg("g-group", "aunt-1"); c != http.StatusOK {
		t.Errorf("group/group-owner: %d, want 200", c)
	}
	if c := pkg("g-group", "stranger"); c != http.StatusNotFound {
		t.Errorf("group/stranger: %d, want 404", c)
	}
	// The same rules hold on the record and game-ir endpoints.
	if c := getAs(t, srv, "/v1/games/g-private", "stranger").Code; c != http.StatusNotFound {
		t.Errorf("record private/stranger: %d, want 404", c)
	}
	if c := getAs(t, srv, "/v1/games/g-quar/game-ir", "stranger").Code; c != http.StatusNotFound {
		t.Errorf("game-ir quarantined/stranger: %d, want 404", c)
	}
}

// Operator endpoints fail closed: unconfigured -> 503, wrong or
// missing credential -> 403, correct credential -> the action runs.
func TestOperatorEndpointsRequireCredential(t *testing.T) {
	st := store.NewMemoryStore()
	srv := New(st, pipeline.NewRunner(st))
	seedReadyGame(st, "g-op")

	// Unconfigured environment: 503, and the game is untouched.
	if rec := post(t, srv, "/v1/games/g-op/quarantine", `{"reason":"x"}`); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("unconfigured quarantine: %d, want 503", rec.Code)
	}
	opEnv(t)
	if rec := post(t, srv, "/v1/games/g-op/quarantine", `{"reason":"x"}`); rec.Code != http.StatusForbidden {
		t.Fatalf("no credential: %d, want 403", rec.Code)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/games/g-op/quarantine", strings.NewReader(`{"reason":"x"}`))
	req.Header.Set("X-Moderation-Token", "wrong")
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("wrong credential: %d, want 403", rec.Code)
	}
	if rec := postOp(t, srv, "/v1/games/g-op/quarantine"); rec.Code != http.StatusOK {
		t.Fatalf("with credential: %d %s", rec.Code, rec.Body.String())
	}
	g, _ := st.GetGame("g-op")
	if !g.Quarantined {
		t.Error("quarantine with credential should have applied")
	}
}

// A photo-sized create body (3 MiB of base64 payload) fits the photo
// routes but still busts the shared cap elsewhere.
func TestPhotoRoutesAcceptPhotoSizedBodies(t *testing.T) {
	st := store.NewMemoryStore()
	srv := New(st, pipeline.NewRunner(st))
	big := strings.Repeat("A", 3<<20)
	rec := do(t, srv, "POST", "/v1/games", map[string]any{
		"images":   []map[string]any{{"data_url": "data:image/jpeg;base64," + big}},
		"keywords": []string{"bear"},
	}, "parent-1")
	if rec.Code == http.StatusRequestEntityTooLarge {
		t.Fatalf("photo-sized create body rejected: %s", rec.Body.String())
	}
	rec = do(t, srv, "POST", "/v1/games/some-id/visibility", map[string]any{
		"visibility": "public", "pad": big,
	}, "parent-1")
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("non-photo route should keep the 2 MiB cap, got %d", rec.Code)
	}
}

// Follow edges persist like every other social relation: a fresh
// server over the same social-state file still knows who follows whom.
func TestFollowsSurviveRestart(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DQ_SOCIAL_STATE_FILE", dir+"/social-state.json")
	st := store.NewMemoryStore()
	srv := New(st, pipeline.NewRunner(st))
	st.CreateGame(&model.Game{ID: "alice-game", Title: "A", OwnerID: "alice",
		Status: model.GameReady, Visibility: model.VisibilityPublic})
	if rec := do(t, srv, "POST", "/v1/users/alice/follow", nil, "viewer"); rec.Code != http.StatusOK {
		t.Fatalf("follow: %d %s", rec.Code, rec.Body.String())
	}
	srv2 := New(st, pipeline.NewRunner(st))
	rec := do(t, srv2, "GET", "/v1/users/me/following", nil, "viewer")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "alice") {
		t.Fatalf("following after restart: %d %s", rec.Code, rec.Body.String())
	}
}

func TestDeliveryRejectsInvalidPersistedGameIR(t *testing.T) {
	st := store.NewMemoryStore()
	srv := New(st, pipeline.NewRunner(st))
	game := seedReadyGame(st, "corrupt-ir-game")
	if game.GameIR == nil {
		t.Fatal("seeded game has no Game IR")
	}
	// Corrupt a required field so validation fails even for a fixture with
	// no optional assets. Delivery must validate before serializing/serving.
	game.GameIR.Game.Title = "   "
	st.UpdateGame(game)

	for _, path := range []string{
		"/v1/games/corrupt-ir-game/package",
		"/v1/games/corrupt-ir-game/assets/untrusted-asset",
	} {
		rec := getAs(t, srv, path, "parent-1")
		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s: got %d, want %d; body=%s", path, rec.Code, http.StatusUnprocessableEntity, rec.Body.String())
		}
	}
}

func TestPostgresBlockHidesCreatorAcrossAPIInstances(t *testing.T) {
	if os.Getenv("DQ_SOCIAL_DATABASE_URL") == "" {
		t.Skip("DQ_SOCIAL_DATABASE_URL is not configured")
	}
	st := store.NewMemoryStore()
	viewer := "integration-feed-viewer-" + pipeline.NewID()
	creator := "integration-feed-creator-" + pipeline.NewID()
	gameID := "integration-blocked-game-" + pipeline.NewID()
	st.CreateGame(&model.Game{
		ID: gameID, Title: "Blocked creator game", OwnerID: creator,
		Status: model.GameReady, Visibility: model.VisibilityPublic,
		ModerationStatus: "approved",
	})
	first := New(st, pipeline.NewRunner(st))
	second := New(st, pipeline.NewRunner(st))
	if first.socialGraph == nil || second.socialGraph == nil {
		t.Fatal("expected both API instances to connect to the shared PostgreSQL graph")
	}
	t.Cleanup(func() {
		_ = first.socialGraph.Unblock(context.Background(), viewer, creator)
		_ = first.socialGraph.Close()
		_ = second.socialGraph.Close()
	})

	blocked := do(t, first, http.MethodPost, "/v1/users/"+creator+"/block", nil, viewer)
	if blocked.Code != http.StatusOK {
		t.Fatalf("block creator: %d %s", blocked.Code, blocked.Body.String())
	}
	feed := do(t, second, http.MethodGet, "/v1/feed", nil, viewer)
	if feed.Code != http.StatusOK {
		t.Fatalf("feed through second instance: %d %s", feed.Code, feed.Body.String())
	}
	var response struct {
		Games []model.Game `json:"games"`
	}
	decode(t, feed, &response)
	for _, game := range response.Games {
		if game.ID == gameID {
			t.Fatalf("blocked creator's game %q remained in feed from second instance", gameID)
		}
	}
}
