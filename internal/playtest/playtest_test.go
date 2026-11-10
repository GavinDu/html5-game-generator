package playtest

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A fake worker script stands in for probe.mjs: it prints a canned report
// and exits 1 (gate failure), which Run must treat as a valid report.
func writeFakeWorker(t *testing.T, body string, exitCode int) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "fake-worker.sh")
	script := "#!/bin/sh\ncat <<'EOF'\n" + body + "\nEOF\nexit " + string(rune('0'+exitCode)) + "\n"
	if err := os.WriteFile(p, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

const cannedReport = `{"base":"http://x","browser":"chromium","games":[
  {"gameId":"g1","verdict":"FAIL","checks":[
    {"checkId":"playtest.boot_clean","status":"fail","evidence":"boom"},
    {"checkId":"simulation.win_reachable","status":"skip","evidence":"unproven"}]},
  {"gameId":"g2","verdict":"PASS","checks":[
    {"checkId":"playtest.boot_clean","status":"pass","evidence":"ok"},
    {"checkId":"playtest.render_nonblank","status":"pass","evidence":"visible"},
    {"checkId":"simulation.no_softlock","status":"pass","evidence":"state changed"}]}
]}`

func TestRunParsesGateFailureReport(t *testing.T) {
	worker := writeFakeWorker(t, cannedReport, 1)
	report, err := Run(context.Background(), Config{
		WorkerCmd: []string{worker},
		BaseURL:   "http://x",
	}, []string{"g1", "g2"})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !report.Failed() {
		t.Fatal("report should be marked failed (g1 verdict FAIL)")
	}
	outcome, ok := report.OutcomeFor("g1")
	if !ok || len(outcome.Checks) != 2 {
		t.Fatalf("outcome for g1: ok=%v checks=%d", ok, len(outcome.Checks))
	}
	if outcome.Checks[0].Status != "fail" {
		t.Fatalf("g1 boot_clean: want fail, got %s", outcome.Checks[0].Status)
	}
	if _, ok := report.OutcomeFor("nope"); ok {
		t.Fatal("unknown game must not produce an outcome")
	}
}

func TestRunAcceptsSkippedInformationalInputTiming(t *testing.T) {
	body := `{"base":"http://x","browser":"chromium","games":[{"gameId":"g1","verdict":"PASS","checks":[
		{"checkId":"playtest.boot_clean","status":"pass"},
		{"checkId":"playtest.render_nonblank","status":"pass"},
		{"checkId":"simulation.no_softlock","status":"pass"},
		{"checkId":"hook.time_to_input","status":"skip","evidence":"late timing is informational"}]}]}`
	worker := writeFakeWorker(t, body, 0)
	report, err := Run(context.Background(), Config{WorkerCmd: []string{worker}, BaseURL: "http://x"}, []string{"g1"})
	if err != nil {
		t.Fatalf("informational timing skip must not invalidate a passing playtest: %v", err)
	}
	if report.Failed() {
		t.Fatal("report with passed hard gates and skipped timing metric must pass")
	}
}

func TestRunRejectsExitOneWithStalePassingJSON(t *testing.T) {
	// A crashed worker can print a structurally valid prior PASS report.
	// Exit 1 may only be used with a real FAIL verdict for the same run.
	body := `{"base":"http://x","browser":"chromium","games":[{"gameId":"g1","verdict":"PASS","checks":[
		{"checkId":"playtest.boot_clean","status":"pass"},
		{"checkId":"playtest.render_nonblank","status":"pass"},
		{"checkId":"simulation.no_softlock","status":"pass"}]}]}`
	worker := writeFakeWorker(t, body, 1)
	if _, err := Run(context.Background(), Config{WorkerCmd: []string{worker}, BaseURL: "http://x"}, []string{"g1"}); err == nil ||
		!strings.Contains(err.Error(), "exited 1 but all reported verdicts were PASS") {
		t.Fatalf("stale worker PASS after failure must fail closed, got %v", err)
	}
}

func TestRunRejectsZeroExitWithReportedFailure(t *testing.T) {
	// Likewise, a broken runner cannot send an explicit FAIL and return
	// success to an outer publish gate.
	body := `{"base":"http://x","browser":"chromium","games":[{"gameId":"g1","verdict":"FAIL","checks":[
		{"checkId":"playtest.boot_clean","status":"fail","evidence":"boot failed"}]}]}`
	worker := writeFakeWorker(t, body, 0)
	if _, err := Run(context.Background(), Config{WorkerCmd: []string{worker}, BaseURL: "http://x"}, []string{"g1"}); err == nil ||
		!strings.Contains(err.Error(), "exited 0 despite a FAIL verdict") {
		t.Fatalf("false zero-exit must fail closed, got %v", err)
	}
}

func TestRunWorkerErrorIsError(t *testing.T) {
	worker := writeFakeWorker(t, "not json", 2)
	if _, err := Run(context.Background(), Config{
		WorkerCmd: []string{worker},
		BaseURL:   "http://x",
	}, nil); err == nil {
		t.Fatal("exit 2 must surface as an error")
	}
}

func TestRunRejectsIncompleteReports(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{name: "empty games", body: `{"base":"http://x","browser":"chromium","games":[]}`},
		{name: "missing base", body: `{"browser":"chromium","games":[{"gameId":"g1","verdict":"PASS"}]}`},
		{name: "missing browser", body: `{"base":"http://x","games":[{"gameId":"g1","verdict":"PASS"}]}`},
		{name: "missing game id", body: `{"base":"http://x","browser":"chromium","games":[{"verdict":"PASS"}]}`},
		{name: "invalid verdict", body: `{"base":"http://x","browser":"chromium","games":[{"gameId":"g1","verdict":"UNKNOWN"}]}`},
		{name: "missing checks", body: `{"base":"http://x","browser":"chromium","games":[{"gameId":"g1","verdict":"PASS"}]}`},
		{name: "invalid check status", body: `{"base":"http://x","browser":"chromium","games":[{"gameId":"g1","verdict":"PASS","checks":[{"checkId":"playtest.boot_clean","status":"maybe"}]}]}`},
		{name: "duplicate game id", body: `{"base":"http://x","browser":"chromium","games":[{"gameId":"g1","verdict":"PASS"},{"gameId":"g1","verdict":"FAIL"}]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			worker := writeFakeWorker(t, tc.body, 0)
			_, err := Run(context.Background(), Config{WorkerCmd: []string{worker}, BaseURL: "http://x"}, nil)
			if err == nil {
				t.Fatal("expected invalid report to be rejected")
			}
		})
	}
}

func TestRunRejectsStalePassReportForAnotherServer(t *testing.T) {
	body := `{"base":"https://old.example.test:8443/games","browser":"chromium","games":[{"gameId":"g1","verdict":"PASS","checks":[
		{"checkId":"playtest.boot_clean","status":"pass"},
		{"checkId":"playtest.render_nonblank","status":"pass"},
		{"checkId":"simulation.no_softlock","status":"pass"}]}]}`
	worker := writeFakeWorker(t, body, 0)
	_, err := Run(context.Background(), Config{
		WorkerCmd: []string{worker},
		BaseURL:   "https://new.example.test:8443/games",
	}, []string{"g1"})
	if err == nil || !strings.Contains(err.Error(), `worker reported base "https://old.example.test:8443/games"`) {
		t.Fatalf("a stale PASS for a different server must be rejected, got %v", err)
	}
}

func TestRunAcceptsWorkerCanonicalTrailingSlashInRequestedBase(t *testing.T) {
	body := `{"base":"http://x","browser":"chromium","games":[{"gameId":"g1","verdict":"PASS","checks":[
		{"checkId":"playtest.boot_clean","status":"pass"},
		{"checkId":"playtest.render_nonblank","status":"pass"},
		{"checkId":"simulation.no_softlock","status":"pass"}]}]}`
	worker := writeFakeWorker(t, body, 0)
	if _, err := Run(context.Background(), Config{
		WorkerCmd: []string{worker}, BaseURL: "http://x/",
	}, []string{"g1"}); err != nil {
		t.Fatalf("worker strips one trailing slash; equivalent base must pass: %v", err)
	}
}

func TestRunAcceptsMatchingVersionBoundIRReport(t *testing.T) {
	hash := strings.Repeat("a", 64)
	body := `{"base":"http://x","browser":"chromium","games":[{"gameId":"g1","irHash":"` + hash + `","verdict":"PASS","checks":[
		{"checkId":"playtest.boot_clean","status":"pass"},
		{"checkId":"playtest.render_nonblank","status":"pass"},
		{"checkId":"simulation.no_softlock","status":"pass"}]}]}`
	worker := writeFakeWorker(t, body, 0)
	_, err := Run(context.Background(), Config{
		WorkerCmd:       []string{worker},
		BaseURL:         "http://x",
		ExpectedIRHashes: map[string]string{"g1": hash},
	}, []string{"g1"})
	if err != nil {
		t.Fatalf("matching version-bound real report must pass: %v", err)
	}
}

func TestRunRejectsStaleOrMissingGameIRHash(t *testing.T) {
	actualHash := strings.Repeat("a", 64)
	expectedHash := strings.Repeat("b", 64)
	body := `{"base":"http://x","browser":"chromium","games":[{"gameId":"g1","irHash":"` + actualHash + `","verdict":"PASS","checks":[
		{"checkId":"playtest.boot_clean","status":"pass"},
		{"checkId":"playtest.render_nonblank","status":"pass"},
		{"checkId":"simulation.no_softlock","status":"pass"}]}]}`
	for _, tc := range []struct {
		name string
		body string
		want string
	}{
		{"stale exact-version report", body, "reported IR hash"},
		{"missing IR hash", strings.Replace(body, `"irHash":"` + actualHash + `",`, "", 1), "reported IR hash"},
		{"malformed hash", strings.Replace(body, actualHash, "not-a-digest", 1), "invalid irHash"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			worker := writeFakeWorker(t, tc.body, 0)
			_, err := Run(context.Background(), Config{
				WorkerCmd:       []string{worker},
				BaseURL:         "http://x",
				ExpectedIRHashes: map[string]string{"g1": expectedHash},
			}, []string{"g1"})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("untrusted version report should be rejected (%s): %v", tc.name, err)
			}
		})
	}
}

func TestRunRequiresCompleteExpectedIRHashBindings(t *testing.T) {
	// Reject missing/extra version pins before executing the worker.
	for _, cfg := range []Config{
		{WorkerCmd: []string{"missing-worker"}, BaseURL: "http://x", ExpectedIRHashes: map[string]string{"g2": strings.Repeat("b", 64)}},
		{WorkerCmd: []string{"missing-worker"}, BaseURL: "http://x", ExpectedIRHashes: map[string]string{"g1": "invalid"}},
		{WorkerCmd: []string{"missing-worker"}, BaseURL: "http://x", ExpectedIRHashes: map[string]string{"g1": strings.Repeat("a", 64), "g2": strings.Repeat("b", 64)}},
	} {
		if _, err := Run(context.Background(), cfg, []string{"g1"}); err == nil ||
			!strings.Contains(err.Error(), "expected IR hash") {
			t.Fatalf("expected hash binding must reject missing/extra/invalid map before worker: %v", err)
		}
	}
}

func TestRunRejectsReportForDifferentBrowser(t *testing.T) {
	worker := writeFakeWorker(t, cannedReport, 1)
	_, err := Run(context.Background(), Config{
		WorkerCmd: []string{worker},
		BaseURL:   "http://x",
		Browser:   "webkit",
	}, []string{"g1", "g2"})
	if err == nil || !strings.Contains(err.Error(), `reported browser "chromium", want "webkit"`) {
		t.Fatalf("Run() error = %v, want browser mismatch", err)
	}
}

func TestRunRejectsUnrequestedGamesInBatchReport(t *testing.T) {
	body := `{"base":"http://x","browser":"chromium","games":[
		{"gameId":"g1","verdict":"PASS","checks":[
			{"checkId":"playtest.boot_clean","status":"pass"},
			{"checkId":"playtest.render_nonblank","status":"pass"},
			{"checkId":"simulation.no_softlock","status":"pass"}]},
		{"gameId":"g2","verdict":"PASS","checks":[
			{"checkId":"playtest.boot_clean","status":"pass"},
			{"checkId":"playtest.render_nonblank","status":"pass"},
			{"checkId":"simulation.no_softlock","status":"pass"}]}
	]}`
	worker := writeFakeWorker(t, body, 0)
	_, err := Run(context.Background(), Config{WorkerCmd: []string{worker}, BaseURL: "http://x"}, []string{"g1"})
	if err == nil || !strings.Contains(err.Error(), `unrequested game "g2"`) {
		t.Fatalf("Run() error = %v, want unrequested game rejection", err)
	}
}

func TestRunRejectsReportMissingRequestedGame(t *testing.T) {
	body := `{"base":"http://x","browser":"chromium","games":[{"gameId":"g1","verdict":"PASS","checks":[{"checkId":"playtest.boot_clean","status":"pass"},{"checkId":"playtest.render_nonblank","status":"pass"},{"checkId":"simulation.no_softlock","status":"pass"}]}]}`
	worker := writeFakeWorker(t, body, 0)
	_, err := Run(context.Background(), Config{WorkerCmd: []string{worker}, BaseURL: "http://x"}, []string{"g1", "g2"})
	if err == nil {
		t.Fatal("missing requested game must fail the playtest batch")
	}
	if !strings.Contains(err.Error(), `missing requested game "g2"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunNormalizesRequestedGameIDs(t *testing.T) {
	worker := writeFakeWorker(t, cannedReport, 1)
	report, err := Run(context.Background(), Config{WorkerCmd: []string{worker}, BaseURL: "http://x"}, []string{" g1 ", "g2"})
	if err != nil {
		t.Fatalf("Run() with surrounding whitespace should normalize IDs: %v", err)
	}
	if len(report.Games) != 2 {
		t.Fatalf("report games = %d, want 2", len(report.Games))
	}
}

func TestRunRejectsAmbiguousRequestedGameIDsBeforeStartingWorker(t *testing.T) {
	cases := []struct {
		name string
		ids  []string
		want string
	}{
		{name: "empty", ids: []string{"g1", "  "}, want: "empty ID"},
		{name: "duplicate after trimming", ids: []string{"g1", " g1 "}, want: "duplicate requested game ID"},
		{name: "comma separator", ids: []string{"g1,g2"}, want: "reserved separator"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// The command does not exist; validation must fail before it is launched.
			_, err := Run(context.Background(), Config{WorkerCmd: []string{filepath.Join(t.TempDir(), "missing")}, BaseURL: "http://x"}, tc.ids)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Run() error = %v, want containing %q", err, tc.want)
			}
		})
	}
}

func TestRunWorkerTimeoutIsReportedAsFailure(t *testing.T) {
	worker := filepath.Join(t.TempDir(), "slow-worker.sh")
	if err := os.WriteFile(worker, []byte("#!/bin/sh\nsleep 2\necho should-not-run\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := Run(context.Background(), Config{
		WorkerCmd: []string{worker},
		BaseURL:   "http://x",
		Timeout:   20 * time.Millisecond,
	}, []string{"g1"})
	if err == nil {
		t.Fatal("worker timeout must fail closed instead of returning a playtest report")
	}
	if !strings.Contains(err.Error(), "worker failed") {
		t.Fatalf("timeout error = %v, want worker failure context", err)
	}
}

func TestRunRejectsEmptyBaseURL(t *testing.T) {
	_, err := Run(context.Background(), Config{WorkerCmd: []string{"unused"}}, nil)
	if err == nil {
		t.Fatal("expected empty BaseURL to be rejected")
	}
}

func TestRunRejectsPASSWhenHardGatesAreMissingOrInconclusive(t *testing.T) {
	cases := []struct {
		name   string
		checks string
	}{
		{name: "missing render gate", checks: `{"checkId":"playtest.boot_clean","status":"pass"},{"checkId":"simulation.no_softlock","status":"pass"}`},
		{name: "skipped render gate", checks: `{"checkId":"playtest.boot_clean","status":"pass"},{"checkId":"playtest.render_nonblank","status":"skip"},{"checkId":"simulation.no_softlock","status":"pass"}`},
		{name: "failed boot gate with PASS verdict", checks: `{"checkId":"playtest.boot_clean","status":"fail"},{"checkId":"playtest.render_nonblank","status":"pass"},{"checkId":"simulation.no_softlock","status":"pass"}`},
		{name: "all gates skipped", checks: `{"checkId":"playtest.boot_clean","status":"skip"},{"checkId":"playtest.render_nonblank","status":"skip"},{"checkId":"simulation.no_softlock","status":"skip"}`},
		{name: "duplicate gate", checks: `{"checkId":"playtest.boot_clean","status":"pass"},{"checkId":"playtest.boot_clean","status":"pass"},{"checkId":"simulation.no_softlock","status":"pass"}`},
		{name: "failed shell gate", checks: `{"checkId":"playtest.boot_clean","status":"pass"},{"checkId":"playtest.render_nonblank","status":"pass"},{"checkId":"simulation.no_softlock","status":"pass"},{"checkId":"shell.start_card","status":"fail"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := `{"base":"http://x","browser":"chromium","games":[{"gameId":"g1","verdict":"PASS","checks":[` + tc.checks + `]}]}`
			worker := writeFakeWorker(t, body, 0)
			_, err := Run(context.Background(), Config{WorkerCmd: []string{worker}, BaseURL: "http://x"}, nil)
			if err == nil {
				t.Fatal("PASS report with missing/inconclusive/failed hard gate must be rejected")
			}
		})
	}
}

func TestRunAllowsFAILReportWhenBootFailurePreventsLaterGates(t *testing.T) {
	body := `{"base":"http://x","browser":"chromium","games":[{"gameId":"g1","verdict":"FAIL","checks":[{"checkId":"playtest.boot_clean","status":"fail"}]}]}`
	worker := writeFakeWorker(t, body, 1)
	report, err := Run(context.Background(), Config{WorkerCmd: []string{worker}, BaseURL: "http://x"}, nil)
	if err != nil {
		t.Fatalf("incomplete FAIL report should remain actionable: %v", err)
	}
	if !report.Failed() {
		t.Fatal("failed boot must remain a failed report")
	}
}

func TestRunRejectsPASSWhenAnyMeasuredCheckFailed(t *testing.T) {
	body := `{"base":"http://x","browser":"chromium","games":[{"gameId":"g1","verdict":"PASS","checks":[
		{"checkId":"playtest.boot_clean","status":"pass"},
		{"checkId":"playtest.render_nonblank","status":"pass"},
		{"checkId":"simulation.no_softlock","status":"pass"},
		{"checkId":"simulation.win_reachable","status":"fail","evidence":"win condition unreachable"}
	]}]}`
	worker := writeFakeWorker(t, body, 0)
	_, err := Run(context.Background(), Config{WorkerCmd: []string{worker}, BaseURL: "http://x"}, nil)
	if err == nil || !strings.Contains(err.Error(), `check "simulation.win_reachable" failed`) {
		t.Fatalf("Run() error = %v, want PASS/failed-check inconsistency", err)
	}
}
