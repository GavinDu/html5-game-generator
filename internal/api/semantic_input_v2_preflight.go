package api

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"

	"doodlequest/internal/gameir"
	"doodlequest/internal/replaytrace"
)

// POST /v1/games/{id}/semantic-input-v2/validate
//
// This endpoint verifies syntax, published Game IR input bindings, declared
// schema and immutable evidence identity. It must never persist a run,
// issue a score verdict, or grant leaderboard rank. The *future* server-owned
// interpreter and attestor are separate release gates.
func (s *Server) handleValidateSemanticInputV2(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Vary", "Authorization, X-User-ID")
	game, allowed := s.deliverableGame(w, r)
	if !allowed {
		return
	}
	if game.GameIR == nil || gameir.Validate(game.GameIR) != nil {
		writeErr(w, http.StatusUnprocessableEntity, "published Game IR is unavailable or invalid")
		return
	}

	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(query) != 2 || len(query["irSha256"]) != 1 || len(query["seed"]) != 1 {
		writeErr(w, http.StatusBadRequest, "exact irSha256 and seed query parameters are required")
		return
	}
	fullIRSHA256 := irSHA256Of(game.GameIR)
	if len(fullIRSHA256) != 64 {
		writeErr(w, http.StatusServiceUnavailable, "unable to identify published Game IR")
		return
	}
	if query.Get("irSha256") != fullIRSHA256 {
		writeErr(w, http.StatusConflict, "semantic input is for a different Game IR version")
		return
	}
	seedRaw := query.Get("seed")
	seedValue, err := strconv.ParseUint(seedRaw, 10, 32)
	if err != nil || strconv.FormatUint(seedValue, 10) != seedRaw {
		writeErr(w, http.StatusBadRequest, "semantic input seed must be canonical uint32")
		return
	}
	if r.Body == nil {
		writeErr(w, http.StatusBadRequest, "semantic input body is required")
		return
	}
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeErr(w, http.StatusUnsupportedMediaType, "semantic input requires application/json")
		return
	}
	// The maximum is enforced *during read* so a large network request
	// cannot allocate memory before the strict wire grammar runs.
	r.Body = http.MaxBytesReader(w, r.Body, int64(replaytrace.MaxEncodedTraceBytes))
	data, err := io.ReadAll(r.Body)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeErr(w, http.StatusRequestEntityTooLarge, "semantic input trace too large")
		} else {
			writeErr(w, http.StatusBadRequest, "unable to read semantic input trace")
		}
		return
	}
	trace, err := replaytrace.DecodeStrictV2(game.GameIR, data)
	if err != nil {
		// Do not echo sensitive raw input or document internals to callers.
		writeErr(w, http.StatusBadRequest, "invalid semantic input v2 trace")
		return
	}
	digest, err := replaytrace.CanonicalSHA256(
		game.GameIR, game.ID, fullIRSHA256, uint32(seedValue), trace,
	)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid semantic input identity")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"schema":        "semantic-input-admission-v2",
		"gameId":        game.ID,
		"irSha256":      fullIRSHA256,
		"seed":          uint32(seedValue),
		"traceSha256":   digest,
		"durationTicks": trace.DurationTicks,
		"eventCount":    len(trace.Events),
		"evidenceStatus": "unverified",
		"rankingScope":   "none",
		"attested":       false,
	})
}
