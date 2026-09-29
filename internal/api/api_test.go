package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"doodlequest/internal/model"
	"doodlequest/internal/pipeline"
	"doodlequest/internal/store"
)

func newTestServer() *Server {
	st := store.NewMemoryStore()
	return New(st, pipeline.NewRunner(st))
}

func do(t *testing.T, srv *Server, method, path string, body any, user string) *httptest.ResponseRecorder {
	t.Helper()
	var rdr *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		rdr = bytes.NewReader(b)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, rdr)
	if user != "" {
		req.Header.Set("X-User-ID", user)
	}
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	return rec
}

func decode(t *testing.T, rec *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.NewDecoder(rec.Body).Decode(v); err != nil {
		t.Fatalf("decode %s: %v", rec.Body.String(), err)
	}
}

func waitJob(t *testing.T, srv *Server, jobID string, want model.JobStatus) *model.Job {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		rec := do(t, srv, "GET", "/v1/jobs/"+jobID, nil, "")
		var job model.Job
		decode(t, rec, &job)
		if job.Status == want || job.Status == model.JobFailed {
			return &job
		}
		time.Sleep(150 * time.Millisecond)
	}
	t.Fatalf("job %s never reached %s", jobID, want)
	return nil
}

// Full pipeline: ingest -> create -> spec -> confirm -> art/codegen/validate/publish.
func TestFullPipeline(t *testing.T) {
	srv := newTestServer()

	rec := do(t, srv, "POST", "/v1/ingest", map[string]any{
		"keywords": []string{"dragon", "castle"},
	}, "parent-1")
	if rec.Code != 200 {
		t.Fatalf("ingest: %d %s", rec.Code, rec.Body.String())
	}
	var ing model.IngestResult
	decode(t, rec, &ing)
	if len(ing.Elements) != 2 || ing.Elements[0].Label != "dragon" {
		t.Fatalf("unexpected elements: %+v", ing.Elements)
	}

	rec = do(t, srv, "POST", "/v1/games", map[string]any{"ingest_id": ing.IngestID}, "parent-1")
	if rec.Code != 202 {
		t.Fatalf("create game: %d %s", rec.Code, rec.Body.String())
	}
	var created struct {
		Game model.Game `json:"game"`
		Job  model.Job  `json:"job"`
	}
	decode(t, rec, &created)

	job := waitJob(t, srv, created.Job.ID, model.JobAwaitingConfirm)
	if job.DesignDoc == nil || job.DesignDoc.Title == "" {
		t.Fatalf("no design doc at confirm gate: %+v", job)
	}
	if len(job.Stages) != 5 {
		t.Fatalf("want 5 stages, got %d", len(job.Stages))
	}

	// Confirming twice: first a bogus early confirm is tested below; here the real one.
	rec = do(t, srv, "POST", "/v1/games/"+created.Game.ID+"/confirm", map[string]any{}, "parent-1")
	if rec.Code != 202 {
		t.Fatalf("confirm: %d %s", rec.Code, rec.Body.String())
	}

	job = waitJob(t, srv, created.Job.ID, model.JobDone)
	for _, st := range job.Stages {
		if st.Status != model.StageDone {
			t.Fatalf("stage %s not done: %+v", st.Name, st)
		}
	}

	rec = do(t, srv, "GET", "/v1/games/"+created.Game.ID+"/bundle", nil, "")
	if rec.Code != 200 {
		t.Fatalf("bundle: %d %s", rec.Code, rec.Body.String())
	}
	var manifest model.BundleManifest
	decode(t, rec, &manifest)
	if manifest.Shim != "gameshim/v1" || len(manifest.Assets.Sprites) != 3 {
		t.Fatalf("bad manifest: %+v", manifest)
	}

	for _, p := range []string{
		"/v1/games/" + created.Game.ID + "/bundle/game.js",
		"/v1/games/" + created.Game.ID + "/bundle/sprites/el_1",
		"/v1/games/" + created.Game.ID + "/bundle/audio/theme.wav",
	} {
		rec = do(t, srv, "GET", p, nil, "")
		if rec.Code != 200 || rec.Body.Len() == 0 {
			t.Fatalf("artifact %s: %d, len %d", p, rec.Code, rec.Body.Len())
		}
	}
}

func TestConfirmTooEarly(t *testing.T) {
	srv := newTestServer()
	rec := do(t, srv, "POST", "/v1/games", map[string]any{
		"elements": []model.Element{{ID: "el_1", Label: "robot", Kind: "character"}},
		"intent":   "robot game",
	}, "parent-1")
	var created struct {
		Game model.Game `json:"game"`
		Job  model.Job  `json:"job"`
	}
	decode(t, rec, &created)
	// Confirm immediately, before SPEC finishes -> 409.
	rec = do(t, srv, "POST", "/v1/games/"+created.Game.ID+"/confirm", map[string]any{}, "parent-1")
	if rec.Code != http.StatusConflict {
		t.Fatalf("want 409, got %d %s", rec.Code, rec.Body.String())
	}
	waitJob(t, srv, created.Job.ID, model.JobAwaitingConfirm)
}

// Groups + author-controlled sharing.
func TestGroupsAndSharing(t *testing.T) {
	srv := newTestServer()

	// Author makes a game and runs it to ready.
	rec := do(t, srv, "POST", "/v1/games", map[string]any{
		"elements": []model.Element{{ID: "el_1", Label: "dragon", Kind: "character"}},
		"intent":   "dragon game",
	}, "author")
	var created struct {
		Game model.Game `json:"game"`
		Job  model.Job  `json:"job"`
	}
	decode(t, rec, &created)
	waitJob(t, srv, created.Job.ID, model.JobAwaitingConfirm)
	do(t, srv, "POST", "/v1/games/"+created.Game.ID+"/confirm", map[string]any{}, "author")
	waitJob(t, srv, created.Job.ID, model.JobDone)

	// Author creates a group and shares the game with it.
	rec = do(t, srv, "POST", "/v1/groups", map[string]any{"name": "Cubs"}, "author")
	var gr model.Group
	decode(t, rec, &gr)

	rec = do(t, srv, "POST", "/v1/games/"+created.Game.ID+"/visibility",
		map[string]any{"visibility": "groups", "group_ids": []string{gr.ID}}, "author")
	if rec.Code != 200 {
		t.Fatalf("visibility: %d %s", rec.Code, rec.Body.String())
	}

	// A stranger cannot see group games; joining with a bad code fails.
	rec = do(t, srv, "GET", "/v1/groups/"+gr.ID+"/games", nil, "stranger")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d", rec.Code)
	}
	rec = do(t, srv, "POST", "/v1/groups/"+gr.ID+"/join", map[string]any{"invite_code": "WRONG"}, "friend")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403 on bad code, got %d", rec.Code)
	}

	// Friend joins with the real code and sees the game.
	rec = do(t, srv, "POST", "/v1/groups/"+gr.ID+"/join", map[string]any{"invite_code": gr.InviteCode}, "friend")
	if rec.Code != 200 {
		t.Fatalf("join: %d %s", rec.Code, rec.Body.String())
	}
	rec = do(t, srv, "GET", "/v1/groups/"+gr.ID+"/games", nil, "friend")
	var list struct {
		Games []model.Game `json:"games"`
	}
	decode(t, rec, &list)
	if len(list.Games) != 1 {
		t.Fatalf("want 1 group game, got %d", len(list.Games))
	}

	// Non-author cannot change visibility.
	rec = do(t, srv, "POST", "/v1/games/"+created.Game.ID+"/visibility",
		map[string]any{"visibility": "public"}, "friend")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d", rec.Code)
	}

	// Author goes public -> appears in feed.
	rec = do(t, srv, "POST", "/v1/games/"+created.Game.ID+"/visibility",
		map[string]any{"visibility": "public"}, "author")
	if rec.Code != 200 {
		t.Fatalf("visibility public: %d %s", rec.Code, rec.Body.String())
	}
	rec = do(t, srv, "GET", "/v1/feed", nil, "")
	var feed struct {
		Games []model.Game `json:"games"`
	}
	decode(t, rec, &feed)
	if len(feed.Games) != 1 {
		t.Fatalf("want 1 feed game, got %d", len(feed.Games))
	}
}
