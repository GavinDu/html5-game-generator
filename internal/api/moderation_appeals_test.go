package api

import (
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"doodlequest/internal/model"
)

func TestModerationAppealRequiresOwnerAndDeduplicates(t *testing.T) {
	dir:=t.TempDir()
	t.Setenv("DQ_SOCIAL_STATE_FILE",filepath.Join(dir,"social.json"))
	t.Setenv("DQ_MODERATION_AUDIT_FILE",filepath.Join(dir,"audit.jsonl"))
	t.Setenv("DQ_MODERATION_TOKEN","moderation-test-token")
	srv:=newTestServer()
	now:=time.Now().UTC()
	srv.store.CreateGame(&model.Game{ID:"appeal-game",Title:"Test",OwnerID:"creator",Status:model.GameReady,Visibility:model.VisibilityPublic,ModerationStatus:"rejected",CreatedAt:now,UpdatedAt:now})
	body:=map[string]any{"reason":"This decision appears mistaken."}
	if got:=do(t,srv,http.MethodPost,"/v1/games/appeal-game/moderation-appeals",body,"someone-else").Code; got!=http.StatusForbidden { t.Fatalf("non-owner got %d",got) }
	first:=do(t,srv,http.MethodPost,"/v1/games/appeal-game/moderation-appeals",body,"creator")
	if first.Code!=http.StatusCreated { t.Fatalf("appeal got %d: %s",first.Code,first.Body.String()) }
	second:=do(t,srv,http.MethodPost,"/v1/games/appeal-game/moderation-appeals",body,"creator")
	if second.Code!=http.StatusConflict { t.Fatalf("duplicate appeal got %d: %s",second.Code,second.Body.String()) }
	queue:=do(t,srv,http.MethodGet,"/v1/moderation/reports",nil,"operator")
	if queue.Code!=http.StatusOK || !strings.Contains(queue.Body.String(),"creator_appeal: This decision appears mistaken.") { t.Fatalf("appeal not in queue: %d %s",queue.Code,queue.Body.String()) }
}

func TestModerationContactEndpoint(t *testing.T) {
	t.Setenv("DQ_MODERATION_CONTACT_URL","https://example.test/moderation")
	t.Setenv("DQ_MODERATION_CONTACT_EMAIL","safety@example.test")
	srv:=newTestServer()
	rec:=do(t,srv,http.MethodGet,"/v1/moderation/contact",nil,"creator")
	if rec.Code!=http.StatusOK || !strings.Contains(rec.Body.String(),"safety@example.test") || !strings.Contains(rec.Body.String(),"external_contact_configured") { t.Fatalf("contact response: %d %s",rec.Code,rec.Body.String()) }
}

func TestScreenPublicationFlagsHighConfidenceTextWithoutAutoReject(t *testing.T) {
	game := &model.Game{Title: "A game", Intent: "collect stars", Caption: "Don't tell your parents and meet me in secret"}
	flags := screenPublication(game)
	if len(flags) != 1 || flags[0] != "grooming_risk" {
		t.Fatalf("unexpected flags: %#v", flags)
	}
	if got := screenPublication(&model.Game{Title: "Friendly star game", Intent: "collect stars"}); len(got) != 0 {
		t.Fatalf("benign text unexpectedly flagged: %#v", got)
	}
}
