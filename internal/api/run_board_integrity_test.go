package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Public provisional ranking must not present modern replay evidence that
// the same server refuses to serve through GET /v1/runs/{id}. Keep the
// original corrupted record for explicit forensic/audit remediation.
func TestLeaderboardDropsInconsistentModernReplayWithoutLaunderingLegacy(t *testing.T) {
	srv := newRunsServer(t)
	const gid = "sample-rainbow-bear"
	code, posted := postRun(t, srv, gid,
		`{"name":"clean-modern","score":4,"won":true,"durationMs":20000,"seed":42,"inputs":[{"tick":10,"type":"pointer","detail":"195,700"}]}`)
	if code != http.StatusCreated {
		t.Fatalf("admitted modern run: status %d body %v", code, posted)
	}
	cleanID, ok := posted["runId"].(string)
	if !ok || cleanID == "" {
		t.Fatalf("missing server minted ID: %v", posted)
	}
	srv.runsMu.Lock()
	if len(srv.runs) != 1 {
		srv.runsMu.Unlock()
		t.Fatal("expected one modern seed run")
	}
	original := srv.runs[0]
	if runEvidenceStatus(&original) != "unverified" {
		srv.runsMu.Unlock()
		t.Fatalf("modern evidence must remain provisional: %s", runEvidenceStatus(&original))
	}

	// All these records retain a matching board/game key; ordinary board
	// sorting would put the 1-second claimed winning scores first.
	changedSeed := original
	changedSeed.ID = "tampered-seed"
	changedSeed.Name = "tampered-player-seed"
	changedSeed.Seed++
	changedSeed.DurationMs = 1000

	missingDigest := original
	missingDigest.ID = "missing-modern-digest"
	missingDigest.Name = "tampered-player-digest"
	missingDigest.ReplayInputSHA256 = ""
	missingDigest.DurationMs = 1000

	unknownSchema := original
	unknownSchema.ID = "unknown-modern-schema"
	unknownSchema.Name = "tampered-player-schema"
	unknownSchema.EvidenceSchema = "invalid-schema"
	unknownSchema.DurationMs = 1000

	// Historical run with no modern fingerprint is *not* independently
	// verified, but must remain visible as a legacy provisional claim.
	legacy := original
	legacy.ID = "historical-browser-claim"
	legacy.Name = "legacy-browser-claim"
	legacy.DurationMs = 22000
	legacy.EvidenceSchema = ""
	legacy.ReplayInputSHA256 = ""
	legacy.IRSHA256 = ""
	clientVerdict := true
	legacy.Verified = &clientVerdict
	legacy.At = time.Now()
	srv.runs = append(srv.runs, changedSeed, missingDigest, unknownSchema, legacy)
	srv.runsMu.Unlock()

	for _, bad := range []RunRecord{changedSeed, missingDigest, unknownSchema} {
		if got := runEvidenceStatus(&bad); got != "invalid_evidence" {
			t.Fatalf("fixture did not produce corrupt modern evidence %s: %s", bad.ID, got)
		}
		req := httptest.NewRequest(http.MethodGet, "/v1/runs/"+bad.ID, nil)
		w := httptest.NewRecorder()
		srv.Routes().ServeHTTP(w, req)
		if w.Code != http.StatusConflict ||
			w.Header().Get("Cache-Control") != "private, no-store" {
			t.Fatalf("corrupted run GET must fail closed: %s %d %s",
				bad.ID, w.Code, w.Body.String())
		}
	}

	board := getBoard(t, srv, gid, "all")
	entries, ok := board["entries"].([]any)
	if !ok || len(entries) != 2 {
		t.Fatalf("only clean and legacy provisional entries may rank: %v", board)
	}
	foundClean, foundLegacy := false, false
	for _, raw := range entries {
		e := raw.(map[string]any)
		name := e["name"].(string)
		if strings.HasPrefix(name, "tampered-") {
			t.Fatalf("corrupted modern replay appeared on the public board: %v", e)
		}
		if e["rankingScope"] != "provisional" {
			t.Fatalf("server did not verify this score; must remain provisional: %v", e)
		}
		if _, leaked := e["verified"]; leaked {
			t.Fatalf("client writable legacy verdict must be suppressed: %v", e)
		}
		switch e["runId"] {
		case cleanID:
			foundClean = true
			if e["evidenceStatus"] != "unverified" || e["rank"] != float64(1) {
				t.Fatalf("clean modern claim should be first provisional entry: %v", e)
			}
		case legacy.ID:
			foundLegacy = true
			if e["evidenceStatus"] != "legacy_unverified" || e["rank"] != float64(2) {
				t.Fatalf("legacy compatibility claim must rank second, never verified: %v", e)
			}
		default:
			t.Fatalf("unexpected run included: %v", e)
		}
	}
	if !foundClean || !foundLegacy {
		t.Fatalf("lost clean/legacy provisional compatibility: %v", entries)
	}

	// Filtering cannot silently rewrite the historical audit evidence.
	srv.runsMu.Lock()
	defer srv.runsMu.Unlock()
	if len(srv.runs) != 5 ||
		runEvidenceStatus(&srv.runs[1]) != "invalid_evidence" ||
		runEvidenceStatus(&srv.runs[2]) != "invalid_evidence" ||
		runEvidenceStatus(&srv.runs[3]) != "invalid_evidence" {
		t.Fatalf("board filtering mutated stored forensic evidence: %+v", srv.runs)
	}
}
