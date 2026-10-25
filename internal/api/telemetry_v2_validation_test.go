package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"doodlequest/internal/pipeline"
	"doodlequest/internal/store"
)

func TestTelemetryV2RejectsInvalidMeasurements(t *testing.T) {
	t.Setenv("DQ_TELEMETRY_DIR", t.TempDir())
	srv := New(store.NewMemoryStore(), pipeline.NewRunner(store.NewMemoryStore()))
	cases := []struct {
		name string
		body string
	}{
		{"empty game", `{"schemaVersion":2,"sessionId":"s","eventId":"e","kind":"session_start"}`},
		{"negative duration", `{"schemaVersion":2,"sessionId":"s","eventId":"e","gameId":"g","kind":"run_end","durationMs":-1}`},
		{"dead inputs exceed inputs", `{"schemaVersion":2,"sessionId":"s","eventId":"e","gameId":"g","kind":"run_end","inputs":2,"deadInputs":3}`},
		{"negative frame bucket", `{"schemaVersion":2,"sessionId":"s","eventId":"e","gameId":"g","kind":"perf_window","buckets":[1,-1]}`},
		{"too many buckets", `{"schemaVersion":2,"sessionId":"s","eventId":"e","gameId":"g","kind":"perf_window","buckets":[1,2,3,4,5,6,7]}`},
		{"oversized identifier", `{"schemaVersion":2,"sessionId":"s","eventId":"e","gameId":"` + strings.Repeat("g", 257) + `","kind":"session_start"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/v1/telemetry/events", strings.NewReader(tc.body))
			rec := httptest.NewRecorder()
			srv.Routes().ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("got %d, want 400: %s", rec.Code, rec.Body.String())
			}
		})
	}
}
