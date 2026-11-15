package api

import (
    "net/http"
)

// Validating the opaque ID before any lookup bounds work and prevents query,
// URL-escape, fragment and slash variants from becoming different identities.
func validVerifiedRunLookupID(id string) bool {
    if len(id) == 0 || len(id) > 128 {
        return false
    }
    for i := 0; i < len(id); i++ {
        c := id[i]
        if (c >= 'a' && c <= 'z') ||
            (c >= 'A' && c <= 'Z') ||
            (c >= '0' && c <= '9') ||
            c == '-' || c == '_' || c == '.' || c == '~' {
            continue
        }
        return false
    }
    return true
}

// GET /v1/runs/{id}/result is the iOS host's dedicated, read-only
// server-attested result contract. The backend does NOT yet have an
// independent replay executor or server-owned verdict ledger. It MUST NOT
// infer any trusted outcome from RunRecord.Verified, a browser score, or an
// evidence fingerprint, including when such values look plausible.
//
// Reject unknown or unauthorized recordings indistinguishably. For a
// currently deliverable recording, report that *trusted verification* is
// unavailable; never fabricate a verified result or promote client values.
// A positive response requires a separate reviewed attestor integration.
func (s *Server) handleGetVerifiedRunResult(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Cache-Control", "private, no-store")
    w.Header().Set("Vary", "Authorization, X-User-ID")

    id := r.PathValue("id")
    if !validVerifiedRunLookupID(id) || r.URL.RawQuery != "" {
        writeErr(w, http.StatusNotFound, "verified run result unavailable")
        return
    }

    // Store only the minimal game reference. Do not expose trace, score,
    // player identity, historical Verified or ActualScore to this endpoint.
    gameID := ""
    s.runsMu.Lock()
    for _, run := range s.runs {
        if run.ID == id {
            gameID = run.GameID
            break
        }
    }
    s.runsMu.Unlock()
    if gameID == "" {
        writeErr(w, http.StatusNotFound, "verified run result unavailable")
        return
    }

    game, ok := s.store.GetGame(gameID)
    if !ok || !s.canDeliver(game, userID(r)) {
        writeErr(w, http.StatusNotFound, "verified run result unavailable")
        return
    }

    // Explicit non-success prevents native clients from decoding a
    // historical or caller-controlled "verified" response. The route
    // must stay unavailable until a server-only, versioned verdict
    // binds full immutable IR, seed, ordered inputs and score.
    w.Header().Set("Retry-After", "3600")
    writeErr(w, http.StatusServiceUnavailable, "independent replay attestation unavailable")
}
