package api

import (
    "net/http"
    "strings"
    "testing"
    "time"
)

// Modern records carry the full Game IR hash. A different published IR
// revision that shares the legacy 12-hex board prefix must never inherit
// the current game's provisional rank, even if its own evidence is intact.
func TestProvisionalBoardRequiresFullCurrentIRForModernRun(t *testing.T) {
    srv := newRunsServer(t)
    const gameID = "sample-rainbow-bear"
    code, response := postRun(t, srv, gameID,
        `{"name":"current-version","score":5,"won":true,"durationMs":20000,"seed":42,"inputs":[{"tick":10,"type":"pointer","detail":"195,700"}]}`)
    if code != http.StatusCreated {
        t.Fatalf("current modern run not created: %d %v", code, response)
    }
    cleanID, _ := response["runId"].(string)
    if cleanID == "" { t.Fatal("missing modern server run ID") }

    srv.runsMu.Lock()
    if len(srv.runs) != 1 {
        srv.runsMu.Unlock()
        t.Fatal("expected exactly one admitted current run")
    }
    modern := srv.runs[0]
    if runEvidenceStatus(&modern) != "unverified" || len(modern.IRSHA256) != 64 {
        srv.runsMu.Unlock()
        t.Fatalf("current modern evidence missing full digest: %+v", modern)
    }
    alternate := modern.IRSHA256[:12] + strings.Repeat("f", 52)
    if alternate == modern.IRSHA256 {
        alternate = modern.IRSHA256[:12] + strings.Repeat("0", 52)
    }
    if alternate == modern.IRSHA256 { t.Fatal("unable to construct second full IR hash") }

    foreign := modern
    foreign.ID = "foreign-ir-same-board-prefix"
    foreign.Name = "foreign-colliding-version"
    foreign.IRSHA256 = alternate
    foreign.At = modern.At.Add(-time.Minute) // Would win any tied time/score board.
    var err error
    foreign.ReplayInputSHA256, err = canonicalRunInputSHA256(
        foreign.GameID, foreign.IRSHA256, foreign.Seed, foreign.DurationMs, foreign.Inputs,
    )
    if err != nil {
        srv.runsMu.Unlock()
        t.Fatalf("cannot create self-consistent foreign replay: %v", err)
    }
    if runEvidenceStatus(&foreign) != "unverified" ||
       foreign.IRHash != modern.IRHash {
        srv.runsMu.Unlock()
        t.Fatalf("foreign version is not an internally consistent prefix collision: %+v", foreign)
    }

    legacy := modern
    legacy.ID = "legacy-short-ir-only"
    legacy.Name = "old-browser-compatible"
    legacy.IRSHA256 = ""
    legacy.ReplayInputSHA256 = ""
    legacy.EvidenceSchema = ""
    legacy.At = modern.At.Add(time.Minute)
    fromOldClient := true
    legacy.Verified = &fromOldClient

    srv.runs = append(srv.runs, foreign, legacy)
    srv.runsMu.Unlock()

    board := getBoard(t, srv, gameID, "all")
    entries, ok := board["entries"].([]any)
    if !ok || len(entries) != 2 {
        t.Fatalf("foreign full-hash version appeared or legacy disappeared: %v", board)
    }
    for index, raw := range entries {
        row := raw.(map[string]any)
        if row["rank"] != float64(index+1) {
            t.Fatalf("unexpected rank gap after foreign-version filtering: %+v", row)
        }
        if row["rankingScope"] != "provisional" {
            t.Fatalf("no client run may become verified through hash consistency: %+v", row)
        }
        if _, found := row["verified"]; found {
            t.Fatalf("historical client verdict leaked into board: %+v", row)
        }
    }
    current := entries[0].(map[string]any)
    historical := entries[1].(map[string]any)
    if current["runId"] != cleanID || current["evidenceStatus"] != "unverified" ||
       historical["runId"] != legacy.ID || historical["evidenceStatus"] != "legacy_unverified" {
        t.Fatalf("unexpected full-hash board partition and legacy fallback: %+v", entries)
    }
    // This is a board-view filter, not an edit to the persisted diagnostic
    // trace or its canonical SHA. Keep the foreign version for audit only.
    srv.runsMu.Lock()
    defer srv.runsMu.Unlock()
    if len(srv.runs) != 3 ||
       srv.runs[1].IRSHA256 != alternate ||
       runEvidenceStatus(&srv.runs[1]) != "unverified" {
        t.Fatalf("filter rewrote foreign historical evidence: %+v", srv.runs)
    }
}
