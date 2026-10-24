package api

import (
	"net/http"
	"os"
	"strings"
	"time"

	"doodlequest/internal/pipeline"
)

type moderationAppealRequest struct { Reason string `json:"reason"` }

func (s *Server) handleModerationAppeal(w http.ResponseWriter, r *http.Request) {
	gameID := strings.TrimSpace(r.PathValue("id"))
	game, ok := s.store.GetGame(gameID)
	if !ok { writeErr(w, http.StatusNotFound, "game not found"); return }
	creator := strings.TrimSpace(userID(r))
	if creator == "" || game.OwnerID != creator {
		writeErr(w, http.StatusForbidden, "only the creator can appeal this moderation decision"); return
	}
	if !game.Quarantined && game.ModerationStatus != "rejected" && game.ModerationStatus != "quarantined" {
		writeErr(w, http.StatusConflict, "only rejected or quarantined games can be appealed"); return
	}
	var req moderationAppealRequest
	if !decodeJSON(w, r, &req) { return }
	reason := strings.TrimSpace(req.Reason)
	if reason == "" || len(reason) > 1000 {
		writeErr(w, http.StatusBadRequest, "appeal reason must contain 1–1000 characters"); return
	}
	s.socialMu.Lock()
	defer s.socialMu.Unlock()
	for _, prior := range s.reports {
		if prior.GameID == gameID && prior.Reporter == creator && prior.Status == "open" && strings.HasPrefix(prior.Reason, "creator_appeal:") {
			writeJSON(w, http.StatusConflict, map[string]any{"error":"an appeal is already open for this game", "report_id":prior.ID}); return
		}
	}
	now := time.Now().UTC()
	appeal := socialReport{ID:pipeline.NewID(), GameID:gameID, Reporter:creator, Reason:"creator_appeal: "+reason, CreatedAt:now, Status:"open"}
	if err := persistModerationAudit(moderationAuditEvent{ID:pipeline.NewID(), Action:"appeal_submitted", GameID:gameID, ReportID:appeal.ID, ActorID:creator, Status:"open", Reason:"creator appeal submitted", CreatedAt:now}); err != nil {
		writeErr(w, http.StatusServiceUnavailable, "could not persist moderation audit"); return
	}
	s.reports = append(s.reports, appeal)
	s.persistSocialStateLocked()
	writeJSON(w, http.StatusCreated, map[string]any{"appeal_id":appeal.ID, "status":appeal.Status, "message":"appeal queued for moderator review"})
}

func (s *Server) handleModerationContact(w http.ResponseWriter, r *http.Request) {
	url := strings.TrimSpace(os.Getenv("DQ_MODERATION_CONTACT_URL"))
	email := strings.TrimSpace(os.Getenv("DQ_MODERATION_CONTACT_EMAIL"))
	writeJSON(w, http.StatusOK, map[string]any{
		"contact_url":url, "contact_email":email,
		"appeals_endpoint":"POST /v1/games/{id}/moderation-appeals",
		"external_contact_configured":url != "" || email != "",
	})
}
