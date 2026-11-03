package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"doodlequest/internal/expression"
	"doodlequest/internal/pipeline"
	"doodlequest/internal/store"
)

func TestFeedIncludesVersionedSlimCardsAndViewerState(t *testing.T) {
	t.Setenv("DQ_ALLOW_INSECURE_USER_ID", "true")
	t.Setenv("DQ_FEED_RANKING_ENABLED", "false")

	st := store.NewMemoryStore()
	now := time.Now().UTC()
	game := feedFixtureGame("card-game-1", "creator-1", now)
	game.Title = "Orbit Garden"
	game.Caption = "Grow a tiny universe"
	game.Topics = []string{"space", "cozy"}
	game.PublishedVersion = 7
	game.PlayableExpression = &expression.Document{
		SchemaVersion:        expression.SchemaVersion,
		Status:               expression.StatusReady,
		Source:               expression.Source{Type: "image", Reference: "private://source-image", SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		Creator:              expression.Creator{OwnerID: "creator-1", Consent: true, Visibility: expression.VisibilityPublic},
		Mode:                 expression.ModeWhatIf,
		Intent:               "A quiet garden in the moonlight",
		ChosenInterpretation: "Moon garden",
		Interpretation: expression.Interpretation{
			Preserved: []expression.Claim{{ID: "p1", Text: "garden", Confidence: 0.95, EvidenceIDs: []string{"e1"}}},
			Invented:  []expression.Claim{{ID: "i1", Text: "moon flowers", Confidence: 0.7}},
		},
		Evidence:   []expression.Evidence{{ID: "e1", Kind: "image_region", SourceReference: "private://region/1", Start: 0, End: 1}},
		Generation: expression.Generation{Seed: 42, GameIRVersion: "1.2", PackageVersion: "7"},
	}
	st.CreateGame(game)
	privateGame := feedFixtureGame("card-game-2", "creator-2", now.Add(-time.Minute))
	privateGame.PlayableExpression = &expression.Document{
		SchemaVersion:        expression.SchemaVersion,
		Status:               expression.StatusReady,
		Source:               expression.Source{Type: "image", Reference: "private://do-not-share"},
		Creator:              expression.Creator{OwnerID: "creator-2", Consent: true, Visibility: expression.VisibilityPrivate},
		Mode:                 expression.ModeMakePlayable,
		Intent:               "A private drawing",
		ChosenInterpretation: "Private interpretation",
		Generation:           expression.Generation{Seed: 7, GameIRVersion: "1.2", PackageVersion: "1"},
	}
	st.CreateGame(privateGame)

	srv := New(st, pipeline.NewRunner(st))
	srv.profiles["creator-1"] = creatorProfile{DisplayName: "Creator One", AvatarURL: "https://cdn.example.test/avatar.png", UpdatedAt: now}
	srv.socialMu.Lock()
	srv.reactions[game.ID] = map[string]string{"viewer-1": "like", "viewer-2": "like"}
	srv.saves["viewer-1"] = map[string]bool{game.ID: true}
	srv.follows["viewer-1"] = map[string]struct{}{"creator-1": {}}
	srv.events = append(srv.events,
		socialEvent{UserID: "viewer-2", GameID: game.ID, Kind: "play_start"},
		socialEvent{UserID: "viewer-2", GameID: game.ID, Kind: "completion"},
		socialEvent{UserID: "viewer-2", GameID: game.ID, Kind: "share"},
	)
	srv.socialMu.Unlock()

	req := httptest.NewRequest(http.MethodGet, "/v1/feed?scope=for_you&limit=20", nil)
	req.Header.Set("X-User-ID", "viewer-1")
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("feed returned %d: %s", rec.Code, rec.Body.String())
	}

	var response struct {
		CardSchemaVersion int               `json:"card_schema_version"`
		Games             []json.RawMessage `json:"games"`
		Cards             []map[string]any  `json:"cards"`
		NextCursor        string            `json:"next_cursor"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode feed: %v", err)
	}
	if response.CardSchemaVersion != FeedCardSchemaVersion {
		t.Fatalf("card schema version = %d, want %d", response.CardSchemaVersion, FeedCardSchemaVersion)
	}
	if len(response.Games) != 2 || len(response.Cards) != 2 {
		t.Fatalf("legacy games/cards lengths = %d/%d, want 2/2", len(response.Games), len(response.Cards))
	}

	var legacy map[string]any
	if err := json.Unmarshal(response.Games[0], &legacy); err != nil {
		t.Fatal(err)
	}
	if _, ok := legacy["game_id"]; !ok {
		t.Fatal("legacy games payload lost game_id")
	}

	var card map[string]any
	for _, candidate := range response.Cards {
		if candidate["game_id"] == game.ID {
			card = candidate
			break
		}
	}
	if card == nil {
		t.Fatalf("missing card for %q: %#v", game.ID, response.Cards)
	}
	if _, ok := card["game_ir"]; ok {
		t.Fatal("slim card must not include game_ir")
	}
	expressionCard, ok := card["playable_expression"].(map[string]any)
	if !ok {
		t.Fatalf("public expression projection missing: %#v", card["playable_expression"])
	}
	if expressionCard["intent"] != "A quiet garden in the moonlight" || expressionCard["chosen_interpretation"] != "Moon garden" {
		t.Fatalf("unexpected expression summary: %#v", expressionCard)
	}
	if _, ok := expressionCard["source_reference"]; ok {
		t.Fatal("public expression projection must not expose source references")
	}
	if _, ok := expressionCard["evidence"]; ok {
		t.Fatal("public expression projection must not expose private evidence metadata")
	}
	var legacyGame map[string]any
	if err := json.Unmarshal(response.Games[0], &legacyGame); err != nil {
		t.Fatal(err)
	}
	if _, ok := legacyGame["playable_expression"]; ok {
		t.Fatal("legacy public game payload must not expose owner-only expression provenance")
	}
	if card["version"] != "7" || card["title"] != "Orbit Garden" || card["caption"] != "Grow a tiny universe" {
		t.Fatalf("unexpected card metadata: %#v", card)
	}
	creator, ok := card["creator"].(map[string]any)
	if !ok || creator["id"] != "creator-1" || creator["display_name"] != "Creator One" || creator["avatar_url"] != "https://cdn.example.test/avatar.png" {
		t.Fatalf("creator payload = %#v", card["creator"])
	}
	preview, ok := card["preview"].(map[string]any)
	if !ok || preview["status"] != "pending" {
		t.Fatalf("missing preview must be explicitly pending: %#v", card["preview"])
	}
	if _, ok := preview["poster_url"]; ok {
		t.Fatal("must not invent a poster URL")
	}
	viewer, ok := card["viewer"].(map[string]any)
	if !ok || viewer["liked"] != true || viewer["saved"] != true || viewer["following"] != true {
		t.Fatalf("viewer state = %#v", card["viewer"])
	}
	stats, ok := card["stats"].(map[string]any)
	if !ok || stats["likes"] != float64(2) || stats["play_starts"] != float64(1) || stats["completions"] != float64(1) || stats["shares"] != float64(1) {
		t.Fatalf("stats must reflect real reactions/events: %#v", card["stats"])
	}
	pkg, ok := card["package"].(map[string]any)
	if !ok || pkg["url"] != "/v1/games/card-game-1/package" || pkg["min_runtime_version"] != MinRuntimeVersion {
		t.Fatalf("package payload = %#v", card["package"])
	}

	var second map[string]any
	for _, candidate := range response.Cards {
		if candidate["game_id"] == "card-game-2" {
			second = candidate
			break
		}
	}
	if second == nil {
		t.Fatal("missing second card")
	}
	if _, ok := second["stats"]; ok {
		t.Fatalf("must omit empty/unobserved stats rather than invent values: %#v", second["stats"])
	}
	if _, ok := second["playable_expression"]; ok {
		t.Fatal("private source expression must not be included in public feed cards")
	}
}
