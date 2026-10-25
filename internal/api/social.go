package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"doodlequest/internal/model"
	"doodlequest/internal/pipeline"
)

type commentIdempotencyRecord struct {
	UserID  string        `json:"user_id"`
	GameID  string        `json:"game_id"`
	Body    string        `json:"body"`
	Comment socialComment `json:"comment"`
}

type socialComment struct {
	ID        string    `json:"id"`
	GameID    string    `json:"game_id"`
	UserID    string    `json:"user_id"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}
type socialReport struct {
	ID               string     `json:"id"`
	GameID           string     `json:"game_id"`
	Reporter         string     `json:"reporter_id"`
	Reason           string     `json:"reason"`
	CreatedAt        time.Time  `json:"created_at"`
	Status           string     `json:"status"`
	ResolvedAt       *time.Time `json:"resolved_at,omitempty"`
	ResolvedBy       string     `json:"resolved_by,omitempty"`
	ResolutionReason string     `json:"resolution_reason,omitempty"`
}
type socialEvent struct {
	EventID    string    `json:"event_id,omitempty"`
	UserID     string    `json:"user_id"`
	GameID     string    `json:"game_id"`
	Kind       string    `json:"kind"`
	DurationMS int       `json:"duration_ms,omitempty"`
	ExpectedMS int       `json:"expected_duration_ms,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}
type publishPostRequest struct {
	Caption       string   `json:"caption"`
	Topics        []string `json:"topics"`
	RemixParentID string   `json:"remix_parent_id,omitempty"`
}
type textRequest struct {
	Body string `json:"body"`
}
type reactionRequest struct {
	Reaction string `json:"reaction"`
}
type reportRequest struct {
	Reason string `json:"reason"`
}
type moderationRequest struct {
	Status string `json:"status"`
	Reason string `json:"reason"`
}
type socialFeedbackRequest struct {
	Kind string `json:"kind"`
}
type socialEventRequest struct {
	EventID    string `json:"event_id"`
	GameID     string `json:"game_id"`
	Kind       string `json:"kind"`
	DurationMS int    `json:"duration_ms"`
	ExpectedMS int    `json:"expected_duration_ms"`
}

type socialStateSnapshot struct {
	MutedTopics        map[string]map[string]bool          `json:"muted_topics"`
	CommentIdempotency map[string]commentIdempotencyRecord `json:"comment_idempotency"`
	Reactions          map[string]map[string]string        `json:"reactions"`
	Comments           map[string][]socialComment          `json:"comments"`
	Saves              map[string]map[string]bool          `json:"saves"`
	Blocks             map[string]map[string]bool          `json:"blocks"`
	NotInterested      map[string]map[string]bool          `json:"not_interested"`
	Reports            []socialReport                      `json:"reports"`
	Events             []socialEvent                       `json:"events"`
	// Follows were the one social edge missing from the snapshot
	// (architecture review 2026-10-23): every other relation survived
	// a restart, follow graphs silently reset.
	Follows map[string]map[string]struct{} `json:"follows"`
}

func socialStatePath() string {
	if path := strings.TrimSpace(os.Getenv("DQ_SOCIAL_STATE_FILE")); path != "" {
		return path
	}
	return filepath.Join("telemetry", "social-state.json")
}
func socialEventsPath() string {
	if path := strings.TrimSpace(os.Getenv("DQ_SOCIAL_EVENTS_FILE")); path != "" {
		return path
	}
	return filepath.Join("telemetry", "social-events.jsonl")
}
func persistSocialEvent(ev socialEvent) {
	data, err := json.Marshal(ev)
	if err != nil {
		return
	}
	path := socialEventsPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	_, _ = f.Write(append(data, '\n'))
	_ = f.Close()
}

func (s *Server) loadSocialState() {
	data, err := os.ReadFile(socialStatePath())
	if err == nil {
		var state socialStateSnapshot
		if json.Unmarshal(data, &state) == nil {
			s.socialMu.Lock()
			if state.Reactions != nil {
				s.reactions = state.Reactions
			}
			if state.Comments != nil {
				s.comments = state.Comments
			}
			if state.Saves != nil {
				s.saves = state.Saves
			}
			if state.Blocks != nil {
				s.blocks = state.Blocks
			}
			if state.NotInterested != nil {
				s.notInterested = state.NotInterested
			}
			if state.MutedTopics != nil {
				s.mutedTopics = state.MutedTopics
			}
			if state.CommentIdempotency != nil {
				s.commentIdempotency = state.CommentIdempotency
			}
			s.reports = state.Reports
			s.events = state.Events
			if state.Follows != nil {
				s.follows = state.Follows
			}
			s.socialMu.Unlock()
		}
	}
	// Events use append-only JSONL so a single impression doesn't rewrite all social state.
	raw, err := os.ReadFile(socialEventsPath())
	if err == nil {
		lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
		if len(lines) > 20000 {
			lines = lines[len(lines)-20000:]
		}
		loaded := make([]socialEvent, 0, len(lines))
		for _, line := range lines {
			var ev socialEvent
			if json.Unmarshal([]byte(line), &ev) == nil {
				loaded = append(loaded, ev)
			}
		}
		s.socialMu.Lock()
		s.events = append(s.events, loaded...)
		if len(s.events) > 20000 {
			s.events = append([]socialEvent(nil), s.events[len(s.events)-20000:]...)
		}
		s.socialMu.Unlock()
	}
}

func (s *Server) persistSocialStateLocked() error {
	state := socialStateSnapshot{Reactions: s.reactions, Comments: s.comments, Saves: s.saves, Blocks: s.blocks, NotInterested: s.notInterested, MutedTopics: s.mutedTopics, CommentIdempotency: s.commentIdempotency, Reports: s.reports, Follows: s.follows}
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	path := socialStatePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (s *Server) registerSocialRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/users/me/following", s.handleListFollowing)
	mux.HandleFunc("GET /v1/users/me/saved", s.handleSavedGames)
	mux.HandleFunc("GET /v1/creators", s.handleDiscoverCreators)
	mux.HandleFunc("GET /v1/users/{id}", s.handleCreatorProfile)
	mux.HandleFunc("POST /v1/users/{id}/follow", s.handleFollow)
	mux.HandleFunc("DELETE /v1/users/{id}/follow", s.handleUnfollow)
	mux.HandleFunc("POST /v1/users/{id}/block", s.handleBlock)
	mux.HandleFunc("DELETE /v1/users/{id}/block", s.handleUnblock)
	mux.HandleFunc("POST /v1/games/{id}/publish", s.handlePublishPost)
	mux.HandleFunc("GET /v1/games/{id}/post", s.handleGetPost)
	mux.HandleFunc("POST /v1/games/{id}/reactions", s.handleSetReaction)
	mux.HandleFunc("DELETE /v1/games/{id}/reactions", s.handleDeleteReaction)
	mux.HandleFunc("GET /v1/games/{id}/comments", s.handleListComments)
	mux.HandleFunc("POST /v1/games/{id}/comments", s.handleAddComment)
	mux.HandleFunc("DELETE /v1/games/{id}/comments/{commentID}", s.handleDeleteComment)
	mux.HandleFunc("POST /v1/games/{id}/save", s.handleSaveGame)
	mux.HandleFunc("DELETE /v1/games/{id}/save", s.handleUnsaveGame)
	mux.HandleFunc("POST /v1/games/{id}/report", s.handleReportGame)
	mux.HandleFunc("POST /v1/games/{id}/moderation-appeals", s.handleModerationAppeal)
	mux.HandleFunc("GET /v1/moderation/contact", s.handleModerationContact)
	mux.HandleFunc("POST /v1/games/{id}/feedback", s.handleSocialFeedback)
	mux.HandleFunc("GET /v1/users/me/preferences", s.handleGetPreferences)
	mux.HandleFunc("PUT /v1/users/me/preferences/topics", s.handleSetTopicPreferences)
	mux.HandleFunc("DELETE /v1/users/me/preferences", s.handleResetPreferences)
	mux.HandleFunc("POST /v1/games/{id}/moderation", s.handleModerateGame)
	mux.HandleFunc("GET /v1/moderation/reports", s.handleListReports)
	mux.HandleFunc("GET /v1/moderation/audit", s.handleModerationAudit)
	mux.HandleFunc("POST /v1/moderation/reports/{id}/resolve", s.handleResolveReport)
	mux.HandleFunc("POST /v1/telemetry/social-event", s.handleSocialEvent)
}

func (s *Server) handleListFollowing(w http.ResponseWriter, r *http.Request) {
	viewer := strings.TrimSpace(userID(r))
	if s.socialGraphErr != nil {
		writeErr(w, http.StatusServiceUnavailable, "shared social graph is unavailable")
		return
	}
	if s.socialGraph != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		ids, err := s.socialGraph.ListFollowing(ctx, viewer)
		cancel()
		if err != nil {
			writeErr(w, http.StatusServiceUnavailable, "could not read shared social graph")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"creator_ids": ids})
		return
	}
	s.socialMu.RLock()
	ids := make([]string, 0, len(s.follows[viewer]))
	for id := range s.follows[viewer] {
		ids = append(ids, id)
	}
	s.socialMu.RUnlock()
	sort.Strings(ids)
	writeJSON(w, http.StatusOK, map[string]any{"creator_ids": ids})
}
func (s *Server) handleFollow(w http.ResponseWriter, r *http.Request) {
	viewer, creator := strings.TrimSpace(userID(r)), strings.TrimSpace(r.PathValue("id"))
	if creator == "" || creator == viewer {
		writeErr(w, http.StatusBadRequest, "a different creator ID is required")
		return
	}
	if !s.hasPublicCreator(creator) {
		writeErr(w, http.StatusNotFound, "creator not found")
		return
	}
	if s.socialGraphErr != nil {
		writeErr(w, http.StatusServiceUnavailable, "shared social graph is unavailable")
		return
	}
	if s.socialGraph != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		err := s.socialGraph.Follow(ctx, viewer, creator)
		cancel()
		if err != nil {
			writeErr(w, http.StatusBadRequest, "could not create follow relationship")
			return
		}
	}
	s.socialMu.Lock()
	if s.follows[viewer] == nil {
		s.follows[viewer] = map[string]struct{}{}
	}
	s.follows[viewer][creator] = struct{}{}
	s.persistSocialStateLocked()
	s.socialMu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"following": true, "creator_id": creator})
}
func (s *Server) handleUnfollow(w http.ResponseWriter, r *http.Request) {
	viewer, creator := strings.TrimSpace(userID(r)), strings.TrimSpace(r.PathValue("id"))
	if s.socialGraphErr != nil {
		writeErr(w, http.StatusServiceUnavailable, "shared social graph is unavailable")
		return
	}
	if s.socialGraph != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		err := s.socialGraph.Unfollow(ctx, viewer, creator)
		cancel()
		if err != nil {
			writeErr(w, http.StatusServiceUnavailable, "could not update shared social graph")
			return
		}
	}
	s.socialMu.Lock()
	delete(s.follows[viewer], creator)
	s.persistSocialStateLocked()
	s.socialMu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"following": false, "creator_id": creator})
}
func (s *Server) hasPublicCreator(id string) bool {
	for _, g := range s.store.ListGames() {
		if g.OwnerID == id && g.Status == model.GameReady && g.Visibility == model.VisibilityPublic && !g.Quarantined && g.ModerationStatus != "pending" && g.ModerationStatus != "rejected" {
			return true
		}
	}
	return false
}

type creatorDiscoveryEntry struct {
	CreatorID string   `json:"creator_id"`
	PostCount int      `json:"post_count"`
	Topics    []string `json:"topics"`
}

// handleDiscoverCreators lists creators with eligible public games.
func (s *Server) handleDiscoverCreators(w http.ResponseWriter, r *http.Request) {
	query := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	topic := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(r.URL.Query().Get("topic"), "#")))
	limit := 20
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 50 {
			writeErr(w, http.StatusBadRequest, "limit must be between 1 and 50")
			return
		}
		limit = n
	}
	cursor := strings.TrimSpace(r.URL.Query().Get("cursor"))
	type aggregate struct {
		count  int
		topics map[string]string
		titles []string
	}
	byCreator := map[string]*aggregate{}
	for _, game := range s.store.ListGames() {
		if game.Status != model.GameReady || game.Visibility != model.VisibilityPublic ||
			game.Quarantined || game.ModerationStatus == "pending" || game.ModerationStatus == "rejected" {
			continue
		}
		entry := byCreator[game.OwnerID]
		if entry == nil {
			entry = &aggregate{topics: map[string]string{}}
			byCreator[game.OwnerID] = entry
		}
		entry.count++
		entry.titles = append(entry.titles, game.Title)
		for _, raw := range game.Topics {
			display := strings.TrimSpace(strings.TrimPrefix(raw, "#"))
			normalized := strings.ToLower(display)
			if normalized != "" {
				if _, exists := entry.topics[normalized]; !exists {
					entry.topics[normalized] = display
				}
			}
		}
	}
	ids := make([]string, 0, len(byCreator))
	for id, entry := range byCreator {
		if id == "" || (cursor != "" && id <= cursor) {
			continue
		}
		if query != "" {
			matched := strings.Contains(strings.ToLower(id), query)
			for _, title := range entry.titles {
				if strings.Contains(strings.ToLower(title), query) {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		if topic != "" {
			if _, exists := entry.topics[topic]; !exists {
				continue
			}
		}
		ids = append(ids, id)
	}
	sort.Strings(ids)
	hasMore := len(ids) > limit
	if hasMore {
		ids = ids[:limit]
	}
	creators := make([]creatorDiscoveryEntry, 0, len(ids))
	for _, id := range ids {
		entry := byCreator[id]
		topics := make([]string, 0, len(entry.topics))
		for _, value := range entry.topics {
			topics = append(topics, value)
		}
		sort.Strings(topics)
		creators = append(creators, creatorDiscoveryEntry{
			CreatorID: id,
			PostCount: entry.count,
			Topics:    topics,
		})
	}
	nextCursor := ""
	if hasMore && len(ids) > 0 {
		nextCursor = ids[len(ids)-1]
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"creators":    creators,
		"next_cursor": nextCursor,
	})
}

func (s *Server) handleCreatorProfile(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	var games []*model.Game
	for _, g := range s.store.ListGames() {
		if g.OwnerID == id && g.Status == model.GameReady && g.Visibility == model.VisibilityPublic && !g.Quarantined && g.ModerationStatus != "rejected" && g.ModerationStatus != "pending" {
			games = append(games, g)
		}
	}
	s.socialMu.RLock()
	followers, following := 0, len(s.follows[id])
	for _, edges := range s.follows {
		if _, ok := edges[id]; ok {
			followers++
		}
	}
	viewer := userID(r)
	_, isFollowing := s.follows[viewer][id]
	_, followsBack := s.follows[id][viewer]
	s.socialMu.RUnlock()
	writeJSON(w, http.StatusOK, map[string]any{"creator_id": id, "posts": games, "post_count": len(games), "follower_count": followers, "following_count": following, "is_following": isFollowing, "is_friend": isFollowing && followsBack})
}
func (s *Server) handlePublishPost(w http.ResponseWriter, r *http.Request) {
	var req publishPostRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	g, ok := s.store.GetGame(r.PathValue("id"))
	if !ok {
		writeErr(w, 404, "game not found")
		return
	}
	if g.OwnerID != userID(r) {
		writeErr(w, 403, "only the creator can publish this game")
		return
	}
	if g.Status != model.GameReady || g.Quarantined {
		writeErr(w, 409, "game must be ready and not quarantined")
		return
	}
	if req.RemixParentID != "" {
		parent, exists := s.store.GetGame(req.RemixParentID)
		if !exists || parent.Status != model.GameReady || parent.Visibility != model.VisibilityPublic || parent.Quarantined || parent.ModerationStatus == "pending" || parent.ModerationStatus == "rejected" {
			writeErr(w, 400, "remix_parent_id must identify an eligible public game")
			return
		}
	}
	if g.PublishedAt != nil {
		writeErr(w, 409, "published versions are immutable; create a new version to edit a post")
		return
	}
	if len(req.Caption) > 1000 {
		writeErr(w, 400, "caption must be at most 1000 characters")
		return
	}
	topics := make([]string, 0, len(req.Topics))
	seen := map[string]bool{}
	for _, t := range req.Topics {
		t = strings.TrimSpace(strings.TrimPrefix(t, "#"))
		if t == "" {
			continue
		}
		if len(t) > 40 {
			writeErr(w, 400, "topic must be at most 40 characters")
			return
		}
		k := strings.ToLower(t)
		if !seen[k] {
			topics = append(topics, t)
			seen[k] = true
		}
		if len(topics) > 10 {
			writeErr(w, 400, "at most 10 topics are allowed")
			return
		}
	}
	original := *g
	if g.Manifest != nil {
		manifest := *g.Manifest
		original.Manifest = &manifest
	}
	now := time.Now().UTC()
	g.Caption = strings.TrimSpace(req.Caption)
	g.Topics = topics
	g.RemixParentID = strings.TrimSpace(req.RemixParentID)
	g.PublishedAt = &now
	g.PublishedVersion = 1
	g.ModerationStatus = "pending"
	g.Visibility = model.VisibilityPublic
	if g.Manifest != nil {
		g.Manifest.Visibility = model.VisibilityPublic
	}
	g.UpdatedAt = now
	flags := screenPublication(g)
	var report *socialReport
	if len(flags) > 0 {
		now := time.Now().UTC()
		report = &socialReport{
			ID: pipeline.NewID(), GameID: g.ID, Reporter: "automated-screening",
			Reason:    "automated_screening: " + strings.Join(flags, ","),
			CreatedAt: now, Status: "open",
		}
		if err := persistModerationAudit(moderationAuditEvent{
			ID: pipeline.NewID(), Action: "automated_screening_flagged",
			GameID: g.ID, ReportID: report.ID, ActorID: "automated-screening",
			Status: "open", Reason: strings.Join(flags, ","), CreatedAt: now,
		}); err != nil {
			writeErr(w, http.StatusServiceUnavailable, "could not persist moderation audit")
			return
		}
	}
	// Screening audit must be durable before the immutable publish mutation.
	// Otherwise an audit-write failure leaves the game pending and immutable
	// with no moderator queue item, and the creator cannot retry publication.
	s.store.UpdateGame(g)
	if report != nil {
		s.socialMu.Lock()
		s.reports = append(s.reports, *report)
		if err := s.persistSocialStateLocked(); err != nil {
			s.reports = s.reports[:len(s.reports)-1]
			s.socialMu.Unlock()
			s.store.UpdateGame(&original)
			writeErr(w, http.StatusServiceUnavailable, "could not persist moderation queue")
			return
		}
		s.socialMu.Unlock()
	}
	writeJSON(w, 202, map[string]any{"post": g, "moderation_status": "pending"})
}
func (s *Server) handleGetPost(w http.ResponseWriter, r *http.Request) {
	g, ok := s.store.GetGame(r.PathValue("id"))
	if !ok || g.Status != model.GameReady || g.Visibility != model.VisibilityPublic || g.Quarantined || g.ModerationStatus == "rejected" || g.ModerationStatus == "pending" {
		writeErr(w, 404, "published post not found")
		return
	}
	writeJSON(w, 200, map[string]any{"post": g, "engagement": s.engagement(g.ID)})
}
func (s *Server) handleSetReaction(w http.ResponseWriter, r *http.Request) {
	var req reactionRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	allowed := map[string]bool{"like": true, "love": true, "funny": true, "wow": true}
	if !allowed[req.Reaction] {
		writeErr(w, 400, "reaction must be like, love, funny, or wow")
		return
	}
	if !s.socialGameAvailableFor(r.PathValue("id"), userID(r)) {
		writeErr(w, 404, "game not found")
		return
	}
	s.socialMu.Lock()
	if s.reactions[r.PathValue("id")] == nil {
		s.reactions[r.PathValue("id")] = map[string]string{}
	}
	s.reactions[r.PathValue("id")][userID(r)] = req.Reaction
	s.persistSocialStateLocked()
	s.socialMu.Unlock()
	writeJSON(w, 200, map[string]any{"reaction": req.Reaction})
}
func (s *Server) handleDeleteReaction(w http.ResponseWriter, r *http.Request) {
	s.socialMu.Lock()
	delete(s.reactions[r.PathValue("id")], userID(r))
	s.persistSocialStateLocked()
	s.socialMu.Unlock()
	writeJSON(w, 200, map[string]any{"reaction": ""})
}
func (s *Server) handleListComments(w http.ResponseWriter, r *http.Request) {
	if !s.socialGameAvailableFor(r.PathValue("id"), userID(r)) {
		writeErr(w, 404, "game not found")
		return
	}
	s.socialMu.RLock()
	out := append([]socialComment(nil), s.comments[r.PathValue("id")]...)
	s.socialMu.RUnlock()
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	page, next, err := paginateComments(out, r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"comments": page, "next_cursor": next})
}

func paginateComments(items []socialComment, r *http.Request) ([]socialComment, string, error) {
	limit := 20
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 50 {
			return nil, "", fmt.Errorf("limit must be between 1 and 50")
		}
		limit = n
	}
	start := 0
	if cursor := r.URL.Query().Get("cursor"); cursor != "" {
		found := false
		for i, c := range items {
			if c.ID == cursor {
				start = i + 1
				found = true
				break
			}
		}
		if !found {
			return nil, "", fmt.Errorf("cursor does not match a comment")
		}
	}
	if start >= len(items) {
		return []socialComment{}, "", nil
	}
	end := start + limit
	if end > len(items) {
		end = len(items)
	}
	page := items[start:end]
	next := ""
	if end < len(items) && len(page) > 0 {
		next = page[len(page)-1].ID
	}
	return page, next, nil
}
func (s *Server) handleAddComment(w http.ResponseWriter, r *http.Request) {
	var req textRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	body := strings.TrimSpace(req.Body)
	if body == "" || len(body) > 2000 {
		writeErr(w, 400, "comment must contain 1–2000 characters")
		return
	}
	id := r.PathValue("id")
	if !s.socialGameAvailableFor(id, userID(r)) {
		writeErr(w, 404, "game not found")
		return
	}
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if len(key) > 128 {
		writeErr(w, http.StatusBadRequest, "Idempotency-Key must be at most 128 characters")
		return
	}
	viewer := userID(r)
	s.socialMu.Lock()
	if key != "" {
		if prior, ok := s.commentIdempotency[key]; ok {
			if prior.UserID != viewer || prior.GameID != id || prior.Body != body {
				s.socialMu.Unlock()
				writeErr(w, http.StatusConflict, "Idempotency-Key was already used for a different comment")
				return
			}
			s.socialMu.Unlock()
			writeJSON(w, http.StatusCreated, map[string]any{"comment": prior.Comment, "idempotent_replay": true})
			return
		}
	}
	c := socialComment{ID: pipeline.NewID(), GameID: id, UserID: viewer, Body: body, CreatedAt: time.Now().UTC()}
	s.comments[id] = append(s.comments[id], c)
	if key != "" {
		if s.commentIdempotency == nil {
			s.commentIdempotency = map[string]commentIdempotencyRecord{}
		}
		s.commentIdempotency[key] = commentIdempotencyRecord{UserID: viewer, GameID: id, Body: body, Comment: c}
	}
	s.persistSocialStateLocked()
	s.socialMu.Unlock()
	writeJSON(w, 201, map[string]any{"comment": c})
}
func (s *Server) handleDeleteComment(w http.ResponseWriter, r *http.Request) {
	gameID, commentID, viewer := r.PathValue("id"), r.PathValue("commentID"), userID(r)
	s.socialMu.Lock()
	defer s.socialMu.Unlock()
	items := s.comments[gameID]
	for i, c := range items {
		if c.ID != commentID {
			continue
		}
		if c.UserID != viewer {
			writeErr(w, http.StatusForbidden, "only the comment author can delete it")
			return
		}
		s.comments[gameID] = append(items[:i], items[i+1:]...)
		s.persistSocialStateLocked()
		writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
		return
	}
	writeErr(w, http.StatusNotFound, "comment not found")
}

func (s *Server) handleSaveGame(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !s.socialGameAvailableFor(id, userID(r)) {
		writeErr(w, 404, "game not found")
		return
	}
	s.socialMu.Lock()
	if s.saves[userID(r)] == nil {
		s.saves[userID(r)] = map[string]bool{}
	}
	s.saves[userID(r)][id] = true
	s.persistSocialStateLocked()
	s.socialMu.Unlock()
	writeJSON(w, 200, map[string]any{"saved": true})
}
func (s *Server) handleUnsaveGame(w http.ResponseWriter, r *http.Request) {
	s.socialMu.Lock()
	delete(s.saves[userID(r)], r.PathValue("id"))
	s.persistSocialStateLocked()
	s.socialMu.Unlock()
	writeJSON(w, 200, map[string]any{"saved": false})
}
func (s *Server) handleSavedGames(w http.ResponseWriter, r *http.Request) {
	s.socialMu.RLock()
	ids := make([]string, 0, len(s.saves[userID(r)]))
	for id := range s.saves[userID(r)] {
		ids = append(ids, id)
	}
	s.socialMu.RUnlock()
	sort.Strings(ids)
	games := make([]*model.Game, 0, len(ids))
	for _, id := range ids {
		if s.socialGameAvailableFor(id, userID(r)) {
			g, _ := s.store.GetGame(id)
			games = append(games, g)
		}
	}
	writeJSON(w, 200, map[string]any{"games": games})
}
func (s *Server) handleReportGame(w http.ResponseWriter, r *http.Request) {
	var req reportRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" || len(reason) > 500 {
		writeErr(w, 400, "a report reason of 1–500 characters is required")
		return
	}
	if _, ok := s.store.GetGame(r.PathValue("id")); !ok {
		writeErr(w, 404, "game not found")
		return
	}
	report := socialReport{ID: pipeline.NewID(), GameID: r.PathValue("id"), Reporter: userID(r), Reason: reason, CreatedAt: time.Now().UTC(), Status: "open"}
	if err := persistModerationAudit(moderationAuditEvent{ID: pipeline.NewID(), Action: "report_created", GameID: report.GameID, ReportID: report.ID, ActorID: report.Reporter, Reason: reason, CreatedAt: report.CreatedAt}); err != nil {
		writeErr(w, http.StatusServiceUnavailable, "could not persist moderation audit")
		return
	}
	s.socialMu.Lock()
	s.reports = append(s.reports, report)
	s.persistSocialStateLocked()
	s.socialMu.Unlock()
	writeJSON(w, 201, map[string]any{"report_id": report.ID, "status": report.Status})
}
func (s *Server) handleBlock(w http.ResponseWriter, r *http.Request) {
	viewer := userID(r)
	creator := strings.TrimSpace(r.PathValue("id"))
	if creator == "" || creator == viewer {
		writeErr(w, http.StatusBadRequest, "a different creator ID is required")
		return
	}
	if s.socialGraphErr != nil {
		writeErr(w, http.StatusServiceUnavailable, "shared social graph is unavailable")
		return
	}
	if s.socialGraph != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		err := s.socialGraph.Block(ctx, viewer, creator)
		cancel()
		if err != nil {
			writeErr(w, http.StatusServiceUnavailable, "could not persist block relation")
			return
		}
	}
	s.socialMu.Lock()
	if s.blocks[viewer] == nil {
		s.blocks[viewer] = map[string]bool{}
	}
	s.blocks[viewer][creator] = true
	_ = s.persistSocialStateLocked()
	s.socialMu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"blocked": true, "creator_id": creator})
}

func (s *Server) handleUnblock(w http.ResponseWriter, r *http.Request) {
	viewer := userID(r)
	creator := strings.TrimSpace(r.PathValue("id"))
	if creator == "" || creator == viewer {
		writeErr(w, http.StatusBadRequest, "a different creator ID is required")
		return
	}
	if s.socialGraphErr != nil {
		writeErr(w, http.StatusServiceUnavailable, "shared social graph is unavailable")
		return
	}
	if s.socialGraph != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		err := s.socialGraph.Unblock(ctx, viewer, creator)
		cancel()
		if err != nil {
			writeErr(w, http.StatusServiceUnavailable, "could not persist block relation")
			return
		}
	}
	s.socialMu.Lock()
	delete(s.blocks[viewer], creator)
	_ = s.persistSocialStateLocked()
	s.socialMu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"blocked": false, "creator_id": creator})
}
func (s *Server) handleSocialFeedback(w http.ResponseWriter, r *http.Request) {
	var req socialFeedbackRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Kind != "not_interested" {
		writeErr(w, 400, "kind must be not_interested")
		return
	}
	id := r.PathValue("id")
	if !s.socialGameAvailableFor(id, userID(r)) {
		writeErr(w, 404, "game not found")
		return
	}
	s.socialMu.Lock()
	if s.notInterested[userID(r)] == nil {
		s.notInterested[userID(r)] = map[string]bool{}
	}
	s.notInterested[userID(r)][id] = true
	s.persistSocialStateLocked()
	s.socialMu.Unlock()
	writeJSON(w, 200, map[string]any{"accepted": true})
}

type topicPreferencesRequest struct {
	MutedTopics []string `json:"muted_topics"`
}

func (s *Server) handleGetPreferences(w http.ResponseWriter, r *http.Request) {
	viewer := userID(r)
	s.socialMu.RLock()
	muted := make([]string, 0, len(s.mutedTopics[viewer]))
	for topic, value := range s.mutedTopics[viewer] {
		if value {
			muted = append(muted, topic)
		}
	}
	ignored := make([]string, 0, len(s.notInterested[viewer]))
	for id, value := range s.notInterested[viewer] {
		if value {
			ignored = append(ignored, id)
		}
	}
	s.socialMu.RUnlock()
	sort.Strings(muted)
	sort.Strings(ignored)
	writeJSON(w, http.StatusOK, map[string]any{"muted_topics": muted, "not_interested_game_ids": ignored})
}

func (s *Server) handleSetTopicPreferences(w http.ResponseWriter, r *http.Request) {
	var req topicPreferencesRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if len(req.MutedTopics) > 100 {
		writeErr(w, http.StatusBadRequest, "at most 100 muted topics are allowed")
		return
	}
	muted := map[string]bool{}
	for _, raw := range req.MutedTopics {
		topic := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(raw, "#")))
		if topic == "" || len(topic) > 40 {
			writeErr(w, http.StatusBadRequest, "each topic must contain 1–40 characters")
			return
		}
		muted[topic] = true
	}
	viewer := userID(r)
	s.socialMu.Lock()
	if s.mutedTopics == nil {
		s.mutedTopics = map[string]map[string]bool{}
	}
	s.mutedTopics[viewer] = muted
	s.persistSocialStateLocked()
	s.socialMu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"muted_topics": req.MutedTopics})
}

func (s *Server) handleResetPreferences(w http.ResponseWriter, r *http.Request) {
	viewer := userID(r)
	s.socialMu.Lock()
	delete(s.mutedTopics, viewer)
	delete(s.notInterested, viewer)
	s.persistSocialStateLocked()
	s.socialMu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"reset": true})
}
func (s *Server) handleModerateGame(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(os.Getenv("DQ_MODERATION_TOKEN"))
	if token == "" {
		writeErr(w, http.StatusServiceUnavailable, "moderation is not configured; set DQ_MODERATION_TOKEN")
		return
	}
	if r.Header.Get("X-Moderation-Token") != token {
		writeErr(w, http.StatusForbidden, "invalid moderation token")
		return
	}
	var req moderationRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	switch req.Status {
	case "approved", "rejected", "quarantined":
	default:
		writeErr(w, 400, "status must be approved, rejected, or quarantined")
		return
	}
	g, ok := s.store.GetGame(r.PathValue("id"))
	if !ok {
		writeErr(w, 404, "game not found")
		return
	}
	moderationReason := strings.TrimSpace(req.Reason)
	if err := persistModerationAudit(moderationAuditEvent{ID: pipeline.NewID(), Action: "game_moderation_changed", GameID: g.ID, ActorID: userID(r), Status: req.Status, Reason: moderationReason, CreatedAt: time.Now().UTC()}); err != nil {
		writeErr(w, http.StatusServiceUnavailable, "could not persist moderation audit")
		return
	}
	g.ModerationStatus = req.Status
	g.QuarantineReason = moderationReason
	g.UpdatedAt = time.Now().UTC()
	if req.Status == "quarantined" {
		g.Quarantined = true
	}
	if req.Status == "approved" {
		g.Quarantined = false
		g.QuarantineReason = ""
	}
	s.store.UpdateGame(g)
	writeJSON(w, 200, map[string]any{"game_id": g.ID, "moderation_status": g.ModerationStatus})
}
func (s *Server) handleSocialEvent(w http.ResponseWriter, r *http.Request) {
	var req socialEventRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	allowed := map[string]bool{"impression": true, "play_start": true, "completion": true, "replay": true, "share": true, "save": true, "follow": true, "quick_exit": true}
	if !allowed[req.Kind] {
		writeErr(w, 400, "unsupported social event kind")
		return
	}
	if req.DurationMS < 0 || req.ExpectedMS < 0 {
		writeErr(w, 400, "durations cannot be negative")
		return
	}
	req.EventID = strings.TrimSpace(req.EventID)
	if len(req.EventID) > 128 {
		writeErr(w, 400, "event_id must be at most 128 characters")
		return
	}
	if _, ok := s.store.GetGame(req.GameID); !ok {
		writeErr(w, 404, "game not found")
		return
	}
	viewer := userID(r)
	s.socialMu.Lock()
	// Client-supplied event IDs make retries safe. Dedupe is intentionally
	// bounded to the in-memory event window; legacy clients without IDs
	// retain the original at-least-once behavior.
	if req.EventID != "" {
		for _, prior := range s.events {
			if prior.UserID == viewer && prior.EventID == req.EventID {
				s.socialMu.Unlock()
				w.WriteHeader(http.StatusAccepted)
				return
			}
		}
	}
	ev := socialEvent{EventID: req.EventID, UserID: viewer, GameID: req.GameID, Kind: req.Kind, DurationMS: req.DurationMS, ExpectedMS: req.ExpectedMS, CreatedAt: time.Now().UTC()}
	s.events = append(s.events, ev)
	if len(s.events) > 20000 {
		s.events = append([]socialEvent(nil), s.events[len(s.events)-20000:]...)
	}
	persistSocialEvent(ev)
	s.socialMu.Unlock()
	w.WriteHeader(http.StatusAccepted)
}
func (s *Server) socialGameAvailableFor(id, viewer string) bool {
	g, ok := s.store.GetGame(id)
	if !ok || g.Status != model.GameReady || g.Visibility != model.VisibilityPublic || g.Quarantined || g.ModerationStatus == "pending" || g.ModerationStatus == "rejected" {
		return false
	}
	return !s.isBlocked(viewer, g.OwnerID)
}
func (s *Server) isBlocked(viewer, creator string) bool {
	s.socialMu.RLock()
	defer s.socialMu.RUnlock()
	return s.blocks[viewer][creator]
}
func (s *Server) engagement(gameID string) map[string]any {
	s.socialMu.RLock()
	defer s.socialMu.RUnlock()
	return map[string]any{"reaction_count": len(s.reactions[gameID]), "comment_count": len(s.comments[gameID]), "event_count": countGameEvents(s.events, gameID)}
}
func countGameEvents(events []socialEvent, gameID string) int {
	n := 0
	for _, ev := range events {
		if ev.GameID == gameID {
			n++
		}
	}
	return n
}

func (s *Server) handleResolveReport(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(os.Getenv("DQ_MODERATION_TOKEN"))
	if token == "" {
		writeErr(w, http.StatusServiceUnavailable, "moderation is not configured; set DQ_MODERATION_TOKEN")
		return
	}
	if r.Header.Get("X-Moderation-Token") != token {
		writeErr(w, http.StatusForbidden, "invalid moderation token")
		return
	}
	id := r.PathValue("id")
	reason := strings.TrimSpace(r.URL.Query().Get("reason"))
	if len(reason) > 500 {
		writeErr(w, http.StatusBadRequest, "resolution reason must be at most 500 characters")
		return
	}
	s.socialMu.Lock()
	defer s.socialMu.Unlock()
	for i := range s.reports {
		if s.reports[i].ID == id {
			// Keep the first resolution as the audit record; repeated requests
			// are safe and cannot rewrite who resolved the report or why.
			if s.reports[i].Status != "resolved" {
				now := time.Now().UTC()
				if err := persistModerationAudit(moderationAuditEvent{ID: pipeline.NewID(), Action: "report_resolved", GameID: s.reports[i].GameID, ReportID: id, ActorID: userID(r), Status: "resolved", Reason: reason, CreatedAt: now}); err != nil {
					writeErr(w, http.StatusServiceUnavailable, "could not persist moderation audit")
					return
				}
				s.reports[i].Status = "resolved"
				s.reports[i].ResolvedAt = &now
				s.reports[i].ResolvedBy = userID(r)
				s.reports[i].ResolutionReason = reason
				s.persistSocialStateLocked()
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"report_id":         id,
				"status":            s.reports[i].Status,
				"resolved_at":       s.reports[i].ResolvedAt,
				"resolved_by":       s.reports[i].ResolvedBy,
				"resolution_reason": s.reports[i].ResolutionReason,
			})
			return
		}
	}
	writeErr(w, http.StatusNotFound, "report not found")
}

func (s *Server) handleListReports(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(os.Getenv("DQ_MODERATION_TOKEN"))
	if token == "" {
		writeErr(w, http.StatusServiceUnavailable, "moderation is not configured; set DQ_MODERATION_TOKEN")
		return
	}
	if r.Header.Get("X-Moderation-Token") != token {
		writeErr(w, http.StatusForbidden, "invalid moderation token")
		return
	}
	limit := 100
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 500 {
			writeErr(w, http.StatusBadRequest, "limit must be between 1 and 500")
			return
		}
		limit = n
	}
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	if status != "" && status != "open" && status != "resolved" {
		writeErr(w, http.StatusBadRequest, "status must be open or resolved")
		return
	}
	gameID := strings.TrimSpace(r.URL.Query().Get("game_id"))
	s.socialMu.RLock()
	reports := make([]socialReport, 0, len(s.reports))
	for _, report := range s.reports {
		if status != "" && report.Status != status {
			continue
		}
		if gameID != "" && report.GameID != gameID {
			continue
		}
		reports = append(reports, report)
	}
	s.socialMu.RUnlock()
	sort.SliceStable(reports, func(i, j int) bool { return reports[i].CreatedAt.After(reports[j].CreatedAt) })
	if len(reports) > limit {
		reports = reports[:limit]
	}
	if reports == nil {
		reports = []socialReport{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"reports": reports})
}

func (s *Server) recordSocialPlay(viewer, gameID string) {
	s.socialMu.Lock()
	kind := "play_start"
	for _, prior := range s.events {
		if prior.UserID == viewer && prior.GameID == gameID && (prior.Kind == "play_start" || prior.Kind == "replay") {
			kind = "replay"
			break
		}
	}
	ev := socialEvent{UserID: viewer, GameID: gameID, Kind: kind, CreatedAt: time.Now().UTC()}
	s.events = append(s.events, ev)
	if len(s.events) > 20000 {
		s.events = append([]socialEvent(nil), s.events[len(s.events)-20000:]...)
	}
	persistSocialEvent(ev)
	s.socialMu.Unlock()
}

func (s *Server) recordSocialCompletion(viewer, gameID string, durationMS int) {
	s.socialMu.Lock()
	ev := socialEvent{UserID: viewer, GameID: gameID, Kind: "completion", DurationMS: durationMS, CreatedAt: time.Now().UTC()}
	s.events = append(s.events, ev)
	if len(s.events) > 20000 {
		s.events = append([]socialEvent(nil), s.events[len(s.events)-20000:]...)
	}
	persistSocialEvent(ev)
	s.socialMu.Unlock()
}
