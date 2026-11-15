package api

import (
	"net/http"
	"net/url"
)

// GET /v1/games/{id}/leaderboard/verified?window=week|all is a separate
// future-proof *authoritative* read surface. Until a server-owned replay
// executor and durable verdict ledger exist, the trustworthy result set is
// empty. Legacy browser-writable Verified fields and provisional board ranks
// must never be copied into this endpoint.
func (s *Server) handleVerifiedOnlyLeaderboard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Vary", "Authorization, X-User-ID")
	game, ok := s.deliverableGame(w, r)
	if !ok {
		return
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(query) > 1 || (len(query) == 1 && len(query["window"]) != 1) {
		writeErr(w, http.StatusBadRequest, "verified leaderboard accepts only an optional single window")
		return
	}
	window := "week"
	if len(query) != 0 {
		window = query.Get("window")
		if window != "week" && window != "all" {
			writeErr(w, http.StatusBadRequest, "verified leaderboard window must be week or all")
			return
		}
	}
	fullIRSHA256 := irSHA256Of(game.GameIR)
	if len(fullIRSHA256) != 64 {
		writeErr(w, http.StatusServiceUnavailable, "published Game IR identity unavailable")
		return
	}
	metric, order := boardMetric(game.GameIR)
	// There is intentionally no call to boardRuns or any historical run
	// materialization. A browser's Verified:true or matching hash is NOT
	// independent server attestation and cannot grant a trusted rank.
	writeJSON(w, http.StatusOK, map[string]any{
		"gameId":               game.ID,
		"irHash":               fullIRSHA256[:12],
		"irSha256":             fullIRSHA256,
		"window":               window,
		"metric":               metric,
		"order":                order,
		"rankingScope":         "verified_only",
		"attestationAvailable": false,
		"entries":              []any{},
	})
}
