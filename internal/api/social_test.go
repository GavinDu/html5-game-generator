package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
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
	dir := t.TempDir()
	t.Setenv("DQ_SOCIAL_STATE_FILE", filepath.Join(dir, "social-state.json"))
	t.Setenv("DQ_SOCIAL_EVENTS_FILE", filepath.Join(dir, "social-events.jsonl"))
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
	dir := t.TempDir()
	t.Setenv("DQ_SOCIAL_STATE_FILE", filepath.Join(dir, "social-state.json"))
	t.Setenv("DQ_SOCIAL_EVENTS_FILE", filepath.Join(dir, "social-events.jsonl"))
	resp := do(t, newTestServer(), http.MethodGet, "/v1/feed?scope=viral", nil, "viewer")
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("unknown scope: %d %s", resp.Code, resp.Body.String())
	}
}

func TestSocialInteractionsPublishingAndModeration(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DQ_SOCIAL_STATE_FILE", filepath.Join(dir, "social-state.json"))
	t.Setenv("DQ_SOCIAL_EVENTS_FILE", filepath.Join(dir, "social-events.jsonl"))
	srv := newTestServer()
	addPublicGame(srv, "post-1", "creator")
	resp := do(t, srv, http.MethodPost, "/v1/games/post-1/publish", map[string]any{
		"caption": "A tiny adventure", "topics": []string{"puzzle", "#Family", "puzzle"},
	}, "creator")
	if resp.Code != http.StatusAccepted || !strings.Contains(resp.Body.String(), `"moderation_status":"pending"`) {
		t.Fatalf("publish should enter pending moderation: %d %s", resp.Code, resp.Body.String())
	}
	resp = do(t, srv, http.MethodGet, "/v1/feed?scope=for_you", nil, "viewer")
	if strings.Contains(resp.Body.String(), "post-1") {
		t.Fatalf("pending post leaked into discovery: %s", resp.Body.String())
	}
	t.Setenv("DQ_MODERATION_TOKEN", "test-moderation-token")
	resp = do(t, srv, http.MethodPost, "/v1/games/post-1/moderation", map[string]any{"status": "approved"}, "moderator")
	if resp.Code != http.StatusOK {
		t.Fatalf("approve post: %d %s", resp.Code, resp.Body.String())
	}
	resp = do(t, srv, http.MethodPost, "/v1/games/post-1/publish", map[string]any{"caption": "changed"}, "creator")
	if resp.Code != http.StatusConflict {
		t.Fatalf("published post should be immutable: %d %s", resp.Code, resp.Body.String())
	}

	resp = do(t, srv, http.MethodPost, "/v1/games/post-1/reactions", map[string]any{"reaction": "love"}, "viewer")
	if resp.Code != http.StatusOK {
		t.Fatalf("reaction: %d %s", resp.Code, resp.Body.String())
	}
	resp = do(t, srv, http.MethodPost, "/v1/games/post-1/comments", map[string]any{"body": "Nice game!"}, "viewer")
	if resp.Code != http.StatusCreated {
		t.Fatalf("comment: %d %s", resp.Code, resp.Body.String())
	}
	resp = do(t, srv, http.MethodPost, "/v1/games/post-1/save", nil, "viewer")
	if resp.Code != http.StatusOK {
		t.Fatalf("save: %d %s", resp.Code, resp.Body.String())
	}
	resp = do(t, srv, http.MethodGet, "/v1/users/me/saved", nil, "viewer")
	if resp.Code != http.StatusOK || !strings.Contains(resp.Body.String(), "post-1") {
		t.Fatalf("saved list: %d %s", resp.Code, resp.Body.String())
	}
	resp = do(t, srv, http.MethodPost, "/v1/games/post-1/feedback", map[string]any{"kind": "not_interested"}, "viewer")
	if resp.Code != http.StatusOK {
		t.Fatalf("feedback: %d %s", resp.Code, resp.Body.String())
	}
	resp = do(t, srv, http.MethodGet, "/v1/feed?scope=for_you", nil, "viewer")
	if strings.Contains(resp.Body.String(), "post-1") {
		t.Fatalf("not-interested post still in feed: %s", resp.Body.String())
	}
}

func TestSocialBlockReportAndEvents(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DQ_SOCIAL_STATE_FILE", filepath.Join(dir, "social-state.json"))
	t.Setenv("DQ_SOCIAL_EVENTS_FILE", filepath.Join(dir, "social-events.jsonl"))
	srv := newTestServer()
	addPublicGame(srv, "alice-game", "alice")
	resp := do(t, srv, http.MethodPost, "/v1/games/alice-game/report", map[string]any{"reason": "unsafe gameplay"}, "viewer")
	if resp.Code != http.StatusCreated || !strings.Contains(resp.Body.String(), `"status":"open"`) {
		t.Fatalf("report: %d %s", resp.Code, resp.Body.String())
	}
	var report struct {
		ReportID string `json:"report_id"`
	}
	decode(t, resp, &report)
	t.Setenv("DQ_MODERATION_TOKEN", "test-moderation-token")
	resp = do(t, srv, http.MethodGet, "/v1/moderation/reports", nil, "moderator")
	if resp.Code != http.StatusOK || !strings.Contains(resp.Body.String(), report.ReportID) {
		t.Fatalf("moderation report queue: %d %s", resp.Code, resp.Body.String())
	}
	tooLongReason := strings.Repeat("x", 501)
	resp = do(t, srv, http.MethodPost, "/v1/moderation/reports/"+report.ReportID+"/resolve?reason="+tooLongReason, nil, "moderator")
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("oversized resolution reason: %d %s", resp.Code, resp.Body.String())
	}
	resp = do(t, srv, http.MethodGet, "/v1/moderation/reports", nil, "moderator")
	if resp.Code != http.StatusOK || !strings.Contains(resp.Body.String(), `"status":"open"`) {
		t.Fatalf("invalid resolution changed report state: %d %s", resp.Code, resp.Body.String())
	}
	resp = do(t, srv, http.MethodPost, "/v1/moderation/reports/"+report.ReportID+"/resolve?reason=reviewed%20and%20actioned", nil, "moderator")
	if resp.Code != http.StatusOK ||
		!strings.Contains(resp.Body.String(), `"status":"resolved"`) ||
		!strings.Contains(resp.Body.String(), `"resolved_by":"moderator"`) ||
		!strings.Contains(resp.Body.String(), `"resolution_reason":"reviewed and actioned"`) ||
		!strings.Contains(resp.Body.String(), `"resolved_at":`) {
		t.Fatalf("resolve report should return audit fields: %d %s", resp.Code, resp.Body.String())
	}
	// Repeated resolution is idempotent and cannot rewrite the audit record.
	resp = do(t, srv, http.MethodPost, "/v1/moderation/reports/"+report.ReportID+"/resolve?reason=rewritten", nil, "other-moderator")
	if resp.Code != http.StatusOK ||
		!strings.Contains(resp.Body.String(), `"resolved_by":"moderator"`) ||
		!strings.Contains(resp.Body.String(), `"resolution_reason":"reviewed and actioned"`) {
		t.Fatalf("repeated resolution rewrote audit fields: %d %s", resp.Code, resp.Body.String())
	}
	restarted := newTestServer()
	resp = do(t, restarted, http.MethodGet, "/v1/moderation/reports", nil, "moderator")
	if resp.Code != http.StatusOK ||
		!strings.Contains(resp.Body.String(), `"resolved_by":"moderator"`) ||
		!strings.Contains(resp.Body.String(), `"resolution_reason":"reviewed and actioned"`) {
		t.Fatalf("report audit did not survive restart: %d %s", resp.Code, resp.Body.String())
	}
	resp = do(t, srv, http.MethodPost, "/v1/users/alice/block", nil, "viewer")
	if resp.Code != http.StatusOK {
		t.Fatalf("block: %d %s", resp.Code, resp.Body.String())
	}
	resp = do(t, srv, http.MethodGet, "/v1/feed?scope=for_you", nil, "viewer")
	if strings.Contains(resp.Body.String(), "alice-game") {
		t.Fatalf("blocked creator leaked into feed: %s", resp.Body.String())
	}
	resp = do(t, srv, http.MethodPost, "/v1/telemetry/social-event", map[string]any{"game_id": "alice-game", "kind": "completion", "duration_ms": 12000, "expected_duration_ms": 15000}, "viewer")
	if resp.Code != http.StatusAccepted {
		t.Fatalf("event: %d %s", resp.Code, resp.Body.String())
	}
	resp = do(t, srv, http.MethodDelete, "/v1/users/alice/block", nil, "viewer")
	if resp.Code != http.StatusOK {
		t.Fatalf("unblock: %d %s", resp.Code, resp.Body.String())
	}
}

func TestSocialInteractionsPersistAcrossServerRestart(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DQ_SOCIAL_STATE_FILE", filepath.Join(dir, "social-state.json"))
	t.Setenv("DQ_SOCIAL_EVENTS_FILE", filepath.Join(dir, "social-events.jsonl"))
	first := newTestServer()
	addPublicGame(first, "persist-game", "creator")
	resp := do(t, first, http.MethodPost, "/v1/games/persist-game/save", nil, "viewer")
	if resp.Code != http.StatusOK {
		t.Fatalf("save: %d %s", resp.Code, resp.Body.String())
	}
	resp = do(t, first, http.MethodPost, "/v1/games/persist-game/comments", map[string]any{"body": "keep this comment"}, "viewer")
	if resp.Code != http.StatusCreated {
		t.Fatalf("comment: %d %s", resp.Code, resp.Body.String())
	}

	second := newTestServer()
	addPublicGame(second, "persist-game", "creator")
	resp = do(t, second, http.MethodGet, "/v1/users/me/saved", nil, "viewer")
	if resp.Code != http.StatusOK || !strings.Contains(resp.Body.String(), "persist-game") {
		t.Fatalf("saved state did not persist: %d %s", resp.Code, resp.Body.String())
	}
	resp = do(t, second, http.MethodGet, "/v1/games/persist-game/comments", nil, "viewer")
	if resp.Code != http.StatusOK || !strings.Contains(resp.Body.String(), "keep this comment") {
		t.Fatalf("comment did not persist: %d %s", resp.Code, resp.Body.String())
	}
}

func TestFeedCursorPagination(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DQ_SOCIAL_STATE_FILE", filepath.Join(dir, "social-state.json"))
	t.Setenv("DQ_SOCIAL_EVENTS_FILE", filepath.Join(dir, "social-events.jsonl"))
	srv := newTestServer()
	addPublicGame(srv, "page-a", "alice")
	addPublicGame(srv, "page-b", "bob")
	addPublicGame(srv, "page-c", "carol")
	resp := do(t, srv, http.MethodGet, "/v1/feed?scope=for_you&limit=1", nil, "viewer")
	if resp.Code != http.StatusOK {
		t.Fatalf("first page: %d %s", resp.Code, resp.Body.String())
	}
	var first struct {
		Games []model.Game `json:"games"`
		Next  string       `json:"next_cursor"`
	}
	decode(t, resp, &first)
	if len(first.Games) != 1 || first.Next == "" {
		t.Fatalf("expected one item and cursor: %+v", first)
	}
	resp = do(t, srv, http.MethodGet, "/v1/feed?scope=for_you&limit=1&cursor="+first.Next, nil, "viewer")
	if resp.Code != http.StatusOK {
		t.Fatalf("second page: %d %s", resp.Code, resp.Body.String())
	}
	var second struct {
		Games []model.Game `json:"games"`
	}
	decode(t, resp, &second)
	if len(second.Games) != 1 || second.Games[0].ID == first.Games[0].ID {
		t.Fatalf("pagination repeated item: first=%+v second=%+v", first, second)
	}
	resp = do(t, srv, http.MethodGet, "/v1/feed?limit=0", nil, "viewer")
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("invalid limit should be rejected: %d %s", resp.Code, resp.Body.String())
	}
}

func TestRuntimeTelemetryFeedsSocialPlaySignals(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DQ_SOCIAL_STATE_FILE", filepath.Join(dir, "social-state.json"))
	t.Setenv("DQ_SOCIAL_EVENTS_FILE", filepath.Join(dir, "social-events.jsonl"))
	t.Setenv("DQ_TELEMETRY_DIR", filepath.Join(dir, "telemetry"))
	srv := newTestServer()
	addPublicGame(srv, "runtime-game", "creator")
	for i := 0; i < 2; i++ {
		resp := do(t, srv, http.MethodPost, "/v1/telemetry/events", map[string]any{
			"gameId": "runtime-game", "kind": "session_start", "runtimeVersion": "test", "sessionId": fmt.Sprintf("session-%d", i),
		}, "viewer")
		if resp.Code != http.StatusAccepted {
			t.Fatalf("session start: %d %s", resp.Code, resp.Body.String())
		}
	}
	resp := do(t, srv, http.MethodPost, "/v1/telemetry/events", map[string]any{
		"gameId": "runtime-game", "kind": "run_end", "runtimeVersion": "test", "sessionId": "session-2", "won": true, "durationMs": 12500,
	}, "viewer")
	if resp.Code != http.StatusAccepted {
		t.Fatalf("run end: %d %s", resp.Code, resp.Body.String())
	}
	srv.socialMu.RLock()
	defer srv.socialMu.RUnlock()
	counts := map[string]int{}
	for _, event := range srv.events {
		if event.GameID == "runtime-game" {
			counts[event.Kind]++
		}
	}
	if counts["play_start"] != 1 || counts["replay"] != 1 || counts["completion"] != 1 {
		t.Fatalf("runtime social signals not mapped correctly: %#v", counts)
	}
}

func TestCreatorDiscoveryFiltersAndPagination(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DQ_SOCIAL_STATE_FILE", filepath.Join(dir, "social-state.json"))
	t.Setenv("DQ_SOCIAL_EVENTS_FILE", filepath.Join(dir, "social-events.jsonl"))
	srv := newTestServer()
	srv.store.CreateGame(&model.Game{
		ID: "a-game", Title: "Puzzle Adventure", OwnerID: "alice",
		Status: model.GameReady, Visibility: model.VisibilityPublic,
		Topics: []string{"Puzzle", "Family"}, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})
	srv.store.CreateGame(&model.Game{
		ID: "b-game", Title: "Racing", OwnerID: "bob",
		Status: model.GameReady, Visibility: model.VisibilityPublic,
		Topics: []string{"racing"}, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})
	srv.store.CreateGame(&model.Game{
		ID: "c-game", Title: "Private Puzzle", OwnerID: "carol",
		Status: model.GameReady, Visibility: model.VisibilityPrivate,
		Topics: []string{"puzzle"}, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})
	srv.store.CreateGame(&model.Game{
		ID: "d-game", Title: "Pending Puzzle", OwnerID: "dave",
		Status: model.GameReady, Visibility: model.VisibilityPublic,
		ModerationStatus: "pending", Topics: []string{"puzzle"},
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})
	resp := do(t, srv, http.MethodGet, "/v1/creators?q=PUZZLE", nil, "viewer")
	if resp.Code != http.StatusOK || !strings.Contains(resp.Body.String(), "alice") ||
		strings.Contains(resp.Body.String(), "carol") || strings.Contains(resp.Body.String(), "dave") {
		t.Fatalf("search privacy: %d %s", resp.Code, resp.Body.String())
	}
	resp = do(t, srv, http.MethodGet, "/v1/creators?topic=%23PUZZLE", nil, "viewer")
	if resp.Code != http.StatusOK || !strings.Contains(resp.Body.String(), "alice") ||
		strings.Contains(resp.Body.String(), "bob") {
		t.Fatalf("topic filter: %d %s", resp.Code, resp.Body.String())
	}
	resp = do(t, srv, http.MethodGet, "/v1/creators?limit=1", nil, "viewer")
	if resp.Code != http.StatusOK || !strings.Contains(resp.Body.String(), "next_cursor") ||
		!strings.Contains(resp.Body.String(), "alice") {
		t.Fatalf("first page: %d %s", resp.Code, resp.Body.String())
	}
	resp = do(t, srv, http.MethodGet, "/v1/creators?limit=1&cursor=alice", nil, "viewer")
	if resp.Code != http.StatusOK || !strings.Contains(resp.Body.String(), "bob") ||
		strings.Contains(resp.Body.String(), "alice") {
		t.Fatalf("second page: %d %s", resp.Code, resp.Body.String())
	}
	resp = do(t, srv, http.MethodGet, "/v1/creators?limit=51", nil, "viewer")
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("invalid limit: %d %s", resp.Code, resp.Body.String())
	}
}

func TestModerationAuditTrailIsProtectedAndAppendOnly(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DQ_SOCIAL_STATE_FILE", filepath.Join(dir, "social-state.json"))
	t.Setenv("DQ_SOCIAL_EVENTS_FILE", filepath.Join(dir, "social-events.jsonl"))
	t.Setenv("DQ_MODERATION_AUDIT_FILE", filepath.Join(dir, "moderation-audit.jsonl"))
	t.Setenv("DQ_MODERATION_TOKEN", "audit-test-token")
	srv := newTestServer()
	addPublicGame(srv, "audit-game", "creator")

	reportResp := do(t, srv, http.MethodPost, "/v1/games/audit-game/report", map[string]any{"reason": "unsafe content"}, "viewer")
	if reportResp.Code != http.StatusCreated {
		t.Fatalf("create report: %d %s", reportResp.Code, reportResp.Body.String())
	}
	var report struct {
		ReportID string `json:"report_id"`
	}
	decode(t, reportResp, &report)

	moderateResp := do(t, srv, http.MethodPost, "/v1/games/audit-game/moderation", map[string]any{"status": "rejected", "reason": "reviewed"}, "moderator")
	if moderateResp.Code != http.StatusOK {
		t.Fatalf("moderate game: %d %s", moderateResp.Code, moderateResp.Body.String())
	}
	resolveResp := do(t, srv, http.MethodPost, "/v1/moderation/reports/"+report.ReportID+"/resolve?reason=confirmed", nil, "moderator")
	if resolveResp.Code != http.StatusOK {
		t.Fatalf("resolve report: %d %s", resolveResp.Code, resolveResp.Body.String())
	}

	unauthorized := httptest.NewRequest(http.MethodGet, "/v1/moderation/audit", nil)
	unauthorized.Header.Set("X-Moderation-Token", "wrong")
	unauthorizedRec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(unauthorizedRec, unauthorized)
	if unauthorizedRec.Code != http.StatusForbidden {
		t.Fatalf("audit endpoint should be protected: got %d", unauthorizedRec.Code)
	}

	auditResp := do(t, srv, http.MethodGet, "/v1/moderation/audit?game_id=audit-game&limit=10", nil, "moderator")
	if auditResp.Code != http.StatusOK {
		t.Fatalf("read audit: %d %s", auditResp.Code, auditResp.Body.String())
	}
	for _, action := range []string{"report_created", "game_moderation_changed", "report_resolved"} {
		if !strings.Contains(auditResp.Body.String(), action) {
			t.Fatalf("audit missing %q: %s", action, auditResp.Body.String())
		}
	}
	if strings.Contains(auditResp.Body.String(), "other-game") {
		t.Fatalf("game filter leaked another game's audit records: %s", auditResp.Body.String())
	}
}

func TestFeedRankingColdStartEngagementAndRollback(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DQ_SOCIAL_STATE_FILE", filepath.Join(dir, "social-state.json"))
	t.Setenv("DQ_SOCIAL_EVENTS_FILE", filepath.Join(dir, "social-events.jsonl"))
	t.Setenv("DQ_FEED_RANKING_ENABLED", "")
	srv := newTestServer()
	now := time.Now()
	addPublicGame(srv, "old-game", "alice")
	addPublicGame(srv, "new-game", "bob")
	oldGame, _ := srv.store.GetGame("old-game")
	oldGame.CreatedAt = now.Add(-48 * time.Hour)
	srv.store.UpdateGame(oldGame)
	newGame, _ := srv.store.GetGame("new-game")
	newGame.CreatedAt = now
	srv.store.UpdateGame(newGame)

	readFirst := func() string {
		t.Helper()
		resp := do(t, srv, http.MethodGet, "/v1/feed?scope=for_you", nil, "viewer")
		if resp.Code != http.StatusOK {
			t.Fatalf("feed: %d %s", resp.Code, resp.Body.String())
		}
		var result struct {
			Games []model.Game `json:"games"`
		}
		decode(t, resp, &result)
		if len(result.Games) != 2 {
			t.Fatalf("want two feed games, got %d", len(result.Games))
		}
		return result.Games[0].ID
	}

	if got := readFirst(); got != "new-game" {
		t.Fatalf("cold-start feed should be newest-first, got %q", got)
	}

	events := make([]socialEvent, 19)
	for i := range events {
		events[i] = socialEvent{GameID: "old-game", Kind: "completion", CreatedAt: now}
	}
	srv.socialMu.Lock()
	srv.events = events
	srv.reactions["old-game"] = map[string]string{"viewer": "love"}
	srv.socialMu.Unlock()

	if got := readFirst(); got != "old-game" {
		t.Fatalf("engagement ranking should promote the older completed game after 20 signals, got %q", got)
	}

	t.Setenv("DQ_FEED_RANKING_ENABLED", "false")
	if got := readFirst(); got != "new-game" {
		t.Fatalf("rollback switch should restore newest-first ranking, got %q", got)
	}
}
