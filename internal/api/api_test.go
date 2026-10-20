package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"doodlequest/internal/gameir"
	"doodlequest/internal/inventory"
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
	publishedGameRec := do(t, srv, "GET", "/v1/games/"+created.Game.ID, nil, "")
	var publishedGame model.Game
	decode(t, publishedGameRec, &publishedGame)
	if publishedGame.GameIR == nil {
		t.Fatal("published game has no canonical Game IR")
	}
	irRec := do(t, srv, "GET", "/v1/games/"+created.Game.ID+"/game-ir", nil, "")
	if irRec.Code != http.StatusOK {
		t.Fatalf("Game IR endpoint: %d %s", irRec.Code, irRec.Body.String())
	}
	var fetchedIR gameir.Document
	decode(t, irRec, &fetchedIR)
	if err := gameir.Validate(&fetchedIR); err != nil {
		t.Fatalf("published Game IR invalid: %v", err)
	}
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

	bundleBase := "/v1/games/" + created.Game.ID + "/bundle"
	for _, p := range []string{
		bundleBase + "/game.js",
		bundleBase + "/" + manifest.Assets.Music.Src,
	} {
		rec = do(t, srv, "GET", p, nil, "")
		if rec.Code != 200 || rec.Body.Len() == 0 {
			t.Fatalf("artifact %s: %d, len %d", p, rec.Code, rec.Body.Len())
		}
	}
	// Every sprite URL exactly as the manifest declares it must be fetchable.
	for _, sp := range manifest.Assets.Sprites {
		p := bundleBase + "/" + sp.Src
		rec = do(t, srv, "GET", p, nil, "")
		if rec.Code != 200 || rec.Body.Len() == 0 {
			t.Fatalf("sprite %s (%s): %d, len %d", sp.ID, p, rec.Code, rec.Body.Len())
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

// The package endpoint exposes the canonical Game IR envelope and asset URLs.
func TestGetPackage(t *testing.T) {
	st := store.NewMemoryStore()
	srv := New(st, pipeline.NewRunner(st))
	data, err := os.ReadFile("../../examples/game-ir-v1/bobos-rainbow.json")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := gameir.DecodeAndValidate(data)
	if err != nil {
		t.Fatal(err)
	}
	st.CreateGame(&model.Game{ID: doc.Game.ID, Title: doc.Game.Title, GameIR: doc})
	st.PutArtifact(doc.Game.ID, "assets/bear.png", []byte("fake-png"), "image/png")
	st.CreateGame(&model.Game{ID: "pkg-draft-1", Title: "Draft"})

	rec := do(t, srv, "GET", "/v1/games/"+doc.Game.ID+"/package", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("package: %d %s", rec.Code, rec.Body.String())
	}
	var pkg GamePackage
	decode(t, rec, &pkg)
	if pkg.PackageVersion != "2.0" || pkg.GameID != doc.Game.ID || pkg.IR == nil {
		t.Fatalf("bad package envelope: %+v", pkg)
	}
	if pkg.IR.IRVersion != gameir.Version || len(pkg.Assets) != 5 {
		t.Fatalf("bad canonical IR/assets: version=%s assets=%d", pkg.IR.IRVersion, len(pkg.Assets))
	}
	if pkg.Assets[1].ID != "bobo" || pkg.Assets[1].URI != "assets/bear.png" ||
		pkg.Assets[1].URL != "/v1/games/"+doc.Game.ID+"/assets/bobo" {
		t.Fatalf("bad asset descriptor: %+v", pkg.Assets[1])
	}
	assetRec := do(t, srv, "GET", pkg.Assets[1].URL, nil, "")
	if assetRec.Code != http.StatusOK || assetRec.Body.String() != "fake-png" ||
		assetRec.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("asset delivery: %d %s", assetRec.Code, assetRec.Body.String())
	}
	if rec := do(t, srv, "GET", "/v1/games/nope/package", nil, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rec.Code)
	}
	if rec := do(t, srv, "GET", "/v1/games/pkg-draft-1/package", nil, ""); rec.Code != http.StatusConflict {
		t.Fatalf("want 409, got %d", rec.Code)
	}
}

func TestGameIRValidateEndpoint(t *testing.T) {
	srv := newTestServer()
	data, err := os.ReadFile("../../examples/game-ir-v1/bobos-rainbow.json")
	if err != nil {
		t.Fatal(err)
	}
	rec := do(t, srv, "POST", "/v1/game-ir/validate", json.RawMessage(data), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("valid Game IR: status %d, body %s", rec.Code, rec.Body.String())
	}
	var result struct {
		Valid bool `json:"valid"`
		IRVersion string `json:"irVersion"`
		GameID string `json:"gameId"`
		SceneCount int `json:"sceneCount"`
		AssetCount int `json:"assetCount"`
		RuleCount int `json:"ruleCount"`
	}
	decode(t, rec, &result)
	if !result.Valid || result.IRVersion != "1.0" || result.GameID != "bobos-rainbow" ||
		result.SceneCount != 1 || result.AssetCount != 5 || result.RuleCount != 2 {
		t.Fatalf("unexpected validation result: %+v", result)
	}
	invalid := strings.Replace(string(data), `"type": "add_score"`, `"type": "unsupported_action"`, 1)
	rec = do(t, srv, "POST", "/v1/game-ir/validate", json.RawMessage(invalid), "")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid Game IR: want 422, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestBoboRainbowRemixPublishesGameIRPackage(t *testing.T) {
	st := store.NewMemoryStore()
	if err := inventory.Load(st, "../../inventory"); err != nil {
		t.Fatalf("load legacy game inventory: %v", err)
	}
	parent, ok := st.GetGame("sample-rainbow-bear")
	if !ok || parent.Status != model.GameReady || parent.Manifest == nil {
		t.Fatal("Bobo's Rainbow parent game was not loaded from inventory")
	}
	srv := New(st, pipeline.NewRunner(st))

	rec := do(t, srv, "POST", "/v1/games", map[string]any{
		"parent_game_id": "sample-rainbow-bear",
		"intent": "Remix Bobo's Rainbow into a short heart-collecting rescue game",
	}, "remix-tester")
	if rec.Code != http.StatusAccepted {
		t.Fatalf("create remix: %d %s", rec.Code, rec.Body.String())
	}
	var created struct {
		Game model.Game `json:"game"`
		Job model.Job `json:"job"`
	}
	decode(t, rec, &created)
	if created.Game.ParentGameID != parent.ID {
		t.Fatalf("lineage not recorded on child: %+v", created.Game)
	}

	job := waitJob(t, srv, created.Job.ID, model.JobAwaitingConfirm)
	if job.Status != model.JobAwaitingConfirm || job.DesignDoc == nil {
		t.Fatalf("remix did not reach design confirmation: %+v", job)
	}
	if job.DesignDoc.Title != "Bobo's Rainbow: Heart Rescue" {
		t.Fatalf("unexpected remix design title: %q", job.DesignDoc.Title)
	}
	rec = do(t, srv, "POST", "/v1/games/"+created.Game.ID+"/confirm", map[string]any{}, "remix-tester")
	if rec.Code != http.StatusAccepted {
		t.Fatalf("confirm remix: %d %s", rec.Code, rec.Body.String())
	}
	job = waitJob(t, srv, created.Job.ID, model.JobDone)
	if job.Status != model.JobDone {
		t.Fatalf("remix generation failed: %s", job.Error)
	}

	child, ok := st.GetGame(created.Game.ID)
	if !ok || child.GameIR == nil {
		t.Fatal("published remix has no canonical Game IR")
	}
	if child.GameIR.Metadata.ParentGameID != parent.ID || child.GameIR.Metadata.ParentVersion != parent.Manifest.Version {
		t.Fatalf("Game IR lineage mismatch: %+v", child.GameIR.Metadata)
	}
	if err := gameir.Validate(child.GameIR); err != nil {
		t.Fatalf("remix Game IR failed semantic validation: %v", err)
	}
	if len(child.GameIR.Assets) < 9 || len(child.GameIR.Scenes) != 1 || len(child.GameIR.Rules) != 4 {
		t.Fatalf("unexpected remix shape: assets=%d scenes=%d rules=%d", len(child.GameIR.Assets), len(child.GameIR.Scenes), len(child.GameIR.Rules))
	}

	// The native iOS player consumes this exact package envelope and fetches
	// each asset through the package's declared delivery URL.
	rec = do(t, srv, "GET", "/v1/games/"+child.ID+"/package", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("package endpoint: %d %s", rec.Code, rec.Body.String())
	}
	var pkg GamePackage
	decode(t, rec, &pkg)
	if pkg.PackageVersion != PackageVersion || pkg.GameID != child.ID || pkg.IR.Metadata.ParentGameID != parent.ID {
		t.Fatalf("package contract mismatch: %+v", pkg)
	}
	for _, asset := range pkg.Assets {
		rec = do(t, srv, "GET", asset.URL, nil, "")
		if rec.Code != http.StatusOK || rec.Body.Len() == 0 {
			t.Fatalf("package asset %q unavailable at %s: %d", asset.ID, asset.URL, rec.Code)
		}
	}
	// Creating a remix must not mutate the original inventory record: its
	// manifest version is untouched and its own canonical IR (seeded from
	// inventory) does not gain the remix's lineage.
	unchanged, ok := st.GetGame(parent.ID)
	if !ok || unchanged.Manifest.Version != parent.Manifest.Version {
		t.Fatal("remix modified the original inventory game")
	}
	if unchanged.GameIR == nil || unchanged.GameIR.Metadata.ParentGameID != "" {
		t.Fatal("remix modified the original inventory game's Game IR")
	}
}

func TestDecodeJSONRejectsTrailingJSONAndOversizedBodies(t *testing.T) {
	srv := newTestServer()
	trailing := httptest.NewRequest(http.MethodPost, "/v1/ingest", strings.NewReader(`{"keywords":["cat"]} {"unexpected":true}`))
	trailingRec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(trailingRec, trailing)
	if trailingRec.Code != http.StatusBadRequest {
		t.Fatalf("trailing JSON: want 400, got %d body=%s", trailingRec.Code, trailingRec.Body.String())
	}
	oversizedBody := strings.NewReader(`{"keywords":["` + strings.Repeat("x", (2<<20)+16) + `"]}`)
	oversized := httptest.NewRequest(http.MethodPost, "/v1/ingest", oversizedBody)
	oversizedRec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(oversizedRec, oversized)
	if oversizedRec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized JSON: want 413, got %d body=%s", oversizedRec.Code, oversizedRec.Body.String())
	}
}
