package api

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"doodlequest/internal/pipeline"
	"doodlequest/internal/store"
)

func TestVersionedPreviewAssetsPublishToFeedAndRemainVersionScoped(t *testing.T) {
	t.Setenv("DQ_ALLOW_INSECURE_USER_ID", "true")
	t.Setenv("DQ_FEED_RANKING_ENABLED", "false")
	st := store.NewMemoryStore()
	game := feedFixtureGame("preview-game", "creator-1", time.Now().UTC())
	game.PublishedVersion = 2
	game.ModerationStatus = "approved"
	st.CreateGame(game)
	srv := New(st, pipeline.NewRunner(st))

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

	png, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/k7sAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}
	poster := request(http.MethodPut, "/v1/games/preview-game/previews/2/poster.png", "image/png", png)
	if poster.Code != http.StatusCreated {
		t.Fatalf("poster upload status = %d: %s", poster.Code, poster.Body.String())
	}
	clipBytes := []byte{0, 0, 0, 16, 'f', 't', 'y', 'p', 'i', 's', 'o', 'm', 0, 0, 0, 0}
	clip := request(http.MethodPut, "/v1/games/preview-game/previews/2/clip.mp4", "video/mp4", clipBytes)
	if clip.Code != http.StatusCreated {
		t.Fatalf("clip upload status = %d: %s", clip.Code, clip.Body.String())
	}

	metadata := request(http.MethodPut, "/v1/games/preview-game/preview", "application/json",
		[]byte(`{"version":2,"status":"ready","aspect":0.5625,"duration_ms":6000,"dominant_color":"#112233"}`))
	if metadata.Code != http.StatusOK {
		t.Fatalf("metadata update status = %d: %s", metadata.Code, metadata.Body.String())
	}

	var response struct {
		Cards []struct {
			GameID  string `json:"game_id"`
			Preview struct {
				PosterURL     string  `json:"poster_url"`
				ClipURL       string  `json:"clip_url"`
				Aspect        float64 `json:"aspect"`
				DurationMS    int     `json:"duration_ms"`
				DominantColor string  `json:"dominant_color"`
				Status        string  `json:"status"`
			} `json:"preview"`
		} `json:"cards"`
	}
	feed := request(http.MethodGet, "/v1/feed?scope=for_you", "", nil)
	if feed.Code != http.StatusOK {
		t.Fatalf("feed status = %d: %s", feed.Code, feed.Body.String())
	}
	if err := json.Unmarshal(feed.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	var found *struct {
		GameID  string `json:"game_id"`
		Preview struct {
			PosterURL     string  `json:"poster_url"`
			ClipURL       string  `json:"clip_url"`
			Aspect        float64 `json:"aspect"`
			DurationMS    int     `json:"duration_ms"`
			DominantColor string  `json:"dominant_color"`
			Status        string  `json:"status"`
		} `json:"preview"`
	}
	for i := range response.Cards {
		if response.Cards[i].GameID == "preview-game" {
			found = &response.Cards[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("preview game missing from feed cards: %#v", response.Cards)
	}
	if found.Preview.Status != "ready" || found.Preview.PosterURL != "/v1/games/preview-game/previews/2/poster.png" ||
		found.Preview.ClipURL != "/v1/games/preview-game/previews/2/clip.mp4" ||
		found.Preview.DurationMS != 6000 || found.Preview.DominantColor != "#112233" {
		t.Fatalf("feed preview metadata = %#v", found.Preview)
	}

	delivered := request(http.MethodGet, found.Preview.ClipURL, "", nil)
	if delivered.Code != http.StatusOK || delivered.Body.String() != string(clipBytes) {
		t.Fatalf("clip delivery status = %d body=%q", delivered.Code, delivered.Body.String())
	}
	oldVersion := request(http.MethodPut, "/v1/games/preview-game/previews/1/poster.png", "image/png", png)
	if oldVersion.Code != http.StatusConflict {
		t.Fatalf("old-version upload status = %d, want 409: %s", oldVersion.Code, oldVersion.Body.String())
	}
}

func TestFailedPreviewCanExposePosterFallbackWithoutClip(t *testing.T) {
	t.Setenv("DQ_ALLOW_INSECURE_USER_ID", "true")
	st := store.NewMemoryStore()
	game := feedFixtureGame("poster-only-game", "creator-1", time.Now().UTC())
	game.PublishedVersion = 1
	st.CreateGame(game)
	srv := New(st, pipeline.NewRunner(st))
	png, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/k7sAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPut, "/v1/games/poster-only-game/previews/1/poster.png", bytes.NewReader(png))
	req.Header.Set("X-User-ID", "creator-1")
	req.Header.Set("Content-Type", "image/png")
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("poster upload status = %d: %s", rec.Code, rec.Body.String())
	}
	req = httptest.NewRequest(http.MethodPut, "/v1/games/poster-only-game/preview", bytes.NewBufferString(`{"version":1,"status":"failed"}`))
	req.Header.Set("X-User-ID", "creator-1")
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("poster fallback metadata status = %d: %s", rec.Code, rec.Body.String())
	}
	game, _ = st.GetGame("poster-only-game")
	if game.Preview == nil || game.Preview.Status != "failed" || game.Preview.PosterURL == "" || game.Preview.ClipURL != "" {
		t.Fatalf("poster-only fallback metadata = %#v", game.Preview)
	}
}
