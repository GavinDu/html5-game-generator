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
    {"checkId":"playtest.boot_clean","status":"pass","evidence":"ok"}]}
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
