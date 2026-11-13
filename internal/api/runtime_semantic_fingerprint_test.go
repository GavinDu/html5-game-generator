package api

import (
	"net/http"
	"strings"
	"testing"
)

func TestTrustedSemanticFingerprintModuleServedFromInstalledRuntimeOnly(t *testing.T) {
	srv := newTestServer()
	rec := do(t, srv, http.MethodGet, "/web/runtime-semantic-fingerprint.mjs", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("trusted semantic module HTTP %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "text/javascript; charset=utf-8" {
		t.Fatalf("semantic module MIME: %q", got)
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("semantic module missing nosniff: %q", got)
	}
	for _, term := range []string{
		"export function semanticFingerprintV1",
		"diagnostic-fnv1a64-v1:",
		"MAX_ENTITIES = 512",
	} {
		if !strings.Contains(rec.Body.String(), term) {
			t.Fatalf("trusted semantic module source missing %q", term)
		}
	}
	if !strings.Contains(pixiRuntimeJS, "from './runtime-semantic-fingerprint.mjs'") ||
		!strings.Contains(pixiRuntimeJS, "semanticFingerprint: semanticFingerprintV1") {
		t.Fatal("shipping browser runtime must import and use the installed fingerprint only in probe snapshots")
	}
	if strings.Contains(rec.Body.String(), "eval(") ||
		strings.Contains(rec.Body.String(), "new Function(") {
		t.Fatal("semantic module must never evaluate authored executable code")
	}
	// A diagnostic fingerprint may be compared by browser CI, never used as
	// a native score attestor or promoted into a privileged WebKit command.
	if strings.Contains(rec.Body.String(), "window.webkit") ||
		strings.Contains(rec.Body.String(), "postMessage(") ||
		strings.Contains(pixiRuntimeJS, "sendNativeMetric('semanticFingerprint'") {
		t.Fatal("diagnostic semantics must not expose a native host command channel")
	}
	unknown := do(t, srv, http.MethodGet, "/web/creator-semantic-fingerprint.mjs", nil, "")
	if unknown.Code != http.StatusNotFound {
		t.Fatalf("unknown executable module must fail closed with 404, got %d", unknown.Code)
	}
}
