package api

import (
	"net/http"

	"doodlequest/internal/model"
)

// Remix is a source-art and Game IR permission, not simply a link to a
// published post. A creator may use their own ready game; other creators
// need an explicit, approved, published opt-in. Four bundled sample games
// are licensed as the app's starter examples and retain that legacy flow.
func isStarterRemixSource(parent *model.Game) bool {
	if parent == nil || parent.OwnerID != "doodlequest" ||
		parent.Visibility != model.VisibilityPublic {
		return false
	}
	switch parent.ID {
	case "sample-rainbow-bear", "sample-ribbon-sky",
		"penguin-heartlight-rescue", "escape-from-injection":
		return true
	default:
		return false
	}
}

func canRemixGame(parent *model.Game, requester string) bool {
	if requester == "" || parent == nil || parent.Status != model.GameReady ||
		parent.Manifest == nil || parent.Quarantined {
		return false
	}
	if parent.OwnerID == requester {
		// An owner may create a private remix of their own ready package;
		// no source assets become publicly visible without a new publish.
		return true
	}
	if parent.Visibility != model.VisibilityPublic ||
		parent.ModerationStatus == "rejected" ||
		parent.ModerationStatus == "pending" {
		return false
	}
	if isStarterRemixSource(parent) {
		return true
	}
	return parent.RemixAllowed && parent.ModerationStatus == "approved" &&
		parent.PublishedAt != nil && parent.PublishedVersion > 0
}

// Client preflight is advisory only: POST /v1/games independently checks
// the same policy. Never expose private parent identity or source metadata.
func (s *Server) handleRemixEligibility(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Vary", "Authorization")
	requester := userID(r)
	if requester == "" {
		writeErr(w, http.StatusUnauthorized, "a verified session is required")
		return
	}
	parent, ok := s.store.GetGame(r.PathValue("id"))
	if !ok || (parent.OwnerID != requester &&
		parent.Visibility != model.VisibilityPublic) {
		writeErr(w, http.StatusNotFound, "game not found")
		return
	}
	eligible := canRemixGame(parent, requester)
	reason := ""
	if !eligible {
		reason = "This game is not available for remixing."
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"eligible": eligible,
		"reason":   reason,
		"game_id":  parent.ID,
	})
}
