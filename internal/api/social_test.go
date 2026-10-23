package api

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"doodlequest/internal/model"
)

func addPublicGame(srv *Server, id, owner string) {
	srv.store.CreateGame(&model.Game{
		ID: id, Title: id, OwnerID: owner,
		Status: model.GameReady, Visibility: model.VisibilityPublic,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})
}

func TestSocialFeedScopesAndFollowLifecycle(t *testing.T) {
	srv := newTestServer()
	addPublicGame(srv, "alice-game", "alice")
	addPublicGame(srv, "bob-game", "bob")
	addPublicGame(srv, "private-game", "alice")
	private, _ := srv.store.GetGame("private-game")
	private.Visibility = model.VisibilityPrivate
	srv.store.UpdateGame(private)
	srv.store.CreateGame(&model.Game{
		ID: "quarantined-game", Title: "quarantined", OwnerID: "alice",
		Status: model.GameReady, Visibility: model.VisibilityPublic,
		Quarantined: true, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})

	resp := do(t, srv, http.MethodGet, "/v1/feed?scope=following", nil, "viewer")
	if resp.Code != http.StatusOK || !strings.Contains(resp.Body.String(), `"games":[]`) {
		t.Fatalf("empty following feed: %d %s", resp.Code, resp.Body.String())
	}

	resp = do(t, srv, http.MethodPost, "/v1/users/missing/follow", nil, "viewer")
	if resp.Code != http.StatusNotFound {
		t.Fatalf("unknown creator follow: %d %s", resp.Code, resp.Body.String())
	}
	resp = do(t, srv, http.MethodPost, "/v1/users/viewer/follow", nil, "viewer")
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("self follow: %d %s", resp.Code, resp.Body.String())
	}

	resp = do(t, srv, http.MethodPost, "/v1/users/alice/follow", nil, "viewer")
	if resp.Code != http.StatusOK {
		t.Fatalf("follow alice: %d %s", resp.Code, resp.Body.String())
	}
	resp = do(t, srv, http.MethodGet, "/v1/users/me/following", nil, "viewer")
	var following struct { CreatorIDs []string `json:"creator_ids"` }
	decode(t, resp, &following)
	if resp.Code != http.StatusOK || len(following.CreatorIDs) != 1 || following.CreatorIDs[0] != "alice" {
		t.Fatalf("following list: %d %s", resp.Code, resp.Body.String())
	}
	resp = do(t, srv, http.MethodGet, "/v1/feed?scope=following", nil, "viewer")
	if resp.Code != http.StatusOK || !strings.Contains(resp.Body.String(), "alice-game") || strings.Contains(resp.Body.String(), "bob-game") || strings.Contains(resp.Body.String(), "private-game") || strings.Contains(resp.Body.String(), "quarantined-game") {
		t.Fatalf("following feed leaked or missed games: %d %s", resp.Code, resp.Body.String())
	}

	resp = do(t, srv, http.MethodGet, "/v1/feed?scope=friends", nil, "viewer")
	if resp.Code != http.StatusOK || !strings.Contains(resp.Body.String(), `"games":[]`) {
		t.Fatalf("one-way friend feed: %d %s", resp.Code, resp.Body.String())
	}
	resp = do(t, srv, http.MethodPost, "/v1/users/viewer/follow", nil, "alice")
	if resp.Code != http.StatusNotFound {
		t.Fatalf("follow without public creator: %d %s", resp.Code, resp.Body.String())
	}
	addPublicGame(srv, "viewer-game", "viewer")
	resp = do(t, srv, http.MethodPost, "/v1/users/viewer/follow", nil, "alice")
	if resp.Code != http.StatusOK {
		t.Fatalf("alice follows viewer: %d %s", resp.Code, resp.Body.String())
	}
	resp = do(t, srv, http.MethodGet, "/v1/feed?scope=friends", nil, "viewer")
	if resp.Code != http.StatusOK || !strings.Contains(resp.Body.String(), "alice-game") || strings.Contains(resp.Body.String(), "bob-game") {
		t.Fatalf("mutual friend feed: %d %s", resp.Code, resp.Body.String())
	}

	resp = do(t, srv, http.MethodDelete, "/v1/users/alice/follow", nil, "viewer")
	if resp.Code != http.StatusOK {
		t.Fatalf("unfollow alice: %d %s", resp.Code, resp.Body.String())
	}
	resp = do(t, srv, http.MethodGet, "/v1/feed?scope=following", nil, "viewer")
	if resp.Code != http.StatusOK || !strings.Contains(resp.Body.String(), `"games":[]`) {
		t.Fatalf("following after unfollow: %d %s", resp.Code, resp.Body.String())
	}
}

func TestFeedRejectsUnknownScope(t *testing.T) {
	resp := do(t, newTestServer(), http.MethodGet, "/v1/feed?scope=viral", nil, "viewer")
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("unknown scope: %d %s", resp.Code, resp.Body.String())
	}
}

func TestGamePostReactionsAndComments(t *testing.T) {
    srv := newTestServer()
    addPublicGame(srv, "social-game", "creator")

    resp := do(t, srv, http.MethodGet, "/v1/games/social-game/social", nil, "alice")
    if resp.Code != http.StatusOK || !strings.Contains(resp.Body.String(), `"reaction_count":0`) {
        t.Fatalf("initial social summary: %d %s", resp.Code, resp.Body.String())
    }

    resp = do(t, srv, http.MethodPost, "/v1/games/social-game/reaction", map[string]any{}, "alice")
    if resp.Code != http.StatusOK || !strings.Contains(resp.Body.String(), `"reacted_by_me":true`) {
        t.Fatalf("react: %d %s", resp.Code, resp.Body.String())
    }
    // Idempotent reactions must not inflate the count.
    do(t, srv, http.MethodPost, "/v1/games/social-game/reaction", map[string]any{}, "alice")
    resp = do(t, srv, http.MethodGet, "/v1/games/social-game/social", nil, "alice")
    if !strings.Contains(resp.Body.String(), `"reaction_count":1`) {
        t.Fatalf("duplicate reaction count: %s", resp.Body.String())
    }
    resp = do(t, srv, http.MethodDelete, "/v1/games/social-game/reaction", nil, "alice")
    if resp.Code != http.StatusOK || !strings.Contains(resp.Body.String(), `"reacted_by_me":false`) {
        t.Fatalf("unreact: %d %s", resp.Code, resp.Body.String())
    }

    resp = do(t, srv, http.MethodPost, "/v1/games/social-game/comments", map[string]any{"text":"  Great game!  "}, "alice")
    if resp.Code != http.StatusCreated || !strings.Contains(resp.Body.String(), "Great game!") {
        t.Fatalf("create comment: %d %s", resp.Code, resp.Body.String())
    }
    resp = do(t, srv, http.MethodGet, "/v1/games/social-game/comments", nil, "bob")
    if resp.Code != http.StatusOK || !strings.Contains(resp.Body.String(), "Great game!") {
        t.Fatalf("list comments: %d %s", resp.Code, resp.Body.String())
    }
    resp = do(t, srv, http.MethodPost, "/v1/games/social-game/comments", map[string]any{"text":"   "}, "alice")
    if resp.Code != http.StatusBadRequest {
        t.Fatalf("blank comment should fail: %d %s", resp.Code, resp.Body.String())
    }
    resp = do(t, srv, http.MethodPost, "/v1/games/social-game/comments", map[string]any{"text":strings.Repeat("x", 501)}, "alice")
    if resp.Code != http.StatusBadRequest {
        t.Fatalf("oversized comment should fail: %d %s", resp.Code, resp.Body.String())
    }

    srv.store.UpdateGame(&model.Game{ID:"hidden-game", OwnerID:"creator", Status:model.GameReady, Visibility:model.VisibilityPrivate})
    resp = do(t, srv, http.MethodPost, "/v1/games/hidden-game/comments", map[string]any{"text":"leak"}, "alice")
    if resp.Code != http.StatusNotFound {
        t.Fatalf("private game social endpoint should not be exposed: %d %s", resp.Code, resp.Body.String())
    }
}
