package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRunEvidenceProvenanceNeverPromotesHistoricalClientVerdict(t *testing.T) {
	legacyVerified := true
	claimedScore := 777
	legacy := RunRecord{
		GameID: "old-game", IRHash: "abc123abc123",
		Verified: &legacyVerified, ActualScore: &claimedScore,
	}
	if got := runEvidenceStatus(&legacy); got != "legacy_unverified" {
		t.Fatalf("untrusted historical positive verdict must remain legacy: %s", got)
	}
	if got := runEvidenceStatus(nil); got != "invalid_evidence" {
		t.Fatalf("nil record must fail closed: %s", got)
	}
	const digest = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	rec := RunRecord{
		ID: "evidence-run", GameID: "game-1", IRHash: digest[:12], IRSHA256: digest,
		EvidenceSchema: persistedRunEvidenceSchemaV1, DurationMs: 20000,
		Seed: 42, Score: 999, Won: true,
		Verified: &legacyVerified, ActualScore: &claimedScore,
	}
	var err error
	rec.ReplayInputSHA256, err = canonicalRunInputSHA256(
		rec.GameID, rec.IRSHA256, rec.Seed, rec.DurationMs, rec.Inputs)
	if err != nil {
		t.Fatal(err)
	}
	if got := runEvidenceStatus(&rec); got != "unverified" {
		t.Fatalf("digest cannot authorize browser score: %s", got)
	}
	rec.Seed++
	if got := runEvidenceStatus(&rec); got != "invalid_evidence" {
		t.Fatalf("stored seed tampering must invalidate evidence: %s", got)
	}
	rec.Seed--
	rec.EvidenceSchema = "unknown-schema"
	if got := runEvidenceStatus(&rec); got != "invalid_evidence" {
		t.Fatalf("unknown modern evidence marker must fail: %s", got)
	}
}

func TestRunAPIsExposeProvisionalRankAndLegacyStatus(t *testing.T) {
	srv := newRunsServer(t)
	gid := "sample-rainbow-bear"
	code, submitted := postRun(t, srv, gid, "{\"name\":\"untrusted\",\"score\":4,\"won\":true,\"durationMs\":20000,\"seed\":42}")
	if code != http.StatusCreated || submitted["evidenceStatus"] != "unverified" ||
		submitted["rankingScope"] != "provisional" || submitted["verified"] != false {
		t.Fatalf("new browser claim lacks provisional provenance: %d %+v", code, submitted)
	}
	runID, _ := submitted["runId"].(string)
	if runID == "" {
		t.Fatal("missing run ID")
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/runs/"+runID, nil)
	w := httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("run detail unavailable: %d %s", w.Code, w.Body.String())
	}
	var detail map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if detail["evidenceStatus"] != "unverified" || detail["rankingScope"] != "provisional" {
		t.Fatalf("run detail must mark claims provisional: %+v", detail)
	}
	if _, has := detail["verified"]; has {
		t.Fatalf("legacy verdict bit leaked: %+v", detail)
	}
	board := getBoard(t, srv, gid, "all")
	entries := board["entries"].([]any)
	if len(entries) != 1 {
		t.Fatalf("expected one provisional entry: %+v", entries)
	}
	entry := entries[0].(map[string]any)
	if entry["evidenceStatus"] != "unverified" || entry["rankingScope"] != "provisional" {
		t.Fatalf("leaderboard must label client claims: %+v", entry)
	}

	legacyVerified := true
	legacyActualScore := 12
	game, ok := srv.store.GetGame(gid)
	if !ok {
		t.Fatal("inventory game missing")
	}
	legacy := RunRecord{
		ID: "legacy-unsafe-result", GameID: gid, IRHash: irHashOf(game.GameIR),
		Name: "old browser result", Score: 5, Won: true,
		DurationMs: 21000, Seed: 17, At: time.Now(),
		Verified: &legacyVerified, ActualScore: &legacyActualScore,
	}
	srv.runsMu.Lock()
	srv.runs = append(srv.runs, legacy)
	srv.runsMu.Unlock()
	board = getBoard(t, srv, gid, "all")
	entries = board["entries"].([]any)
	found := false
	for _, candidate := range entries {
		e := candidate.(map[string]any)
		if e["runId"] != legacy.ID {
			continue
		}
		found = true
		if e["evidenceStatus"] != "legacy_unverified" || e["rankingScope"] != "provisional" {
			t.Fatalf("historical client verified=true must never be authoritative: %+v", e)
		}
		if _, has := e["verified"]; has {
			t.Fatalf("client-origin legacy verified bit leaked: %+v", e)
		}
	}
	if !found {
		t.Fatal("legacy entry missing from compatibility provisional board")
	}
}
