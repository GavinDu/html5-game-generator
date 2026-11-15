package api

import (
	"net/http"
	"strings"
	"testing"
)

// Pure state semantics are only preinstalled trusted engine code. Game IR
// cannot choose a module URL or invoke a privileged native control.
func TestRuntimeSemanticStateReducerModuleIsTrustedDataOnly(t *testing.T) {
	srv := newTestServer()
	rec := do(t, srv, http.MethodGet, "/web/runtime-semantic-state-reducer.mjs", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("semantic state module HTTP %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "text/javascript; charset=utf-8" {
		t.Fatalf("untrusted semantic state module MIME: %q", got)
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("semantic state module should refuse MIME sniff: %q", got)
	}
	body := rec.Body.String()
	for _, needle := range []string{
		"export function createSemanticState",
		"export function canonicalSemanticState",
		"export function reduceSemanticStateTick",
		"post-terminal actions rejected",
	} {
		if !strings.Contains(body, needle) {
			t.Errorf("missing trusted semantic reducer contract %q", needle)
		}
	}
	for _, forbidden := range []string{
		"eval(", "new Function(", "document.", "window.", "fetch(",
		"localStorage.", "sessionStorage.", "import(",
		"Math.random(", "Date.now(", "performance.now(",
	} {
		if strings.Contains(body, forbidden) {
			t.Errorf("pure semantic reducer unexpectedly contains %q", forbidden)
		}
	}
}
