package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"doodlequest/internal/model"
)

func TestRecordedRunTraceDeliveryRequiresGameVisibility(t *testing.T) {
	t.Setenv("DQ_ALLOW_INSECURE_USER_ID", "true")
	srv := newRunsServer(t)
	published, ok := srv.store.GetGame("sample-rainbow-bear")
	if !ok {
		t.Fatal("inventory game unavailable")
	}
	published.Visibility = model.VisibilityPublic
	published.Status = model.GameReady
	published.ModerationStatus = "approved"
	srv.store.UpdateGame(published)

	private := *published
	private.ID = "private-replay-game"
	private.OwnerID = "creator-private"
	private.Visibility = model.VisibilityPrivate
	srv.store.CreateGame(&private)

	srv.runsMu.Lock()
	srv.runs = append(srv.runs,
		RunRecord{ID: "trace-private", GameID: private.ID, Seed: 17,
			Inputs: []RunInput{{Tick: 15, Type: "pointer", Detail: "secret-trace"}}},
		RunRecord{ID: "trace-public", GameID: published.ID, Seed: 19},
		RunRecord{ID: "trace-orphan", GameID: "deleted-game", Seed: 20},
	)
	srv.runsMu.Unlock()

	cases := []struct {
		name   string
		runID  string
		viewer string
		want   int
	}{
		{"private anonymous", "trace-private", "", http.StatusNotFound},
		{"private other player", "trace-private", "another-player", http.StatusNotFound},
		{"private creator", "trace-private", "creator-private", http.StatusOK},
		{"public anonymous", "trace-public", "", http.StatusOK},
		{"deleted game recording", "trace-orphan", "creator-private", http.StatusNotFound},
		{"nonexistent recording", "missing-run", "", http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/v1/runs/"+tc.runID, nil)
			if tc.viewer != "" {
				req.Header.Set("X-User-ID", tc.viewer)
			}
			rec := httptest.NewRecorder()
			srv.Routes().ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("GET run as %q = %d, want %d: %s",
					tc.viewer, rec.Code, tc.want, rec.Body.String())
			}
			if !strings.Contains(rec.Header().Get("Cache-Control"), "no-store") {
				t.Fatalf("recorded trace must never be cacheable: %v", rec.Header())
			}
			if tc.want != http.StatusOK && strings.Contains(rec.Body.String(), "secret-trace") {
				t.Fatal("private input trace leaked through denied response")
			}
			if tc.runID == "trace-private" && tc.want == http.StatusOK &&
				!strings.Contains(rec.Body.String(), "secret-trace") {
				t.Fatal("authorized creator cannot recover their recorded trace")
			}
		})
	}

	published.Quarantined = true
	srv.store.UpdateGame(published)
	req := httptest.NewRequest(http.MethodGet, "/v1/runs/trace-public", nil)
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("quarantined game trace must no longer be public: %d", rec.Code)
	}
}
