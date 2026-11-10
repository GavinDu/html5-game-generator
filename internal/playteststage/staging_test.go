package playteststage

import (
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "os"
    "strings"
    "testing"
    "net/url"

    "doodlequest/internal/api"
    "doodlequest/internal/gameir"
    "doodlequest/internal/media"
    "doodlequest/internal/model"
    "doodlequest/internal/pipeline"
    "doodlequest/internal/store"
)

func sourceFixture(t *testing.T) (*store.MemoryStore, *gameir.Document) {
    t.Helper()
    raw, err := os.ReadFile("../../examples/game-ir-v1/bobos-rainbow.json")
    if err != nil { t.Fatal(err) }
    doc, err := gameir.DecodeAndValidate(raw)
    if err != nil { t.Fatal(err) }

    st := store.NewMemoryStore()
    st.CreateGame(&model.Game{
        ID: doc.Game.ID,
        Title: doc.Game.Title,
        OwnerID: "real-private-owner",
        Status: model.GameGenerating,
        Visibility: model.VisibilityPrivate,
        ModerationStatus: "pending",
    })
    for _, a := range doc.Assets {
        switch a.Kind {
        case "image":
            st.PutArtifact(doc.Game.ID, a.URI, media.SpritePNG(a.ID), "image/png")
        case "audio":
            st.PutArtifact(doc.Game.ID, a.URI, media.MusicWAV("adventurous"), "audio/wav")
        default:
            t.Fatalf("unexpected fixture asset %q", a.Kind)
        }
    }
    return st, doc
}

func TestStagePublishesOnlyIsolatedLoopbackSnapshot(t *testing.T) {
    original, doc := sourceFixture(t)
    stage, err := Open(original, doc.Game.ID, doc)
    if err != nil { t.Fatal(err) }
    defer stage.Close()

    target, err := url.Parse(stage.BaseURL)
    if err != nil || target.Hostname() != "127.0.0.1" {
        t.Fatalf("stage must bind only IPv4 loopback: %q %v", stage.BaseURL, err)
    }
    get := func(path string) *http.Response {
        t.Helper()
        res, err := http.Get(stage.BaseURL + path)
        if err != nil { t.Fatal(err) }
        t.Cleanup(func(){ _ = res.Body.Close() })
        return res
    }
    packagePath := "/v1/games/" + doc.Game.ID + "/package"
    pkgResponse := get(packagePath)
    if pkgResponse.StatusCode != http.StatusOK {
        t.Fatalf("private staged package: got HTTP %d", pkgResponse.StatusCode)
    }
    if !strings.Contains(pkgResponse.Header.Get("Cache-Control"), "no-store") {
        t.Fatal("unpublished stage must be explicitly non-cacheable")
    }
    var pkg api.GamePackage
    if err := json.NewDecoder(pkgResponse.Body).Decode(&pkg); err != nil { t.Fatal(err) }
    if pkg.GameID != doc.Game.ID || len(pkg.IRSHA256) != 64 || pkg.IRHash != pkg.IRSHA256[:12] {
        t.Fatalf("staged package must bind exact canonical Game IR: %+v", pkg)
    }
    if len(pkg.Assets) != len(doc.Assets) {
        t.Fatalf("staging lost asset bytes: got %d want %d", len(pkg.Assets), len(doc.Assets))
    }
    if res := get("/player?gameId=" + doc.Game.ID); res.StatusCode != http.StatusOK {
        t.Fatalf("staged real player shell: HTTP %d", res.StatusCode)
    }
    if res := get("/web/pixi-runtime.js"); res.StatusCode != http.StatusOK {
        t.Fatalf("staged production runtime: HTTP %d", res.StatusCode)
    }
    if res := get("/v1/games/"+doc.Game.ID+"/assets/bobo"); res.StatusCode != http.StatusOK {
        t.Fatalf("staged private hero bytes: HTTP %d", res.StatusCode)
    }
    if res := get("/v1/games/another-game/package"); res.StatusCode == http.StatusOK {
        t.Fatal("stage must never include another game")
    }
    if res := get("/v1/feed"); res.StatusCode == http.StatusOK {
        t.Fatal("staging must not expose feed discovery")
    }

    // The normal public API is still unable to deliver this unready,
    // private/pending game. Never update its status to Ready as a shortcut.
    production := api.New(original, pipeline.NewRunner(original)).Routes()
    rec := httptest.NewRecorder()
    production.ServeHTTP(rec, httptest.NewRequest("GET", packagePath, nil))
    if rec.Code != http.StatusNotFound {
        t.Fatalf("unpublished game leaked via public package: HTTP %d", rec.Code)
    }
    g, _ := original.GetGame(doc.Game.ID)
    if g.Status != model.GameGenerating || g.Visibility != model.VisibilityPrivate || g.ModerationStatus != "pending" || g.GameIR != nil {
        t.Fatalf("live game was mutated for staging: %+v", g)
    }

    rec = httptest.NewRecorder()
    stageServer := http.DefaultClient
    req, err := http.NewRequest("POST", stage.BaseURL+"/v1/games", strings.NewReader("{}"))
    if err != nil { t.Fatal(err) }
    res, err := stageServer.Do(req)
    if err != nil { t.Fatal(err) }
    defer res.Body.Close()
    if res.StatusCode >= 200 && res.StatusCode < 300 {
        t.Fatal("staging must reject all mutation routes")
    }
}

func TestStageRejectsMissingOrCorruptInputsBeforeOpeningListener(t *testing.T) {
    st, doc := sourceFixture(t)
    _, err := Open(st, "some-other-id", doc)
    if err == nil || !strings.Contains(err.Error(), "not found") {
        t.Fatalf("missing source game must fail: %v", err)
    }
    wrong := *doc
    wrong.Game.ID = "some-other-id"
    _, err = Open(st, doc.Game.ID, &wrong)
    if err == nil || !strings.Contains(err.Error(), "identity") {
        t.Fatalf("mismatched identity must fail: %v", err)
    }
    st.PutArtifact(doc.Game.ID, doc.Assets[0].URI, []byte{}, "image/png")
    _, err = Open(st, doc.Game.ID, doc)
    if err == nil || !strings.Contains(err.Error(), "missing asset") {
        t.Fatalf("missing referenced bytes must fail: %v", err)
    }
    st, doc = sourceFixture(t)
    corrupt := *doc
    corrupt.Assets = append([]gameir.Asset(nil), doc.Assets...)
    corrupt.Assets[0].SHA256 = strings.Repeat("0", 64)
    _, err = Open(st, doc.Game.ID, &corrupt)
    if err == nil || !strings.Contains(err.Error(), "SHA-256 mismatch") {
        t.Fatalf("declared asset digest mismatch must fail: %v", err)
    }
    g, _ := st.GetGame(doc.Game.ID)
    g.Status = model.GameReady
    st.UpdateGame(g)
    _, err = Open(st, doc.Game.ID, doc)
    if err == nil || !strings.Contains(err.Error(), "must be generating") {
        t.Fatalf("staging a public Ready game must fail: %v", err)
    }
}
