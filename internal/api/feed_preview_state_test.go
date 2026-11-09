package api

import (
	"testing"

	"doodlequest/internal/model"
)

func TestFeedPreviewFailsClosedOnUnapprovedAndIncompleteMedia(t *testing.T) {
	const poster = "/v1/games/game/previews/2/poster.png"
	const clip = "/v1/games/game/previews/2/clip.mp4"
	tests := []struct {
		name string
		media *model.PreviewMetadata
		wantStatus, wantPoster, wantClip string
		wantDuration int
	}{
		{"missing", nil, "pending", "", "", 0},
		{"pending retains draft URLs", &model.PreviewMetadata{Version: 2, Status: "pending", PosterURL: poster, ClipURL: clip, DurationMS: 6000}, "pending", "", "", 0},
		{"stale package version", &model.PreviewMetadata{Version: 1, Status: "ready", PosterURL: poster, ClipURL: clip, DurationMS: 6000}, "pending", "", "", 0},
		{"unknown approval state", &model.PreviewMetadata{Version: 2, Status: "reviewing", PosterURL: poster, ClipURL: clip, DurationMS: 6000}, "pending", "", "", 0},
		{"missing poster", &model.PreviewMetadata{Version: 2, Status: "ready", ClipURL: clip, DurationMS: 6000}, "pending", "", "", 0},
		{"missing clip", &model.PreviewMetadata{Version: 2, Status: "ready", PosterURL: poster, DurationMS: 6000}, "pending", "", "", 0},
		{"missing duration", &model.PreviewMetadata{Version: 2, Status: "ready", PosterURL: poster, ClipURL: clip}, "pending", "", "", 0},
		{"excessive duration", &model.PreviewMetadata{Version: 2, Status: "ready", PosterURL: poster, ClipURL: clip, DurationMS: 10001}, "pending", "", "", 0},
		{"valid approved media", &model.PreviewMetadata{Version: 2, Status: "ready", PosterURL: poster, ClipURL: clip, DurationMS: 6000}, "ready", poster, clip, 6000},
		{"failed with poster but stale clip", &model.PreviewMetadata{Version: 2, Status: "failed", PosterURL: poster, ClipURL: clip, DurationMS: 6000}, "failed", poster, "", 0},
		{"failed without poster", &model.PreviewMetadata{Version: 2, Status: "failed", ClipURL: clip}, "pending", "", "", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			game := &model.Game{ID: "game", PublishedVersion: 2, Preview: tt.media}
			got := safeFeedPreview(game, publishedGameVersion(game))
			if got.Status != tt.wantStatus || got.PosterURL != tt.wantPoster ||
				got.ClipURL != tt.wantClip || got.DurationMS != tt.wantDuration {
				t.Fatalf("preview = %+v; want state=%q poster=%q clip=%q duration=%d",
					got, tt.wantStatus, tt.wantPoster, tt.wantClip, tt.wantDuration)
			}
		})
	}
}

func TestFeedPreviewUnavailableVersionNeverExposesURLs(t *testing.T) {
	if got := safeFeedPreview(nil, 0); got.Status != "pending" {
		t.Fatalf("nil game must be pending: %+v", got)
	}
	game := &model.Game{ID: "game", Preview: &model.PreviewMetadata{
		Version: 2, Status: "ready", PosterURL: "/poster", ClipURL: "/clip", DurationMS: 6000,
	}}
	if got := safeFeedPreview(game, 0); got.Status != "pending" || got.ClipURL != "" {
		t.Fatalf("unpublished preview leaked: %+v", got)
	}
}
