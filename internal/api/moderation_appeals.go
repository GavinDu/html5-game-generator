package api

import (
	"net/http"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"doodlequest/internal/model"
	"doodlequest/internal/pipeline"
)

type moderationAppealRequest struct {
	Reason string `json:"reason"`
}

// screenPublication flags a narrow set of high-confidence text patterns for
// human triage. It is not a semantic classifier and does not inspect binary
// media. Matches never auto-reject content; publication remains pending.
func screenPublication(game *model.Game) []string {
	if game == nil {
		return nil
	}
	parts := []string{game.Title, game.Intent, game.Caption}
	parts = append(parts, game.Topics...)
	for _, element := range game.Elements {
		parts = append(parts, element.Label, element.Description)
	}
	text := strings.ToLower(strings.Join(strings.Fields(strings.Join(parts, " ")), " "))
	rules := []struct {
		category string
		phrases  []string
	}{
		{category: "self_harm_risk", phrases: []string{
			"kill yourself", "how to kill myself", "how to kill yourself",
			"suicide method", "how to overdose",
		}},
		{category: "sexual_exploitation_risk", phrases: []string{
			"send nudes", "send me nude", "nude pictures", "explicit private photo",
		}},
		{category: "violent_instruction_risk", phrases: []string{
			"how to make a bomb", "build a bomb", "make an explosive", "make a weapon",
		}},
		{category: "grooming_risk", phrases: []string{
			"don't tell your parents", "dont tell your parents", "meet me in secret",
			"keep this between us", "send me a private photo",
		}},
		{category: "sensitive_personal_data_risk", phrases: []string{
			"give me your password", "share your password", "send your password",
			"credit card number", "social security number",
		}},
	}
	var flags []string
	for _, rule := range rules {
		for _, phrase := range rule.phrases {
			if strings.Contains(text, phrase) {
				flags = append(flags, rule.category)
				break
			}
		}
	}
	return flags
}

func (s *Server) handleModerationAppeal(w http.ResponseWriter, r *http.Request) {
	gameID := strings.TrimSpace(r.PathValue("id"))
	game, ok := s.store.GetGame(gameID)
	if !ok {
		writeErr(w, http.StatusNotFound, "game not found")
		return
	}
	creator := strings.TrimSpace(userID(r))
	if creator == "" || game.OwnerID != creator {
		writeErr(w, http.StatusForbidden, "only the creator can appeal this moderation decision")
		return
	}
	if !game.Quarantined && game.ModerationStatus != "rejected" && game.ModerationStatus != "quarantined" {
		writeErr(w, http.StatusConflict, "only rejected or quarantined games can be appealed")
		return
	}
	var req moderationAppealRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" || utf8.RuneCountInString(reason) > 1000 {
		writeErr(w, http.StatusBadRequest, "appeal reason must contain 1–1000 characters")
		return
	}

	s.socialMu.Lock()
	defer s.socialMu.Unlock()
	for _, prior := range s.reports {
		if prior.GameID == gameID && prior.Reporter == creator &&
			prior.Status == "open" && strings.HasPrefix(prior.Reason, "creator_appeal:") {
			writeJSON(w, http.StatusConflict, map[string]any{
				"error":     "an appeal is already open for this game",
				"report_id": prior.ID,
			})
			return
		}
	}
	now := time.Now().UTC()
	appeal := socialReport{
		ID: pipeline.NewID(), GameID: gameID, Reporter: creator,
		Reason: "creator_appeal: " + reason, CreatedAt: now, Status: "open",
	}
	if err := s.persistAudit(r.Context(), moderationAuditEvent{
		ID: pipeline.NewID(), Action: "appeal_submitted", GameID: gameID,
		ReportID: appeal.ID, ActorID: creator, Status: "open",
		Reason: "creator appeal submitted", CreatedAt: now,
	}); err != nil {
		writeErr(w, http.StatusServiceUnavailable, "could not persist moderation audit")
		return
	}
	s.reports = append(s.reports, appeal)
	if err := s.persistSocialStateLocked(); err != nil {
		s.reports = s.reports[:len(s.reports)-1]
		writeErr(w, http.StatusServiceUnavailable, "could not persist appeal queue")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"appeal_id": appeal.ID, "status": appeal.Status,
		"message": "appeal queued for moderator review",
	})
}

func (s *Server) handleModerationContact(w http.ResponseWriter, r *http.Request) {
	url := strings.TrimSpace(os.Getenv("DQ_MODERATION_CONTACT_URL"))
	email := strings.TrimSpace(os.Getenv("DQ_MODERATION_CONTACT_EMAIL"))
	writeJSON(w, http.StatusOK, map[string]any{
		"contact_url":                 url,
		"contact_email":               email,
		"appeals_endpoint":            "POST /v1/games/{id}/moderation-appeals",
		"external_contact_configured": url != "" || email != "",
	})
}
