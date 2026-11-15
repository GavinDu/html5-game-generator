package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"doodlequest/internal/model"
)

func TestVerifiedResultReadFailsClosedForLegacyClientVerdictsAndPrivateRuns(t *testing.T) {
	t.Setenv("DQ_ALLOW_INSECURE_USER_ID", "true")
	srv := newRunsServer(t)
	public, ok := srv.store.GetGame("sample-rainbow-bear")
	if !ok {
		t.Fatal("missing public inventory fixture")
	}
	public.Visibility = model.VisibilityPublic
	public.Status = model.GameReady
	public.ModerationStatus = "approved"
	srv.store.UpdateGame(public)
	private := *public
	private.ID = "private-verdict-game"
	private.OwnerID = "private-verdict-owner"
	private.Visibility = model.VisibilityPrivate
	srv.store.CreateGame(&private)

	// Historical RunRecord.Verified and ActualScore were client-writable.
	// Even an internally stored old verified=true may never be elevated.
	legacyVerified := true
	clientScore := 999999
	srv.runsMu.Lock()
	srv.runs = append(srv.runs,
		RunRecord{
			ID: "legacy-public-run", GameID: public.ID, Name: "legacy-secret-player",
			Score: clientScore, Won: true, DurationMs: 1, Seed: 1,
			Verified: &legacyVerified, ActualScore: &clientScore,
		},
		RunRecord{
			ID: "legacy-private-run", GameID: private.ID,
			Verified: &legacyVerified, ActualScore: &clientScore,
		},
		RunRecord{ID: "orphan-run", GameID: "deleted-game"},
	)
	count := len(srv.runs)
	srv.runsMu.Unlock()

	request := func(path string, user string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if user != "" {
			req.Header.Set("X-User-ID", user)
		}
		rec := httptest.NewRecorder()
		srv.Routes().ServeHTTP(rec, req)
		return rec
	}
	cases := []struct {
		name string
		path string
		user string
		want int
	}{
		{"legacy public never verified", "/v1/runs/legacy-public-run/result", "", http.StatusServiceUnavailable},
		{"legacy private hidden anonymously", "/v1/runs/legacy-private-run/result", "", http.StatusNotFound},
		{"legacy private hidden from others", "/v1/runs/legacy-private-run/result", "someone-else", http.StatusNotFound},
		{"legacy private creator cannot promote a claim", "/v1/runs/legacy-private-run/result", "private-verdict-owner", http.StatusServiceUnavailable},
		{"orphan not found", "/v1/runs/orphan-run/result", "", http.StatusNotFound},
		{"unknown not found", "/v1/runs/missing-run/result", "", http.StatusNotFound},
		{"no arbitrary query", "/v1/runs/legacy-public-run/result?status=verified", "", http.StatusNotFound},
		{"too long", "/v1/runs/" + strings.Repeat("a", 129) + "/result", "", http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := request(tc.path, tc.user)
			if rec.Code != tc.want {
				t.Fatalf("result read = %d, want %d: %s", rec.Code, tc.want, rec.Body.String())
			}
			if rec.Header().Get("Cache-Control") != "private, no-store" ||
				!strings.Contains(rec.Header().Get("Vary"), "Authorization") ||
				!strings.Contains(rec.Header().Get("Vary"), "X-User-ID") {
				t.Fatalf("result read privacy headers missing: %v", rec.Header())
			}
			for _, privateData := range []string{
				"legacy-secret-player", "legacy-public-run", "legacy-private-run",
				"999999", "actualScore", "attested", "rankingScope",
				"\"result\"", "\"score\"", "\"verified\":true",
			} {
				if strings.Contains(rec.Body.String(), privateData) {
					t.Fatalf("unverified run leaked %q into native result response", privateData)
				}
			}
			if rec.Code == http.StatusServiceUnavailable &&
				rec.Header().Get("Retry-After") == "" {
				t.Fatal("service-unavailable result missing bounded retry guidance")
			}
		})
	}

	// Revoking a public game's publication is effective immediately.
	public.Quarantined = true
	srv.store.UpdateGame(public)
	if rec := request("/v1/runs/legacy-public-run/result", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("revoked game verdict should disappear: %d", rec.Code)
	}
	srv.runsMu.Lock()
	defer srv.runsMu.Unlock()
	if len(srv.runs) != count {
		t.Fatal("read route changed the run ledger")
	}
	if srv.runs[count-3].Verified == nil || !*srv.runs[count-3].Verified ||
		srv.runs[count-3].ActualScore == nil ||
		*srv.runs[count-3].ActualScore != clientScore {
		t.Fatal("read route mutated historical verdict evidence")
	}
}

func TestVerifiedResultRunIDAdmissionBounds(t *testing.T) {
	for _, valid := range []string{"a", "abc-123_ef.ghi~", strings.Repeat("A", 128)} {
		if !validVerifiedRunLookupID(valid) {
			t.Fatalf("valid bounded opaque run ID rejected: %q", valid)
		}
	}
	for _, invalid := range []string{
		"", "a/b", "a%2Fb", "a?b", "a b", "a#b", "a\nb",
		"☃", strings.Repeat("a", 129),
	} {
		if validVerifiedRunLookupID(invalid) {
			t.Fatalf("unsafe opaque run ID admitted: %q", invalid)
		}
	}
}
