package api

import (
	"net/http"
	"strings"
	"testing"
)

func TestTrustedSeededRNGModuleIsServedAndNotExecutableGameContent(t *testing.T) {
	srv := newTestServer()
	rec := do(t, srv, http.MethodGet, "/web/runtime-seeded-rng.mjs", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("seeded RNG module HTTP %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "text/javascript; charset=utf-8" {
		t.Fatalf("incorrect module content type: %q", got)
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("ES module MIME sniffing protection missing: %q", got)
	}
	body := rec.Body.String()
	for _, part := range []string{
		"export function createSeededRandom(",
		"state = (seed >>> 0) || 1;",
		"0x6D2B79F5",
	} {
		if !strings.Contains(body, part) {
			t.Errorf("trusted module missing expected source %q", part)
		}
	}
	if strings.Contains(body, "eval(") || strings.Contains(body, "new Function(") {
		t.Fatal("trusted source module unexpectedly contains dynamic evaluation")
	}
}
