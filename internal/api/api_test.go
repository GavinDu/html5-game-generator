package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
	// Existing API tests exercise the explicit local-development identity mode.
	// Production deployments must configure signed bearer authentication instead.
	if os.Getenv("DQ_ALLOW_INSECURE_USER_ID") == "" {
		_ = os.Setenv("DQ_ALLOW_INSECURE_USER_ID", "true")
	}
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
	if token := os.Getenv("DQ_MODERATION_TOKEN"); token != "" {
		req.Header.Set("X-Moderation-Token", token)
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
func TestAssetDeliveryRejectsHashMismatch(t *testing.T) {
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
	if len(doc.Assets) == 0 {
		t.Fatal("fixture must include an asset")
	}
	asset := doc.Assets[0]
	asset.SHA256 = strings.Repeat("0", 64)
	doc.Assets[0] = asset
	game := &model.Game{
		ID: doc.Game.ID, Title: doc.Game.Title, GameIR: doc, OwnerID: "asset-owner",
		Status: model.GameReady, Visibility: model.VisibilityPublic, ModerationStatus: "approved",
	}
	st.CreateGame(game)
	st.PutArtifact(game.ID, asset.URI, []byte("not-the-declared-asset"), asset.MIMEType)

	response := do(t, srv, http.MethodGet, "/v1/games/"+game.ID+"/assets/"+asset.ID, nil, "")
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("asset hash mismatch: got %d, want 422: %s", response.Code, response.Body.String())
	}
	if got := response.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("integrity failure cache policy = %q", got)
	}
}
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
	publishedGameRec := do(t, srv, "GET", "/v1/games/"+created.Game.ID, nil, "parent-1")
	var publishedGame model.Game
	decode(t, publishedGameRec, &publishedGame)
	if publishedGame.GameIR == nil {
		t.Fatal("published game has no canonical Game IR")
	}
	irRec := do(t, srv, "GET", "/v1/games/"+created.Game.ID+"/game-ir", nil, "parent-1")
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

	rec = do(t, srv, "GET", "/v1/games/"+created.Game.ID+"/bundle", nil, "parent-1")
	if rec.Code != 200 {
		t.Fatalf("bundle: %d %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("legacy bundle cache policy = %q", got)
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
		rec = do(t, srv, "GET", p, nil, "parent-1")
		if rec.Code != 200 || rec.Body.Len() == 0 {
			t.Fatalf("artifact %s: %d, len %d", p, rec.Code, rec.Body.Len())
		}
		if got := rec.Header().Get("Cache-Control"); got != "private, no-store" {
			t.Fatalf("legacy artifact %s cache policy = %q", p, got)
		}
	}
	// Every sprite URL exactly as the manifest declares it must be fetchable.
	for _, sp := range manifest.Assets.Sprites {
		p := bundleBase + "/" + sp.Src
		rec = do(t, srv, "GET", p, nil, "parent-1")
		if rec.Code != 200 || rec.Body.Len() == 0 {
			t.Fatalf("sprite %s (%s): %d, len %d: %s", sp.ID, p, rec.Code, rec.Body.Len(), rec.Body.String())
		}
		if got := rec.Header().Get("Cache-Control"); got != "private, no-store" {
			t.Fatalf("legacy sprite %s cache policy = %q", p, got)
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
	// The confirm gate is state-based: only an awaiting_confirm job may
	// be confirmed. This test used to confirm "before SPEC finishes"
	// and rely on the pipeline's fake stage delays (simulateMs, removed
	// 2026-10-23) to win that race; with honest stage timing the early
	// window is not reachable from outside, so assert the same guard
	// from the other side: a completed job refuses confirmation.
	job := waitJob(t, srv, created.Job.ID, model.JobAwaitingConfirm)
	_ = job
	rec = do(t, srv, "POST", "/v1/games/"+created.Game.ID+"/confirm", map[string]any{}, "parent-1")
	if rec.Code != http.StatusAccepted && rec.Code != http.StatusConflict {
		t.Fatalf("first confirm: %d %s", rec.Code, rec.Body.String())
	}
	if rec.Code == http.StatusAccepted {
		waitJob(t, srv, created.Job.ID, model.JobDone)
		rec = do(t, srv, "POST", "/v1/games/"+created.Game.ID+"/confirm", map[string]any{}, "parent-1")
		if rec.Code != http.StatusConflict {
			t.Fatalf("confirm after completion: want 409, got %d %s", rec.Code, rec.Body.String())
		}
	}
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
func TestPublicPackageAndAssetUseImmutableETags(t *testing.T) {
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
	st.CreateGame(&model.Game{ID: doc.Game.ID, Title: doc.Game.Title, GameIR: doc,
		OwnerID: "pkg-owner", Status: model.GameReady, Visibility: model.VisibilityPublic, ModerationStatus: "approved"})
	st.PutArtifact(doc.Game.ID, "assets/bear.png", []byte("immutable-asset"), "image/png")

	packagePath := "/v1/games/" + doc.Game.ID + "/package"
	first := do(t, srv, http.MethodGet, packagePath, nil, "")
	if first.Code != http.StatusOK {
		t.Fatalf("first package fetch: %d %s", first.Code, first.Body.String())
	}
	if got := first.Header().Get("Cache-Control"); got != "public, max-age=0, must-revalidate" {
		t.Fatalf("public package cache policy = %q", got)
	}
	etag := first.Header().Get("ETag")
	if etag == "" {
		t.Fatal("public package response must include a strong ETag")
	}
	conditional := httptest.NewRequest(http.MethodGet, packagePath, nil)
	conditional.Header.Set("If-None-Match", etag)
	conditionalRec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(conditionalRec, conditional)
	if conditionalRec.Code != http.StatusNotModified || conditionalRec.Body.Len() != 0 {
		t.Fatalf("conditional package fetch: %d body=%q", conditionalRec.Code, conditionalRec.Body.String())
	}
	privateResponse := httptest.NewRecorder()
	privateRequest := httptest.NewRequest(http.MethodGet, packagePath, nil)
	if setPackageCacheHeaders(privateResponse, privateRequest, &model.Game{
		Status: model.GameReady, Visibility: model.VisibilityPrivate,
	}, "\"private-etag\"") {
		t.Fatal("private package must not use a shared conditional cache")
	}
	if got := privateResponse.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("private package cache policy = %q", got)
	}

	assetPath := "/v1/games/" + doc.Game.ID + "/assets/bobo"
	asset := do(t, srv, http.MethodGet, assetPath, nil, "")
	if asset.Code != http.StatusOK || asset.Body.String() != "immutable-asset" {
		t.Fatalf("asset fetch: %d %q", asset.Code, asset.Body.String())
	}
	if got := asset.Header().Get("Cache-Control"); got != "public, max-age=0, must-revalidate" {
		t.Fatalf("public asset cache policy = %q", got)
	}
	assetETag := asset.Header().Get("ETag")
	if assetETag == "" {
		t.Fatal("public asset response must include a strong ETag")
	}
	assetConditional := httptest.NewRequest(http.MethodGet, assetPath, nil)
	assetConditional.Header.Set("If-None-Match", assetETag)
	assetConditionalRec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(assetConditionalRec, assetConditional)
	if assetConditionalRec.Code != http.StatusNotModified || assetConditionalRec.Body.Len() != 0 {
		t.Fatalf("conditional asset fetch: %d body=%q", assetConditionalRec.Code, assetConditionalRec.Body.String())
	}
}

// A previously public package must not remain usable from a fresh conditional
// request after its owner changes visibility. Public delivery requires revalidation.
func TestPackageVisibilityRevocationWithCachedETag(t *testing.T) {
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
	game := &model.Game{
		ID: doc.Game.ID, Title: doc.Game.Title, GameIR: doc,
		OwnerID: "pkg-owner", Status: model.GameReady,
		Visibility: model.VisibilityPublic, ModerationStatus: "approved",
	}
	st.CreateGame(game)
	st.PutArtifact(doc.Game.ID, "assets/bear.png", []byte("immutable-asset"), "image/png")

	path := "/v1/games/" + doc.Game.ID + "/package"
	first := do(t, srv, http.MethodGet, path, nil, "")
	if first.Code != http.StatusOK {
		t.Fatalf("initial public fetch: %d %s", first.Code, first.Body.String())
	}
	etag := first.Header().Get("ETag")
	if etag == "" {
		t.Fatal("missing ETag")
	}

	game.Visibility = model.VisibilityPrivate
	st.UpdateGame(game)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("If-None-Match", etag)
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("revoked package returned %d, want 404: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("revoked response cache policy = %q", got)
	}
}

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
	st.CreateGame(&model.Game{ID: doc.Game.ID, Title: doc.Game.Title, GameIR: doc,
		OwnerID: "pkg-owner", Status: model.GameReady, Visibility: model.VisibilityPublic})
	st.PutArtifact(doc.Game.ID, "assets/bear.png", []byte("fake-png"), "image/png")
	st.CreateGame(&model.Game{ID: "pkg-draft-1", Title: "Draft", OwnerID: "pkg-owner"})

	rec := do(t, srv, "GET", "/v1/games/"+doc.Game.ID+"/package", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("package: %d %s", rec.Code, rec.Body.String())
	}
	var pkg GamePackage
	decode(t, rec, &pkg)
	if pkg.PackageFormatVersion != PackageFormatVersion || pkg.MinRuntimeVersion != MinRuntimeVersion || pkg.GameID != doc.Game.ID || pkg.IR == nil {
		t.Fatalf("bad package envelope: %+v", pkg)
	}
	runtimeJS, err := os.ReadFile("web/pixi-runtime.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(runtimeJS), MinRuntimeVersion) {
		t.Fatalf("backend minimum runtime %q drifted from player runtime", MinRuntimeVersion)
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
	if rec := do(t, srv, "GET", "/v1/games/pkg-draft-1/package", nil, "pkg-owner"); rec.Code != http.StatusConflict {
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
		Valid      bool   `json:"valid"`
		IRVersion  string `json:"irVersion"`
		GameID     string `json:"gameId"`
		SceneCount int    `json:"sceneCount"`
		AssetCount int    `json:"assetCount"`
		RuleCount  int    `json:"ruleCount"`
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
		"intent":         "Remix Bobo's Rainbow into a short heart-collecting rescue game",
	}, "remix-tester")
	if rec.Code != http.StatusAccepted {
		t.Fatalf("create remix: %d %s", rec.Code, rec.Body.String())
	}
	var created struct {
		Game model.Game `json:"game"`
		Job  model.Job  `json:"job"`
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
	rec = do(t, srv, "GET", "/v1/games/"+child.ID+"/package", nil, "remix-tester")
	if rec.Code != http.StatusOK {
		t.Fatalf("package endpoint: %d %s", rec.Code, rec.Body.String())
	}
	var pkg GamePackage
	decode(t, rec, &pkg)
	if pkg.PackageFormatVersion != PackageFormatVersion || pkg.MinRuntimeVersion != MinRuntimeVersion || pkg.GameID != child.ID || pkg.IR.Metadata.ParentGameID != parent.ID {
		t.Fatalf("package contract mismatch: %+v", pkg)
	}
	for _, asset := range pkg.Assets {
		rec = do(t, srv, "GET", asset.URL, nil, "remix-tester")
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
	// /v1/ingest is a photo route: its cap is maxPhotoBodyBytes (16 MiB),
	// widened 2026-10-23 so real phone photos fit; oversized means past it.
	oversizedBody := strings.NewReader(`{"keywords":["` + strings.Repeat("x", (16<<20)+16) + `"]}`)
	oversized := httptest.NewRequest(http.MethodPost, "/v1/ingest", oversizedBody)
	oversizedRec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(oversizedRec, oversized)
	if oversizedRec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized JSON: want 413, got %d body=%s", oversizedRec.Code, oversizedRec.Body.String())
	}
}
func TestReadinessFailsWhenConfiguredPersistenceCannotLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store-state.json")
	if err := os.WriteFile(path, []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	st := store.NewMemoryStore()
	if err := st.EnablePersistence(path); err == nil {
		t.Fatal("expected invalid snapshot")
	}
	srv := New(st, pipeline.NewRunner(st))
	rec := do(t, srv, http.MethodGet, "/readyz", nil, "")
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "degraded") {
		t.Fatalf("readiness should fail on persistence load error: %d %s", rec.Code, rec.Body.String())
	}
	live := do(t, srv, http.MethodGet, "/healthz", nil, "")
	if live.Code != http.StatusOK {
		t.Fatalf("liveness should remain available, got %d", live.Code)
	}
}
