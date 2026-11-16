package api

import (
	"net/http"
	"strings"
	"testing"
)

func TestTrustedSemanticInputV2RecorderModuleIsSameOriginAndDataOnly(t *testing.T) {
	srv := newTestServer()
	rec := do(t, srv, http.MethodGet, "/web/runtime-semantic-input-v2.mjs", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("semantic v2 recorder HTTP %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "text/javascript; charset=utf-8" {
		t.Fatalf("semantic v2 module MIME is unsafe: %q", got)
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("semantic v2 recorder must disable MIME guessing: %q", got)
	}
	module := rec.Body.String()
	for _, expected := range []string{
		"SEMANTIC_INPUT_V2_SCHEMA = 'semantic-input-trace-v2'",
		"MAX_SEMANTIC_INPUT_V2_EVENTS = 2000",
		"MAX_SEMANTIC_INPUT_V2_TICKS = 108000",
		"MAX_BINDINGS = 32",
		"GAME_IR_ACTION_ID = /^[A-Za-z0-9_-]{1,128}$/",
		"export function createSemanticInputV2Recorder(",
		"record(edge)",
		"finish(durationTicks)",
		"failureReason",
		"if (kinds.has(key))", // preserve Go validator's duplicate (action,input) rejection
		"duplicate semantic input action binding",
	} {
		if !strings.Contains(module, expected) {
			t.Errorf("preinstalled v2 producer module missing %q", expected)
		}
	}
	if strings.Contains(module, "eval(") || strings.Contains(module, "new Function(") {
		t.Fatal("semantic v2 input module must not execute user-supplied code")
	}
}
