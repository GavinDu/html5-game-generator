package api

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"

	"doodlequest/internal/replaytrace"
	"doodlequest/internal/semanticreplay"
)

// A conservative *process-wide* cost cap on the independent Go replay
// diagnostic. Read-only callers cannot reserve unlimited server workers by
// supplying otherwise valid expensive traces. This is not a per-account
// request-rate policy and must not be mistaken for signed attestation.
var semanticTapDiagnosticSlots = make(chan struct{}, 4)

// POST /v1/games/{id}/semantic-tap-score-v1/diagnose
//
// Authorized, diagnostic-only Go semantic execution of a *tiny* fixed-tick
// tap-to-score capability. This is NOT an attestation endpoint: client seed
// and trace are untrusted, and no run, verified flag, rank or board is written.
func (s *Server) handleDiagnoseSemanticTapScoreV1(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Vary", "Authorization, X-User-ID")
	game, allowed := s.deliverableGame(w, r)
	if !allowed {
		return
	}
	// The whole published IR is a single admission unit: unsupported
	// gameplay must never be partly simulated into a believable score.
	if err := semanticreplay.SupportedIR(game.GameIR); err != nil {
		writeErr(w, http.StatusUnprocessableEntity, "published game is outside the diagnostic-only tap-score capability")
		return
	}
	// Authorize before capacity admission so private games remain 404.
	// Never enqueue an unbounded number of clients awaiting replay slots.
	select {
	case semanticTapDiagnosticSlots <- struct{}{}:
		defer func() { <-semanticTapDiagnosticSlots }()
	default:
		w.Header().Set("Retry-After", "1")
		writeErr(w, http.StatusTooManyRequests, "semantic diagnostic capacity temporarily exhausted")
		return
	}
	// The game ID and digest are server-owned. The query only supplies an
	// expected immutable version and untrusted, canonical diagnostic seed.
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(query) != 2 ||
		len(query["irSha256"]) != 1 || len(query["seed"]) != 1 {
		writeErr(w, http.StatusBadRequest, "exact irSha256 and seed query parameters are required")
		return
	}
	fullIRSHA256 := irSHA256Of(game.GameIR)
	if len(fullIRSHA256) != 64 {
		writeErr(w, http.StatusServiceUnavailable, "published Game IR unavailable")
		return
	}
	if query.Get("irSha256") != fullIRSHA256 {
		writeErr(w, http.StatusConflict, "diagnostic is for a different published Game IR")
		return
	}
	seedRaw := query.Get("seed")
	seed, err := strconv.ParseUint(seedRaw, 10, 32)
	if err != nil || strconv.FormatUint(seed, 10) != seedRaw {
		writeErr(w, http.StatusBadRequest, "seed must be canonical uint32")
		return
	}
	if r.Body == nil {
		writeErr(w, http.StatusBadRequest, "semantic input body required")
		return
	}
	contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || contentType != "application/json" {
		writeErr(w, http.StatusUnsupportedMediaType, "semantic input requires application/json")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, int64(replaytrace.MaxEncodedTraceBytes))
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeErr(w, http.StatusRequestEntityTooLarge, "semantic input exceeds size limit")
		} else {
			writeErr(w, http.StatusBadRequest, "unable to read semantic input")
		}
		return
	}
	trace, err := replaytrace.DecodeStrictV2(game.GameIR, body)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid canonical semantic input")
		return
	}
	// This MUST use the authorized immutable record's ID. IR game.id may
	// be creator-authored metadata and is not an access-control boundary.
	diagnostic, err := semanticreplay.ReplayDiagnosticForRecord(
		game.GameIR, game.ID, trace, uint32(seed),
	)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, "semantic input cannot be diagnosed under the declared capability")
		return
	}
	// Intentionally omit any key named "verified", signed token, runId or
	// leaderboard rank: a deterministic client-provided trace is NOT proof
	// that the game ran or that a browser score was earned.
	writeJSON(w, http.StatusOK, map[string]any{
		"schema":         "semantic-tap-score-diagnostic-v1",
		"proofVersion":   diagnostic.ProofVersion,
		"gameId":         game.ID,
		"irSha256":       fullIRSHA256,
		"seed":           diagnostic.Seed,
		"durationTicks":  diagnostic.DurationTicks,
		"eventCount":     len(trace.Events),
		"traceSha256":    diagnostic.TraceSHA256,
		"stateSha256":    diagnostic.StateSHA256,
		"score":          diagnostic.Score,
		"terminal":       diagnostic.Terminal,
		"downEvents":     diagnostic.DownEvents,
		"actionsApplied": diagnostic.ActionsApplied,
		"evidenceStatus": "unverified",
		"rankingScope":   "none",
		"attested":       false,
	})
}
