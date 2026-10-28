package playtest

import (
	"context"
	"os"
	"path/filepath"
	"testing"
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

func TestRunRejectsEmptyBaseURL(t *testing.T) {
	_, err := Run(context.Background(), Config{WorkerCmd: []string{"unused"}}, nil)
	if err == nil {
		t.Fatal("expected empty BaseURL to be rejected")
	}
}


func TestRunRejectsPASSWhenHardGatesAreMissingOrInconclusive(t *testing.T) {
    cases := []struct {
        name string
        checks string
    }{
        {name: "missing render gate", checks: `{"checkId":"playtest.boot_clean","status":"pass"},{"checkId":"simulation.no_softlock","status":"pass"}`},
        {name: "skipped render gate", checks: `{"checkId":"playtest.boot_clean","status":"pass"},{"checkId":"playtest.render_nonblank","status":"skip"},{"checkId":"simulation.no_softlock","status":"pass"}`},
        {name: "failed boot gate with PASS verdict", checks: `{"checkId":"playtest.boot_clean","status":"fail"},{"checkId":"playtest.render_nonblank","status":"pass"},{"checkId":"simulation.no_softlock","status":"pass"}`},
        {name: "all gates skipped", checks: `{"checkId":"playtest.boot_clean","status":"skip"},{"checkId":"playtest.render_nonblank","status":"skip"},{"checkId":"simulation.no_softlock","status":"skip"}`},
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
