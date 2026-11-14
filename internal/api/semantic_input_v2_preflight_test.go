package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"doodlequest/internal/model"
	"doodlequest/internal/replaytrace"
)

func TestSemanticInputV2HTTPPreflightBindsPublishedIRWithoutScoreWrites(t *testing.T) {
	t.Setenv("DQ_ALLOW_INSECURE_USER_ID", "true") // Fixture identity only.
	srv := newRunsServer(t)
	const gameID = "sample-rainbow-bear"
	game, ok := srv.store.GetGame(gameID)
	if !ok || game.GameIR == nil {
		t.Fatal("missing published sample Game IR")
	}
	full := irSHA256Of(game.GameIR)
	if len(full) != 64 {
		t.Fatalf("published IR has no full digest: %q", full)
	}
	path := "/v1/games/" + gameID + "/semantic-input-v2/validate?irSha256=" + url.QueryEscape(full) + "&seed=42"
	idle := []byte(`{"schema":"semantic-input-trace-v2","durationTicks":120,"events":[]}`)
	call := func(method, route string, body []byte, viewer string, contentType string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, route, bytes.NewReader(body))
		req.Header.Set("Content-Type", contentType)
		if viewer != "" {
			req.Header.Set("X-User-ID", viewer)
		}
		w := httptest.NewRecorder()
		srv.Routes().ServeHTTP(w, req)
		return w
	}

	before := len(srv.runs)
	public := call(http.MethodPost, path, idle, "", "application/json")
	if public.Code != http.StatusOK {
		t.Fatalf("published idle semantic trace rejected: %d %s", public.Code, public.Body.String())
	}
	var data map[string]any
	if err := json.Unmarshal(public.Body.Bytes(), &data); err != nil {
		t.Fatal(err)
	}
	if data["schema"] != "semantic-input-admission-v2" ||
		data["irSha256"] != full || data["seed"] != float64(42) ||
		data["eventCount"] != float64(0) || data["durationTicks"] != float64(120) ||
		data["evidenceStatus"] != "unverified" ||
		data["rankingScope"] != "none" || data["attested"] != false ||
		len(data["traceSha256"].(string)) != 64 {
		t.Fatalf("diagnostic-only semantic identity response changed: %+v", data)
	}
	if got := public.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("identity-bearing response must not be shared cached: %q", got)
	}
	if got := public.Header().Get("Vary"); !strings.Contains(got, "Authorization") ||
		!strings.Contains(got, "X-User-ID") {
		t.Fatalf("identity-based response must vary on auth: %q", got)
	}
	publicAgain := call(http.MethodPost, path, idle, "", "application/json; charset=utf-8")
	if publicAgain.Code != http.StatusOK {
		t.Fatalf("valid charset JSON rejected: %d", publicAgain.Code)
	}
	var repeat map[string]any
	if err := json.Unmarshal(publicAgain.Body.Bytes(), &repeat); err != nil {
		t.Fatal(err)
	}
	if data["traceSha256"] != repeat["traceSha256"] {
		t.Fatalf("identical canonical evidence did not hash identically: %+v %+v", data, repeat)
	}

	// Use a *real declared* game input instead of a fabricated action.
	if len(game.GameIR.InputBindings) == 0 {
		t.Fatal("sample published game must have at least one control binding")
	}
	binding := game.GameIR.InputBindings[0]
	var device string
	var position *replaytrace.Position
	switch binding.Input {
	case "tap", "drag":
		device = "pointer"
		position = &replaytrace.Position{X: 1000, Y: 2000}
	case "keyboard":
		device = "keyboard"
	default:
		t.Fatalf("published sample has unsupported input binding: %+v", binding)
	}
	trace := replaytrace.Trace{
		Schema: replaytrace.SchemaV2, DurationTicks: 10,
		Events: []replaytrace.Event{
			{Tick: 1, Ordinal: 0, Action: binding.Action, Device: device, Phase: "down", Position: position},
			{Tick: 2, Ordinal: 0, Action: binding.Action, Device: device, Phase: "up"},
		},
	}
	bytesTrace, err := json.Marshal(trace)
	if err != nil {
		t.Fatal(err)
	}
	action := call(http.MethodPost, path, bytesTrace, "", "application/json")
	if action.Code != http.StatusOK {
		t.Fatalf("actual declared semantic control rejected: %d %s", action.Code, action.Body.String())
	}
	if len(srv.runs) != before {
		t.Fatal("diagnostic semantic preflight must never submit a run or a score")
	}

	invalidCases := []struct {
		name  string
		route string
		body  []byte
		mime  string
		want  int
	}{
		{"stale-full-IR", strings.Replace(path, full, strings.Repeat("f", 64), 1), idle, "application/json", http.StatusConflict},
		{"missing-hash", "/v1/games/" + gameID + "/semantic-input-v2/validate?seed=42", idle, "application/json", http.StatusBadRequest},
		{"wrong-case-hash-key", strings.Replace(path, "irSha256=", "irSHA256=", 1), idle, "application/json", http.StatusBadRequest},
		{"repeated-seed", path + "&seed=43", idle, "application/json", http.StatusBadRequest},
		{"extra-key", path + "&verified=true", idle, "application/json", http.StatusBadRequest},
		{"noncanonical-seed", strings.Replace(path, "seed=42", "seed=042", 1), idle, "application/json", http.StatusBadRequest},
		{"negative-seed", strings.Replace(path, "seed=42", "seed=-1", 1), idle, "application/json", http.StatusBadRequest},
		{"seed-overflow", strings.Replace(path, "seed=42", "seed=4294967296", 1), idle, "application/json", http.StatusBadRequest},
		{"empty-body", path, nil, "application/json", http.StatusBadRequest},
		{"unknown-schema", path, []byte(`{"schema":"semantic-input-trace-v1","durationTicks":1,"events":[]}`), "application/json", http.StatusBadRequest},
		{"unknown-member", path, []byte(`{"schema":"semantic-input-trace-v2","durationTicks":1,"events":[],"verified":true}`), "application/json", http.StatusBadRequest},
		{"duplicate-key", path, []byte(`{"schema":"semantic-input-trace-v2","durationTicks":1,"events":[],"events":[]}`), "application/json", http.StatusBadRequest},
		{"case-aliased-events", path, []byte(`{"schema":"semantic-input-trace-v2","durationTicks":1,"events":[],"Events":[]}`), "application/json", http.StatusBadRequest},
		{"null-events", path, []byte(`{"schema":"semantic-input-trace-v2","durationTicks":1,"events":null}`), "application/json", http.StatusBadRequest},
		{"too-large", path, bytes.Repeat([]byte(" "), replaytrace.MaxEncodedTraceBytes+1), "application/json", http.StatusRequestEntityTooLarge},
		{"wrong-mime", path, idle, "text/html", http.StatusUnsupportedMediaType},
		{"wrong-game", "/v1/games/not-a-published-game/semantic-input-v2/validate?irSha256=" + full + "&seed=42", idle, "application/json", http.StatusNotFound},
	}
	for _, tc := range invalidCases {
		t.Run(tc.name, func(t *testing.T) {
			w := call(http.MethodPost, tc.route, tc.body, "", tc.mime)
			if w.Code != tc.want {
				t.Fatalf("bad semantic preflight got %d want %d: %s", w.Code, tc.want, w.Body.String())
			}
			if got := w.Header().Get("Cache-Control"); got != "private, no-store" {
				t.Fatalf("negative identity-bearing response cache policy: %q", got)
			}
		})
	}
	if len(srv.runs) != before {
		t.Fatal("invalid input must never create run state")
	}

	game.OwnerID = "real-creator"
	game.Visibility = model.VisibilityPrivate
	game.ModerationStatus = "approved"
	srv.store.UpdateGame(game)
	for _, viewer := range []string{"", "unknown-viewer"} {
		denied := call(http.MethodPost, path, idle, viewer, "application/json")
		if denied.Code != http.StatusNotFound {
			t.Fatalf("private semantic evidence exposed to viewer %q: %d %s",
				viewer, denied.Code, denied.Body.String())
		}
	}
	owner := call(http.MethodPost, path, idle, "real-creator", "application/json")
	if owner.Code != http.StatusOK {
		t.Fatalf("creator private diagnostic validation denied: %d %s", owner.Code, owner.Body.String())
	}
	if len(srv.runs) != before {
		t.Fatal("owner's diagnostic preflight must not submit a run")
	}
}

func TestSemanticInputV2HTTPPreflightCanonicalUint32SeedBounds(t *testing.T) {
	for _, input := range []string{"0", "1", strconv.FormatUint(1<<32-1, 10)} {
		value, err := strconv.ParseUint(input, 10, 32)
		if err != nil || strconv.FormatUint(value, 10) != input {
			t.Fatalf("canonical uint32 seed invalid: %q", input)
		}
	}
}
