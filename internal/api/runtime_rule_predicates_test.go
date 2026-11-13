package api

import (
	"net/http"
	"strings"
	"testing"
)

func TestTrustedRulePredicatesModuleServedWithSafeMIME(t *testing.T) {
	srv := newTestServer()
	rec := do(t, srv, http.MethodGet, "/web/runtime-rule-predicates.mjs", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("trusted rule-predicates module HTTP %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "text/javascript; charset=utf-8" {
		t.Fatalf("rule-predicates MIME: %q", got)
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("rule-predicates module missing nosniff: %q", got)
	}
	for _, term := range []string{"export function conditionsMet", "export function ruleMatches", "lookupSemanticValue"} {
		if !strings.Contains(rec.Body.String(), term) {
			t.Fatalf("trusted predicates missing %q", term)
		}
	}
	if !strings.Contains(pixiRuntimeJS, "from './runtime-rule-predicates.mjs'") {
		t.Fatal("shipping interpreter is not importing pure preinstalled rule semantics")
	}
	if strings.Contains(rec.Body.String(), "eval(") || strings.Contains(rec.Body.String(), "new Function(") {
		t.Fatal("trusted predicates must not evaluate arbitrary authored code")
	}
	rec = do(t, srv, http.MethodGet, "/web/creator-rule-predicates.mjs", nil, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unregistered game-code modules must fail closed: got %d", rec.Code)
	}
}
