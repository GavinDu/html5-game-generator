package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"doodlequest/internal/model"
)

func TestModerationAppealRequiresOwnerAndDeduplicates(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DQ_SOCIAL_STATE_FILE", filepath.Join(dir, "social.json"))
	t.Setenv("DQ_MODERATION_AUDIT_FILE", filepath.Join(dir, "audit.jsonl"))
	t.Setenv("DQ_MODERATION_TOKEN", "moderation-test-token")
	srv := newTestServer()
	now := time.Now().UTC()
	srv.store.CreateGame(&model.Game{
		ID: "appeal-game", Title: "Test", OwnerID: "creator",
		Status: model.GameReady, Visibility: model.VisibilityPublic,
		ModerationStatus: "rejected", CreatedAt: now, UpdatedAt: now,
	})
	body := map[string]any{"reason": "This decision appears mistaken."}
	if got := do(t, srv, http.MethodPost, "/v1/games/appeal-game/moderation-appeals", body, "someone-else").Code; got != http.StatusForbidden {
		t.Fatalf("non-owner got %d", got)
	}
	first := do(t, srv, http.MethodPost, "/v1/games/appeal-game/moderation-appeals", body, "creator")
	if first.Code != http.StatusCreated {
		t.Fatalf("appeal got %d: %s", first.Code, first.Body.String())
	}
	second := do(t, srv, http.MethodPost, "/v1/games/appeal-game/moderation-appeals", body, "creator")
	if second.Code != http.StatusConflict {
		t.Fatalf("duplicate appeal got %d: %s", second.Code, second.Body.String())
	}
	queue := do(t, srv, http.MethodGet, "/v1/moderation/reports", nil, "operator")
	if queue.Code != http.StatusOK || !strings.Contains(queue.Body.String(), "creator_appeal: This decision appears mistaken.") {
		t.Fatalf("appeal not in queue: %d %s", queue.Code, queue.Body.String())
	}
}

func TestModerationContactEndpoint(t *testing.T) {
	t.Setenv("DQ_MODERATION_CONTACT_URL", "https://example.test/moderation")
	t.Setenv("DQ_MODERATION_CONTACT_EMAIL", "safety@example.test")
	srv := newTestServer()
	rec := do(t, srv, http.MethodGet, "/v1/moderation/contact", nil, "creator")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "safety@example.test") || !strings.Contains(rec.Body.String(), "external_contact_configured") {
		t.Fatalf("contact response: %d %s", rec.Code, rec.Body.String())
	}
}

func TestScreenPublicationFlagsHighConfidenceTextWithoutAutoReject(t *testing.T) {
	game := &model.Game{
		Title: "A game", Intent: "collect stars",
		Caption: "Don't tell your parents and meet me in secret",
	}
	flags := screenPublication(game)
	if len(flags) != 1 || flags[0] != "grooming_risk" {
		t.Fatalf("unexpected flags: %#v", flags)
	}
	if got := screenPublication(&model.Game{Title: "Friendly star game", Intent: "collect stars"}); len(got) != 0 {
		t.Fatalf("benign text unexpectedly flagged: %#v", got)
	}
}

func TestFlaggedPublishAuditFailureDoesNotPublishGame(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DQ_SOCIAL_STATE_FILE", filepath.Join(dir, "social.json"))
	t.Setenv("DQ_MODERATION_AUDIT_FILE", filepath.Join(dir, "audit.jsonl"))
	if err := os.Mkdir(filepath.Join(dir, "audit.jsonl"), 0o700); err != nil {
		t.Fatal(err)
	}
	srv := newTestServer()
	addPublicGame(srv, "screened-game", "creator")

	resp := do(t, srv, http.MethodPost, "/v1/games/screened-game/publish", map[string]any{
		"caption": "Don't tell your parents and meet me in secret",
	}, "creator")
	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("publish with unavailable audit should fail closed: %d %s", resp.Code, resp.Body.String())
	}
	game, ok := srv.store.GetGame("screened-game")
	if !ok {
		t.Fatal("game disappeared after failed publish")
	}
	if game.PublishedAt != nil || game.Caption != "" || game.ModerationStatus != "" {
		t.Fatalf("failed publish mutated the game: %#v", game)
	}
	srv.socialMu.RLock()
	defer srv.socialMu.RUnlock()
	if len(srv.reports) != 0 {
		t.Fatalf("failed publish queued an incomplete moderation report: %#v", srv.reports)
	}
}

func TestFlaggedPublishQueueFailureDoesNotPublishGame(t *testing.T) {
	dir := t.TempDir()
	socialPath := filepath.Join(dir, "social.json")
	t.Setenv("DQ_SOCIAL_STATE_FILE", socialPath)
	if err := os.Mkdir(socialPath, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DQ_MODERATION_AUDIT_FILE", filepath.Join(dir, "audit.jsonl"))
	srv := newTestServer()
	addPublicGame(srv, "screened-game", "creator")

	resp := do(t, srv, http.MethodPost, "/v1/games/screened-game/publish", map[string]any{
		"caption": "Don't tell your parents and meet me in secret",
	}, "creator")
	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("publish with unavailable moderator queue should fail closed: %d %s", resp.Code, resp.Body.String())
	}
	game, ok := srv.store.GetGame("screened-game")
	if !ok || game.PublishedAt != nil || game.Caption != "" || game.ModerationStatus != "" {
		t.Fatalf("failed queue persistence left the game published: %#v", game)
	}
	srv.socialMu.RLock()
	defer srv.socialMu.RUnlock()
	if len(srv.reports) != 0 {
		t.Fatalf("failed publish retained an unpersisted moderation report: %#v", srv.reports)
	}
}

func TestFlaggedPublishQueuesAuditableModeratorReview(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DQ_SOCIAL_STATE_FILE", filepath.Join(dir, "social.json"))
	auditPath := filepath.Join(dir, "audit.jsonl")
	t.Setenv("DQ_MODERATION_AUDIT_FILE", auditPath)
	srv := newTestServer()
	addPublicGame(srv, "screened-game", "creator")

	resp := do(t, srv, http.MethodPost, "/v1/games/screened-game/publish", map[string]any{
		"caption": "Don't tell your parents and meet me in secret",
	}, "creator")
	if resp.Code != http.StatusAccepted {
		t.Fatalf("flagged publish should remain pending for review: %d %s", resp.Code, resp.Body.String())
	}
	game, ok := srv.store.GetGame("screened-game")
	if !ok || game.PublishedAt == nil || game.ModerationStatus != "pending" {
		t.Fatalf("flagged game was not held pending: %#v", game)
	}
	srv.socialMu.RLock()
	defer srv.socialMu.RUnlock()
	if len(srv.reports) != 1 || !strings.Contains(srv.reports[0].Reason, "grooming_risk") {
		t.Fatalf("flagged publish missing moderator queue item: %#v", srv.reports)
	}
	audit, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(audit), `"automated_screening_flagged"`) {
		t.Fatalf("flagged publish missing audit event: %s", audit)
	}
}

func TestModerationAppealQueueFailureDoesNotClaimSuccess(t *testing.T) {
	dir := t.TempDir()
	socialPath := filepath.Join(dir, "social.json")
	t.Setenv("DQ_SOCIAL_STATE_FILE", socialPath)
	if err := os.Mkdir(socialPath, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DQ_MODERATION_AUDIT_FILE", filepath.Join(dir, "audit.jsonl"))
	srv := newTestServer()
	now := time.Now().UTC()
	srv.store.CreateGame(&model.Game{
		ID: "appeal-game", Title: "Test", OwnerID: "creator",
		Status: model.GameReady, Visibility: model.VisibilityPublic,
		ModerationStatus: "rejected", CreatedAt: now, UpdatedAt: now,
	})

	resp := do(t, srv, http.MethodPost, "/v1/games/appeal-game/moderation-appeals", map[string]any{
		"reason": "This decision appears mistaken.",
	}, "creator")
	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("appeal with unavailable queue should fail: %d %s", resp.Code, resp.Body.String())
	}
	srv.socialMu.RLock()
	defer srv.socialMu.RUnlock()
	if len(srv.reports) != 0 {
		t.Fatalf("failed appeal persistence retained an unpersisted queue item: %#v", srv.reports)
	}
}
