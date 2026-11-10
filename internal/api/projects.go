package api

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"doodlequest/internal/model"
)

// Owner projects are an explicitly authenticated private projection. Do not
// return the full Game/Job: it may contain uploaded art, private Game IR,
// creator source evidence, asset hashes or pipeline internals.
type ownerProjectSummary struct {
	GameID           string           `json:"game_id"`
	Title            string           `json:"title"`
	Status           model.GameStatus `json:"status"`
	Visibility       model.Visibility `json:"visibility"`
	ModerationStatus string           `json:"moderation_status,omitempty"`
	JobStatus        model.JobStatus  `json:"job_status,omitempty"`
	CreatedAt        time.Time        `json:"created_at"`
	UpdatedAt        time.Time        `json:"updated_at"`
	RemixParentID    string           `json:"remix_parent_id,omitempty"`
	PublishedVersion int              `json:"published_version,omitempty"`
	PreviewPosterURL string           `json:"preview_poster_url,omitempty"`
}

type ownerProjectsResponse struct {
	Projects   []ownerProjectSummary `json:"projects"`
	NextCursor *string               `json:"next_cursor"`
}

type ownerProjectCursor struct {
	CreatedAt time.Time `json:"created_at"`
	GameID    string    `json:"game_id"`
}

func encodeOwnerProjectCursor(g *model.Game) string {
	b, _ := json.Marshal(ownerProjectCursor{CreatedAt: g.CreatedAt, GameID: g.ID})
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeOwnerProjectCursor(value string) (ownerProjectCursor, bool) {
	var c ownerProjectCursor
	if len(value) == 0 || len(value) > 512 {
		return c, false
	}
	b, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(b) > 384 || json.Unmarshal(b, &c) != nil ||
		strings.TrimSpace(c.GameID) == "" || len(c.GameID) > 128 {
		return ownerProjectCursor{}, false
	}
	return c, true
}

// GET /v1/users/me/projects?limit=&cursor=&status=
//
// Store.ListGames() has deterministic descending (created_at, game_id)
// ordering. Filter by authenticated ownership *before* selecting a cursor
// window, so no one can enumerate or observe other users' draft games.
func (s *Server) handleMyProjects(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	owner := userID(r)
	if owner == "" {
		writeErr(w, http.StatusUnauthorized, "a verified session is required")
		return
	}
	limit := 20
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 50 {
			writeErr(w, http.StatusBadRequest, "limit must be an integer from 1 to 50")
			return
		}
		limit = n
	}
	filter := strings.TrimSpace(r.URL.Query().Get("status"))
	switch filter {
	case "", "all", "draft", "generating", "ready", "failed":
	default:
		writeErr(w, http.StatusBadRequest, "unsupported project status")
		return
	}
	var cursor ownerProjectCursor
	hasCursor := false
	if raw := r.URL.Query().Get("cursor"); raw != "" {
		var ok bool
		cursor, ok = decodeOwnerProjectCursor(raw)
		if !ok {
			writeErr(w, http.StatusBadRequest, "invalid project cursor")
			return
		}
		hasCursor = true
	}
	// Keep this endpoint's content private even if a proxy ignores the
	// general API cache policy.
	w.Header().Set("Vary", "Authorization")
	games := make([]*model.Game, 0, limit+1)
	for _, g := range s.store.ListGames() {
		if g.OwnerID != owner {
			continue
		}
		if filter != "" && filter != "all" && string(g.Status) != filter {
			continue
		}
		// Lists are ordered newest first. After a cursor, take records
		// strictly older than its created_at or with a greater game ID
		// at the same timestamp. The cursor never identifies an owner's
		// private game to another caller.
		if hasCursor && !g.CreatedAt.Before(cursor.CreatedAt) &&
			!(g.CreatedAt.Equal(cursor.CreatedAt) && g.ID > cursor.GameID) {
			continue
		}
		games = append(games, g)
		if len(games) > limit {
			break
		}
	}
	out := ownerProjectsResponse{Projects: make([]ownerProjectSummary, 0, limit)}
	for _, g := range games {
		if len(out.Projects) >= limit {
			break
		}
		item := ownerProjectSummary{
			GameID: g.ID, Title: g.Title, Status: g.Status,
			Visibility: g.Visibility, ModerationStatus: g.ModerationStatus,
			CreatedAt: g.CreatedAt, UpdatedAt: g.UpdatedAt,
			RemixParentID: g.RemixParentID, PublishedVersion: g.PublishedVersion,
		}
		if job, ok := s.store.GetJobByGame(g.ID); ok {
			item.JobStatus = job.Status
		}
		// Version-bound approved preview metadata only; a merely uploaded
		// asset is not a validated game poster.
		if g.Preview != nil && g.Preview.Status == "ready" &&
			g.Preview.Version == g.PublishedVersion {
			item.PreviewPosterURL = g.Preview.PosterURL
		}
		out.Projects = append(out.Projects, item)
	}
	if len(games) > limit {
		next := encodeOwnerProjectCursor(games[limit-1])
		out.NextCursor = &next
	}
	writeJSON(w, http.StatusOK, out)
}
