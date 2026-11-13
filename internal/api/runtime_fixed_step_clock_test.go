package api

import (
	"net/http"
	"strings"
	"testing"
)

func TestTrustedFixedStepClockModuleServedWithSafeMIME(t *testing.T) {
	srv := newTestServer()
	rec := do(t, srv, http.MethodGet, "/web/runtime-fixed-step-clock.mjs", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("fixed-step clock HTTP %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "text/javascript; charset=utf-8" {
		t.Fatalf("unexpected JavaScript MIME: %q", got)
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("missing trusted module nosniff: %q", got)
	}
	body := rec.Body.String()
	for _, required := range []string{
		"export function createFixedStepClock()",
		"export const FIXED_STEP_MS = 1000 / 60;",
		"export const MAX_CATCHUP_STEPS = 3;",
		"export const MAX_FRAME_DELTA_MS = 250;",
		"accumulatorMS += Math.min(deltaMS, MAX_FRAME_DELTA_MS)",
		"if (steps === MAX_CATCHUP_STEPS && accumulatorMS >= FIXED_STEP_MS)",
	} {
		if !strings.Contains(body, required) {
			t.Errorf("trusted clock module missing %q", required)
		}
	}
	if strings.Contains(body, "eval(") || strings.Contains(body, "new Function(") {
		t.Fatal("trusted clock module unexpectedly contains dynamic evaluation")
	}
}
