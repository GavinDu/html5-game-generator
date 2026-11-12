package api

import (
	"net/http"
	"strings"
	"testing"
)

func TestTrustedRuntimeWorkBudgetModuleIsServedWithSafeMIME(t *testing.T) {
	srv := newTestServer()
	rec := do(t, srv, http.MethodGet, "/web/runtime-work-budget.js", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("trusted work budget HTTP %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "text/javascript; charset=utf-8" {
		t.Fatalf("unexpected JavaScript MIME type: %q", got)
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("trusted module missing nosniff: %q", got)
	}
	body := rec.Body.String()
	for _, required := range []string{
		"export function createSimulationWorkBudget(",
		"maxChecks: 32768",
		"maxActions: 8192",
		"rule evaluation work budget exceeded",
		"rule action work budget exceeded",
	} {
		if !strings.Contains(body, required) {
			t.Errorf("trusted work module missing %q", required)
		}
	}
	if strings.Contains(body, "eval(") || strings.Contains(body, "new Function(") {
		t.Fatal("work budget module unexpectedly includes dynamic evaluation")
	}
}
