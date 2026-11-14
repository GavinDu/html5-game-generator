package api

import (
	"net/http"
	"strings"
	"testing"
)

func TestTrustedReplayInputRecorderModuleServedSafely(t *testing.T) {
	srv := newTestServer()
	rec := do(t, srv, http.MethodGet, "/web/runtime-replay-input.mjs", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("replay input recorder HTTP %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "text/javascript; charset=utf-8" {
		t.Fatalf("unexpected trusted replay module MIME: %q", got)
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("replay module missing nosniff: %q", got)
	}
	body := rec.Body.String()
	for _, required := range []string{
		"MAX_VERIFIABLE_RUN_INPUTS = 2000",
		"export function createRunInputRecorder(",
		"export function indexValidatedReplayInputs(",
		"inputs.length >= maxEvents",
		"complete = false",
	} {
		if !strings.Contains(body, required) {
			t.Errorf("trusted replay input module missing %q", required)
		}
	}
	if strings.Contains(body, "eval(") || strings.Contains(body, "new Function(") {
		t.Fatal("trusted replay input module includes dynamic evaluation")
	}
}
