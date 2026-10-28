package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"doodlequest/internal/pipeline"
	"doodlequest/internal/store"
)

type fakePreviewScanner struct {
	result PreviewScanResult
	err    error
}

func (f fakePreviewScanner) ScanPreview(context.Context, string, int, string, []byte) (PreviewScanResult, error) {
	return f.result, f.err
}

func TestPreviewScannerRejectsBeforeStorageAndAudits(t *testing.T) {
	t.Setenv("DQ_ALLOW_INSECURE_USER_ID", "true")
	t.Setenv("DQ_MODERATION_AUDIT_FILE", filepath.Join(t.TempDir(), "audit.jsonl"))
	st := store.NewMemoryStore()
	game := feedFixtureGame("scan-rejected", "creator-1", time.Now().UTC())
	game.PublishedVersion = 1
	st.CreateGame(game)
	srv := New(st, pipeline.NewRunner(st))
	srv.SetPreviewScanner(fakePreviewScanner{result: PreviewScanResult{Status: "rejected", Reason: "unsafe visual"}})

	png, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/k7sAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPut, "/v1/games/scan-rejected/previews/1/poster.png", bytes.NewReader(png))
	req.Header.Set("X-User-ID", "creator-1")
	req.Header.Set("Content-Type", "image/png")
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("rejected scan status = %d: %s", rec.Code, rec.Body.String())
	}
	if _, _, found := st.GetArtifact(game.ID, previewArtifactKey(1, "poster.png")); found {
		t.Fatal("rejected preview was stored")
	}
	audit, err := os.ReadFile(os.Getenv("DQ_MODERATION_AUDIT_FILE"))
	if err != nil || !strings.Contains(string(audit), `"status":"rejected"`) {
		t.Fatalf("rejected scan audit missing: %v %s", err, audit)
	}
}

func TestPreviewScannerUnavailableFailsClosedAndAudits(t *testing.T) {
	t.Setenv("DQ_ALLOW_INSECURE_USER_ID", "true")
	t.Setenv("DQ_MODERATION_AUDIT_FILE", filepath.Join(t.TempDir(), "audit.jsonl"))
	st := store.NewMemoryStore()
	game := feedFixtureGame("scan-error", "creator-1", time.Now().UTC())
	game.PublishedVersion = 1
	st.CreateGame(game)
	srv := New(st, pipeline.NewRunner(st))
	srv.SetPreviewScanner(fakePreviewScanner{err: errors.New("provider timeout")})

	png, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/k7sAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPut, "/v1/games/scan-error/previews/1/poster.png", bytes.NewReader(png))
	req.Header.Set("X-User-ID", "creator-1")
	req.Header.Set("Content-Type", "image/png")
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("scanner error status = %d: %s", rec.Code, rec.Body.String())
	}
	if _, _, found := st.GetArtifact(game.ID, previewArtifactKey(1, "poster.png")); found {
		t.Fatal("preview was stored after scanner error")
	}
	audit, err := os.ReadFile(os.Getenv("DQ_MODERATION_AUDIT_FILE"))
	if err != nil || !strings.Contains(string(audit), `"status":"error"`) {
		t.Fatalf("scanner error audit missing: %v %s", err, audit)
	}
}

func TestRequiredPreviewScannerMissingFailsClosedAndAuditsPending(t *testing.T) {
	t.Setenv("DQ_ALLOW_INSECURE_USER_ID", "true")
	t.Setenv("DQ_MODERATION_AUDIT_FILE", filepath.Join(t.TempDir(), "audit.jsonl"))
	t.Setenv("DQ_PREVIEW_SCAN_REQUIRED", "false")
	st := store.NewMemoryStore()
	game := feedFixtureGame("scan-required-missing", "creator-1", time.Now().UTC())
	game.PublishedVersion = 1
	st.CreateGame(game)
	srv := New(st, pipeline.NewRunner(st))
	srv.SetPreviewScanRequired(true)

	png, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/k7sAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPut, "/v1/games/scan-required-missing/previews/1/poster.png", bytes.NewReader(png))
	req.Header.Set("X-User-ID", "creator-1")
	req.Header.Set("Content-Type", "image/png")
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("missing required scanner status = %d: %s", rec.Code, rec.Body.String())
	}
	if _, _, found := st.GetArtifact(game.ID, previewArtifactKey(1, "poster.png")); found {
		t.Fatal("preview was stored without the required scanner")
	}
	audit, err := os.ReadFile(os.Getenv("DQ_MODERATION_AUDIT_FILE"))
	if err != nil || !strings.Contains(string(audit), `"status":"pending"`) ||
		!strings.Contains(string(audit), "required preview safety scanner is not configured") {
		t.Fatalf("pending scan audit missing: %v %s", err, audit)
	}
}


func TestSourceImageRequiresExplicitOptInAndPassingRescan(t *testing.T) {
	t.Setenv("DQ_ALLOW_INSECURE_USER_ID", "true")
	t.Setenv("DQ_MODERATION_AUDIT_FILE", filepath.Join(t.TempDir(), "audit.jsonl"))
	st := store.NewMemoryStore()
	game := feedFixtureGame("source-opt-in", "creator-1", time.Now().UTC())
	game.PublishedVersion = 1
	st.CreateGame(game)
	srv := New(st, pipeline.NewRunner(st))
	srv.SetPreviewScanner(fakePreviewScanner{result: PreviewScanResult{Status: "passed", Reason: "safe"}})

	png, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/k7sAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}
	request := func(method, path, contentType string, body []byte) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, bytes.NewReader(body))
		req.Header.Set("X-User-ID", "creator-1")
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		rec := httptest.NewRecorder()
		srv.Routes().ServeHTTP(rec, req)
		return rec
	}
	for _, asset := range []string{"poster.png", "source.png"} {
		if rec := request(http.MethodPut, "/v1/games/source-opt-in/previews/1/"+asset, "image/png", png); rec.Code != http.StatusCreated {
			t.Fatalf("%s upload status = %d: %s", asset, rec.Code, rec.Body.String())
		}
	}
	metadata := []byte(`{"version":1,"status":"failed","show_source_image":false}`)
	if rec := request(http.MethodPut, "/v1/games/source-opt-in/preview", "application/json", metadata); rec.Code != http.StatusOK {
		t.Fatalf("metadata without source opt-in status = %d: %s", rec.Code, rec.Body.String())
	}
	if rec := request(http.MethodGet, "/v1/games/source-opt-in/previews/1/source.png", "", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("source image without opt-in status = %d, want 404: %s", rec.Code, rec.Body.String())
	}

	metadata = []byte(`{"version":1,"status":"failed","show_source_image":true}`)
	if rec := request(http.MethodPut, "/v1/games/source-opt-in/preview", "application/json", metadata); rec.Code != http.StatusOK {
		t.Fatalf("metadata with source opt-in status = %d: %s", rec.Code, rec.Body.String())
	}
	if rec := request(http.MethodGet, "/v1/games/source-opt-in/previews/1/source.png", "", nil); rec.Code != http.StatusOK {
		t.Fatalf("opted-in source image status = %d: %s", rec.Code, rec.Body.String())
	}
	audit, err := os.ReadFile(os.Getenv("DQ_MODERATION_AUDIT_FILE"))
	if err != nil || !strings.Contains(string(audit), `"action":"source_image_scan"`) || !strings.Contains(string(audit), `"status":"passed"`) {
		t.Fatalf("source image scan audit missing: %v %s", err, audit)
	}
}

func TestSourceImageOptInFailsWhenRescanRejectsPreviouslyStoredAsset(t *testing.T) {
	t.Setenv("DQ_ALLOW_INSECURE_USER_ID", "true")
	t.Setenv("DQ_MODERATION_AUDIT_FILE", filepath.Join(t.TempDir(), "audit.jsonl"))
	t.Setenv("DQ_PREVIEW_SCAN_REQUIRED", "false")
	st := store.NewMemoryStore()
	game := feedFixtureGame("source-rescan-rejected", "creator-1", time.Now().UTC())
	game.PublishedVersion = 1
	st.CreateGame(game)
	srv := New(st, pipeline.NewRunner(st))

	png, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/k7sAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}
	request := func(method, path, contentType string, body []byte) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, bytes.NewReader(body))
		req.Header.Set("X-User-ID", "creator-1")
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		rec := httptest.NewRecorder()
		srv.Routes().ServeHTTP(rec, req)
		return rec
	}
	for _, asset := range []string{"poster.png", "source.png"} {
		if rec := request(http.MethodPut, "/v1/games/source-rescan-rejected/previews/1/"+asset, "image/png", png); rec.Code != http.StatusCreated {
			t.Fatalf("%s upload status = %d: %s", asset, rec.Code, rec.Body.String())
		}
	}
	srv.SetPreviewScanner(fakePreviewScanner{result: PreviewScanResult{Status: "rejected", Reason: "unsafe source image"}})
	metadata := []byte(`{"version":1,"status":"failed","show_source_image":true}`)
	rec := request(http.MethodPut, "/v1/games/source-rescan-rejected/preview", "application/json", metadata)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("rejected source image opt-in status = %d, want 422: %s", rec.Code, rec.Body.String())
	}
	gameAfter, ok := st.GetGame(game.ID)
	if !ok || gameAfter.Preview != nil || (gameAfter.Preview != nil && gameAfter.Preview.SourceImageURL != "") {
		t.Fatalf("rejected source image was exposed in preview metadata: %#v", gameAfter.Preview)
	}
}
