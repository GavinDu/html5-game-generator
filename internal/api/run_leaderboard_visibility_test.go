package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"doodlequest/internal/model"
)

// A leaderboard contains players' names, score/time and opaque run IDs.
// It must not remain public after its game becomes private, quarantined,
// unpublished or owner/group restricted. Run POSTs share the same scope.
func TestRunAndLeaderboardDeliveryAuthorizationMatchesGamePackage(t *testing.T) {
	t.Setenv("DQ_ALLOW_INSECURE_USER_ID", "true") // Local fixture identities only.
	srv := newRunsServer(t)
	const gid = "sample-rainbow-bear"
	const owner = "ranking-creator"
	const stranger = "uninvited-viewer"
	route := "/v1/games/" + gid + "/leaderboard?window=all"
	runRoute := "/v1/games/" + gid + "/runs"
	record := map[string]any{
		"name": "Sensitive Display Name", "score": 12, "won": true,
		"durationMs": 25000, "seed": 42,
	}
	// Approved public game: an anonymous player can submit and read.
	created := do(t, srv, http.MethodPost, runRoute, record, "")
	if created.Code != http.StatusCreated {
		t.Fatalf("public anonymous run unavailable: %d %s", created.Code, created.Body.String())
	}
	public := do(t, srv, http.MethodGet, route, nil, "")
	if public.Code != http.StatusOK || !strings.Contains(public.Body.String(), "Sensitive Display Name") {
		t.Fatalf("public approved leaderboard unavailable: %d %s", public.Code, public.Body.String())
	}
	if cache := public.Header().Get("Cache-Control"); cache != "private, no-store" {
		t.Fatalf("public dynamic leaderboard must not be shared cached: %q", cache)
	}
	if vary := public.Header().Get("Vary"); !strings.Contains(vary, "Authorization") ||
		!strings.Contains(vary, "X-User-ID") {
		t.Fatalf("viewer-specific board must vary on identity: %q", vary)
	}

	// A later change to private must immediately revoke read/submit rights
	// for an outsider, while preserving the author's own preview.
	game, ok := srv.store.GetGame(gid)
	if !ok {
		t.Fatal("fixture game missing")
	}
	game.OwnerID = owner
	game.Status = model.GameReady
	game.Visibility = "private"
	game.GroupIDs = nil
	game.ModerationStatus = "approved"
	game.Quarantined = false
	srv.store.UpdateGame(game)

	before := len(srv.runs)
	path := filepath.Join(os.Getenv("DQ_TELEMETRY_DIR"), "runs.jsonl")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, viewer := range []string{"", stranger} {
		for _, method := range []string{http.MethodGet, http.MethodPost} {
			target := route
			var body any
			if method == http.MethodPost {
				target, body = runRoute, record
			}
			denied := do(t, srv, method, target, body, viewer)
			if denied.Code != http.StatusNotFound ||
				!strings.Contains(denied.Body.String(), "game not found") ||
				strings.Contains(denied.Body.String(), "Sensitive Display Name") {
				t.Fatalf("%s viewer %q private game data leaked: %d %s",
					method, viewer, denied.Code, denied.Body.String())
			}
			if got := denied.Header().Get("Cache-Control"); got != "private, no-store" {
				t.Fatalf("denied board/run must not cache: %q", got)
			}
		}
	}
	if len(srv.runs) != before {
		t.Fatal("unauthorized private run POST mutated in-memory score board")
	}
	afterInfo, err := os.Stat(path)
	if err != nil || afterInfo.Size() != info.Size() {
		t.Fatalf("unauthorized run POST wrote to durable replay log: %v", err)
	}
	ownerBoard := do(t, srv, http.MethodGet, route, nil, owner)
	if ownerBoard.Code != http.StatusOK || !strings.Contains(ownerBoard.Body.String(), "Sensitive Display Name") {
		t.Fatalf("owner private leaderboard denied: %d %s", ownerBoard.Code, ownerBoard.Body.String())
	}
	ownerRun := do(t, srv, http.MethodPost, runRoute, record, owner)
	if ownerRun.Code != http.StatusCreated {
		t.Fatalf("creator private replay submit denied: %d %s", ownerRun.Code, ownerRun.Body.String())
	}
	if len(srv.runs) != before+1 {
		t.Fatal("creator's authorized run was not stored")
	}

	// Group-only, quarantined, rejected, or still-draft games must never
	// publish a board or accept an outsider's run by guessing its ID.
	for _, variant := range []struct {
		name       string
		visibility model.Visibility
		status     model.GameStatus
		moderation string
		quarantine bool
	}{
		{"group-not-a-member", "groups", model.GameReady, "approved", false},
		{"quarantined", model.VisibilityPublic, model.GameReady, "approved", true},
		{"moderation-rejected", model.VisibilityPublic, model.GameReady, "rejected", false},
		{"unpublished", model.VisibilityPublic, "draft", "approved", false},
	} {
		t.Run(variant.name, func(t *testing.T) {
			game.Visibility = variant.visibility
			game.Status = variant.status
			game.ModerationStatus = variant.moderation
			game.Quarantined = variant.quarantine
			srv.store.UpdateGame(game)
			for _, method := range []string{http.MethodGet, http.MethodPost} {
				target := route
				var body any
				if method == http.MethodPost {
					target, body = runRoute, record
				}
				denied := do(t, srv, method, target, body, stranger)
				if denied.Code != http.StatusNotFound {
					t.Fatalf("outsider %s on %s: %d %s", method, variant.name,
						denied.Code, denied.Body.String())
				}
			}
		})
	}
}
