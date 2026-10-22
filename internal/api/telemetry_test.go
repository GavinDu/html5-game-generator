package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFingerprintStableAcrossVolatileDetail(t *testing.T) {
	a := &runtimeErrorReport{
		Name:           "TypeError",
		Message:        "Cannot read properties of null (reading 'texture') at entity 12345",
		Stack:          "TypeError: x\n    at makeView (https://example.com/web/pixi-runtime.js:304:14)\n    at createEntity (https://example.com/web/pixi-runtime.js:471:18)",
		RuntimeVersion: "2026.10.22.1",
	}
	b := &runtimeErrorReport{
		Name:           "TypeError",
		Message:        "Cannot read properties of null (reading 'texture') at entity 987",
		Stack:          "TypeError: x\n    at makeView (https://other.host/web/pixi-runtime.js:304:14)\n    at createEntity (https://other.host/web/pixi-runtime.js:471:18)",
		RuntimeVersion: "2026.10.22.1",
	}
	if fingerprintRuntimeError(a) != fingerprintRuntimeError(b) {
		t.Fatalf("same bug with different ids/hosts must share a fingerprint:\n%s\n%s",
			fingerprintRuntimeError(a), fingerprintRuntimeError(b))
	}
	c := &runtimeErrorReport{
		Name:           "TypeError",
		Message:        "a completely different failure",
		Stack:          a.Stack,
		RuntimeVersion: a.RuntimeVersion,
	}
	if fingerprintRuntimeError(a) == fingerprintRuntimeError(c) {
		t.Fatal("different messages must not share a fingerprint")
	}
	d := &runtimeErrorReport{
		Name:           a.Name,
		Message:        a.Message,
		Stack:          a.Stack,
		RuntimeVersion: "2026.10.23.1",
	}
	if fingerprintRuntimeError(a) == fingerprintRuntimeError(d) {
		t.Fatal("different runtime versions must not share a fingerprint")
	}
}

func TestHandleRuntimeError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DQ_TELEMETRY_DIR", dir)
	s := &Server{}

	body := `{"gameId":"sample-rainbow-bear","runtimeVersion":"2026.10.22.1","kind":"boot","name":"TypeError","message":"boom 42","stack":"at x (https://h/web/pixi-runtime.js:1:1)","logTail":["[PKG] hi"]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/telemetry/runtime-error", strings.NewReader(body))
	rec := httptest.NewRecorder()
	s.handleRuntimeError(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("want 202, got %d", rec.Code)
	}
	data, err := os.ReadFile(filepath.Join(dir, "runtime-errors.jsonl"))
	if err != nil {
		t.Fatalf("telemetry file not written: %v", err)
	}
	if !strings.Contains(string(data), `"fingerprint"`) || !strings.Contains(string(data), `"sample-rainbow-bear"`) {
		t.Fatalf("telemetry record missing fields: %s", data)
	}

	bad := httptest.NewRequest(http.MethodPost, "/v1/telemetry/runtime-error", strings.NewReader("{nope"))
	rec2 := httptest.NewRecorder()
	s.handleRuntimeError(rec2, bad)
	if rec2.Code != http.StatusBadRequest {
		t.Fatalf("malformed body: want 400, got %d", rec2.Code)
	}
}
