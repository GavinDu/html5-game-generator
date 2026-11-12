package api

import (
	"net/http"
	"strings"
	"testing"
)

func TestTrustedMovementPlaneModuleIsServedWithSafeMIME(t *testing.T) {
	srv := newTestServer()
	rec := do(t, srv, http.MethodGet, "/web/runtime-movement-plane.mjs", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("movement plane adapter HTTP %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "text/javascript; charset=utf-8" {
		t.Fatalf("movement adapter MIME type: %q", got)
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("trusted module is missing nosniff: %q", got)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "resolveHorizontalMovement") ||
		!strings.Contains(body, "resolveLegacyHorizontalMovement") {
		t.Fatal("trusted movement module is missing explicit and compatibility dispatch")
	}
	if !strings.Contains(pixiRuntimeJS, "from './runtime-movement-plane.mjs'") {
		t.Fatal("generic interpreter must import the served trusted control adapter")
	}
	if strings.Contains(pixiRuntimeJS, "holes_stitched") {
		t.Fatal("generic interpreter must not branch on a game objective identifier")
	}
}

func TestMovementPlaneModuleCannotBeRequestedAsArbitraryStaticContent(t *testing.T) {
	srv := newTestServer()
	rec := do(t, srv, http.MethodGet, "/web/unregistered-runtime-module.mjs", nil, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unregistered runtime modules must fail closed (status %d)", rec.Code)
	}
}
