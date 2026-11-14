package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"doodlequest/internal/pipeline"
)

// A modern run's canonical evidence fingerprint is only a diagnostic
// integrity invariant. A client-supplied score or this SHA must never become
// a trusted verification verdict.
func TestModernPersistedRunEvidenceDriftFailsClosedOnGET(t *testing.T) {
	srv := newRunsServer(t)
	const gameID = "sample-rainbow-bear"
	body := `{"name":"drift-check","score":6,"won":true,"durationMs":20000,"seed":42,"inputs":[{"tick":10,"type":"pointer","detail":"10,20"},{"tick":10,"type":"pointer","detail":"30,40"}]}`
	code, posted := postRun(t, srv, gameID, body)
	if code != http.StatusCreated {
		t.Fatalf("submit modern run: %d %v", code, posted)
	}
	id, ok := posted["runId"].(string)
	if !ok || id == "" {
		t.Fatalf("missing server run identifier: %v", posted)
	}
	srv.runsMu.Lock()
	index := -1
	for i := range srv.runs {
		if srv.runs[i].ID == id {
			index = i
			break
		}
	}
	if index < 0 {
		srv.runsMu.Unlock()
		t.Fatalf("accepted run not in store: %s", id)
	}
	original := srv.runs[index]
	srv.runsMu.Unlock()
	if len(original.ReplayInputSHA256) != 64 || len(original.IRSHA256) != 64 ||
		!persistedReplayEvidenceConsistent(&original) {
		t.Fatalf("modern run missing canonical admitted evidence: %+v", original)
	}
	get := func() *httptest.ResponseRecorder {
		t.Helper()
		rec := httptest.NewRecorder()
		srv.Routes().ServeHTTP(rec,
			httptest.NewRequest(http.MethodGet, "/v1/runs/"+id, nil))
		return rec
	}
	if rec := get(); rec.Code != http.StatusOK {
		t.Fatalf("authentic run fetch: %d %s", rec.Code, rec.Body.String())
	}

	tests := []struct {
		name   string
		change func(*RunRecord)
	}{
		{"reordered-same-tick", func(r *RunRecord) { r.Inputs[0], r.Inputs[1] = r.Inputs[1], r.Inputs[0] }},
		{"removed-input", func(r *RunRecord) { r.Inputs = r.Inputs[:1] }},
		{"changed-coordinate", func(r *RunRecord) { r.Inputs[0].Detail = "99,20" }},
		{"changed-seed", func(r *RunRecord) { r.Seed++ }},
		{"changed-duration", func(r *RunRecord) { r.DurationMs++ }},
		{"changed-game", func(r *RunRecord) { r.GameID = "another-game" }},
		{"mismatched-board-short-hash", func(r *RunRecord) { r.IRHash = "ffffffffffff" }},
		{"mismatched-full-hash", func(r *RunRecord) { r.IRSHA256 = strings.Repeat("f", 64) }},
		{"tampered-replay-digest", func(r *RunRecord) { r.ReplayInputSHA256 = strings.Repeat("0", 64) }},
		{"malformed-replay-digest", func(r *RunRecord) { r.ReplayInputSHA256 = "short" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			altered := original
			altered.Inputs = append([]RunInput(nil), original.Inputs...)
			test.change(&altered)
			if persistedReplayEvidenceConsistent(&altered) {
				t.Fatalf("tampered replay still matches persisted canonical evidence: %+v", altered)
			}
			// Game-ID corruption may be rejected by delivery authorization
			// before the digest guard; the important invariant is that no
			// tampered input data is served.
			srv.runsMu.Lock()
			srv.runs[index] = altered
			srv.runsMu.Unlock()
			rec := get()
			want := http.StatusConflict
			if test.name == "changed-game" {
				want = http.StatusNotFound
			}
			if rec.Code != want || !strings.Contains(rec.Header().Get("Cache-Control"), "no-store") {
				t.Fatalf("tampered run GET must fail closed: %d (%s) %s", rec.Code,
					rec.Header().Get("Cache-Control"), rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), "replayInputSha256") ||
				strings.Contains(rec.Body.String(), "99,20") {
				t.Fatalf("corrupted private replay evidence leaked: %s", rec.Body.String())
			}
			srv.runsMu.Lock()
			srv.runs[index] = original
			srv.runsMu.Unlock()
		})
	}

	// Historical evidence predates the new digest and cannot be reconstructed
	// into a trusted verification. Permit diagnostic legacy playback only.
	legacy := original
	legacy.IRSHA256 = ""
	legacy.ReplayInputSHA256 = ""
	legacy.EvidenceSchema = ""
	srv.runsMu.Lock()
	srv.runs[index] = legacy
	srv.runsMu.Unlock()
	rec := get()
	if rec.Code != http.StatusOK {
		t.Fatalf("legacy diagnostic record must remain readable: %d %s", rec.Code, rec.Body.String())
	}
	var decoded map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"irSha256", "replayInputSha256", "evidenceSchema", "verified", "actualScore"} {
		if _, exists := decoded[forbidden]; exists {
			t.Fatalf("legacy diagnostic run must not invent %s: %v", forbidden, decoded)
		}
	}
}

func TestModernReplayDriftFromPersistedJSONLRejectedAfterServerRestart(t *testing.T) {
	srv := newRunsServer(t)
	const gameID = "sample-rainbow-bear"
	code, reply := postRun(t, srv, gameID,
		`{"name":"after-restart","score":3,"won":false,"durationMs":18000,"seed":77,"inputs":[{"tick":5,"type":"pointer","detail":"1,2"}]}`)
	if code != http.StatusCreated {
		t.Fatalf("run submit: %d %v", code, reply)
	}
	id, _ := reply["runId"].(string)
	if id == "" {
		t.Fatalf("missing runID: %v", reply)
	}
	var record RunRecord
	srv.runsMu.Lock()
	for _, candidate := range srv.runs {
		if candidate.ID == id {
			record = candidate
		}
	}
	srv.runsMu.Unlock()
	if len(record.ReplayInputSHA256) != 64 {
		t.Fatalf("expected admitted canonical evidence, got %+v", record)
	}
	record.Seed++ // Simulate a bad JSONL edit or incomplete migration.
	raw, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(telemetryDir(), "runs.jsonl")
	if err := os.WriteFile(path, append(raw, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	restarted := New(srv.store, pipeline.NewRunner(srv.store))
	rec := httptest.NewRecorder()
	restarted.Routes().ServeHTTP(rec,
		httptest.NewRequest(http.MethodGet, "/v1/runs/"+id, nil))
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Header().Get("Cache-Control"), "no-store") {
		t.Fatalf("restarted server cannot serve corrupt JSONL: %d %s", rec.Code, rec.Body.String())
	}
}

func TestExplicitModernReplayEvidenceSchemaBlocksMissingFingerprint(t *testing.T) {
	srv := newRunsServer(t)
	const gameID = "sample-rainbow-bear"
	code, posted := postRun(t, srv, gameID,
		`{"name":"schema-pin","score":4,"won":false,"durationMs":15000,"seed":77,"inputs":[{"tick":5,"type":"pointer","detail":"1,2"}]}`)
	if code != http.StatusCreated {
		t.Fatalf("submit versioned replay: %d %v", code, posted)
	}
	id, _ := posted["runId"].(string)
	if id == "" {
		t.Fatalf("missing new run id: %v", posted)
	}
	var original RunRecord
	srv.runsMu.Lock()
	index := -1
	for i, item := range srv.runs {
		if item.ID == id {
			index, original = i, item
			break
		}
	}
	srv.runsMu.Unlock()
	if index < 0 || original.EvidenceSchema != persistedRunEvidenceSchemaV1 ||
		len(original.ReplayInputSHA256) != 64 || !persistedReplayEvidenceConsistent(&original) {
		t.Fatalf("new run must pin schema and fingerprint: %+v", original)
	}
	for _, tc := range []struct {
		name    string
		modify  func(*RunRecord)
		allowed bool
	}{
		{"deleted-digest", func(r *RunRecord) { r.ReplayInputSHA256 = "" }, false},
		{"unsupported-schema", func(r *RunRecord) { r.EvidenceSchema = "replay-input-evidence-v99" }, false},
		{"blank-modern-fingerprint", func(r *RunRecord) { r.ReplayInputSHA256 = "x" }, false},
		{"old-digest-only", func(r *RunRecord) { r.EvidenceSchema = "" }, true},
		{"true-legacy", func(r *RunRecord) {
			r.EvidenceSchema, r.IRSHA256, r.ReplayInputSHA256 = "", "", ""
		}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mutated := original
			tc.modify(&mutated)
			if got := persistedReplayEvidenceConsistent(&mutated); got != tc.allowed {
				t.Fatalf("schema/trace integrity got=%t want=%t: %+v", got, tc.allowed, mutated)
			}
			srv.runsMu.Lock()
			srv.runs[index] = mutated
			srv.runsMu.Unlock()
			req := httptest.NewRequest(http.MethodGet, "/v1/runs/"+id, nil)
			rec := httptest.NewRecorder()
			srv.Routes().ServeHTTP(rec, req)
			wantCode := http.StatusConflict
			if tc.allowed {
				wantCode = http.StatusOK
			}
			if rec.Code != wantCode {
				t.Errorf("stored format gate: HTTP %d, expected %d: %s", rec.Code, wantCode, rec.Body.String())
			}
			if !strings.Contains(rec.Header().Get("Cache-Control"), "no-store") {
				t.Errorf("replay responses may never be cached: %q", rec.Header().Get("Cache-Control"))
			}
			srv.runsMu.Lock()
			srv.runs[index] = original
			srv.runsMu.Unlock()
		})
	}

	// Exercise actual JSONL reload: dropping ONLY the modern hash must be
	// rejected even if a reader knows nothing about the previous process.
	missing := original
	missing.ReplayInputSHA256 = ""
	raw, err := json.Marshal(missing)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(telemetryDir(), "runs.jsonl"),
		append(raw, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	restarted := New(srv.store, pipeline.NewRunner(srv.store))
	rec := httptest.NewRecorder()
	restarted.Routes().ServeHTTP(rec,
		httptest.NewRequest(http.MethodGet, "/v1/runs/"+id, nil))
	if rec.Code != http.StatusConflict {
		t.Fatalf("restarted server admitted modern run with deleted SHA: %d %s",
			rec.Code, rec.Body.String())
	}
}
