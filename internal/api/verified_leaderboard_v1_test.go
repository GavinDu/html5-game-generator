package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"doodlequest/internal/model"
)

func TestVerifiedOnlyLeaderboardNeverPromotesClientAndLegacyRanks(t *testing.T) {
	t.Setenv("DQ_ALLOW_INSECURE_USER_ID", "true")
	srv := newRunsServer(t)
	const gameID = "sample-rainbow-bear"
	game, ok := srv.store.GetGame(gameID)
	if !ok || game.GameIR == nil {
		t.Fatal("missing published fixture Game IR")
	}
	full := irSHA256Of(game.GameIR)
	beforeRuns := len(srv.runs)
	request := func(path, viewer string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if viewer != "" {
			req.Header.Set("X-User-ID", viewer)
		}
		rec := httptest.NewRecorder()
		srv.Routes().ServeHTTP(rec, req)
		return rec
	}
	base := "/v1/games/" + gameID + "/leaderboard/verified"
	checkEmpty := func(w *httptest.ResponseRecorder, expectedWindow string) {
		t.Helper()
		if w.Code != http.StatusOK {
			t.Fatalf("verified-only board refused: %d %s", w.Code, w.Body.String())
		}
		var data map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &data); err != nil {
			t.Fatal(err)
		}
		entries, valid := data["entries"].([]any)
		if !valid || len(entries) != 0 || data["gameId"] != gameID ||
			data["irSha256"] != full || data["irHash"] != full[:12] ||
			data["window"] != expectedWindow ||
			data["rankingScope"] != "verified_only" ||
			data["attestationAvailable"] != false ||
			data["metric"] != "time" || data["order"] != "asc" {
			t.Fatalf("trusted board may not contain client-origin ranks: %+v", data)
		}
		if _, found := data["rank"]; found {
			t.Fatalf("trusted empty board cannot advertise any rank: %+v", data)
		}
		if w.Header().Get("Cache-Control") != "private, no-store" ||
			!strings.Contains(w.Header().Get("Vary"), "Authorization") ||
			!strings.Contains(w.Header().Get("Vary"), "X-User-ID") {
			t.Fatalf("private, identity scoped board headers missing: %v", w.Header())
		}
	}
	checkEmpty(request(base, ""), "week")
	code, response := postRun(t, srv, gameID,
		`{"name":"ClientClaim","score":12,"won":true,"durationMs":25000,"seed":42}`)
	if code != http.StatusCreated || response["rank"] != float64(1) {
		t.Fatalf("legacy provisional submission contract unexpectedly changed: %d %+v", code, response)
	}
	provisional := getBoard(t, srv, gameID, "week")
	if entries, ok := provisional["entries"].([]any); !ok || len(entries) != 1 {
		t.Fatalf("legacy provisional board should remain separate: %+v", provisional)
	}
	// Simulate persisted pre-attestor bad legacy client-writable Verified.
	v := true
	srv.runsMu.Lock()
	srv.runs[0].Verified = &v
	srv.runsMu.Unlock()
	checkEmpty(request(base+"?window=week", ""), "week")
	checkEmpty(request(base+"?window=all", ""), "all")
	checkEmpty(request(base+"?window=all", ""), "all")
	if len(srv.runs) != beforeRuns+1 {
		t.Fatal("read-only verified board unexpectedly modified stored runs")
	}
	invalid := []string{
		base + "?window=", base + "?window=year", base + "?window=Week",
		base + "?window=week&window=all", base + "?scope=provisional",
		base + "?window=all&verified=true", base + "?window=%",
	}
	for _, uri := range invalid {
		bad := request(uri, "")
		if bad.Code != http.StatusBadRequest {
			t.Fatalf("malformed verified-board query accepted: %q %d %s", uri, bad.Code, bad.Body.String())
		}
		if bad.Header().Get("Cache-Control") != "private, no-store" {
			t.Fatalf("untrusted error response can be cached: %q", uri)
		}
	}
	denied := request("/v1/games/not-a-published-game/leaderboard/verified", "")
	if denied.Code != http.StatusNotFound {
		t.Fatalf("unknown trusted game leaked: %d", denied.Code)
	}
	game.Visibility = model.VisibilityPrivate
	game.OwnerID = "real-publisher"
	game.ModerationStatus = "approved"
	srv.store.UpdateGame(game)
	for _, viewer := range []string{"", "guest"} {
		w := request(base, viewer)
		if w.Code != http.StatusNotFound {
			t.Fatalf("nonowner sees private board: %q status=%d", viewer, w.Code)
		}
	}
	checkEmpty(request(base, "real-publisher"), "week")
	if len(srv.runs) != beforeRuns+1 {
		t.Fatal("verified-only query mutated client claims")
	}
}
