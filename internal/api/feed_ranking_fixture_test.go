package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"doodlequest/internal/model"
	"doodlequest/internal/pipeline"
	"doodlequest/internal/store"
)

func feedFixtureGame(id, creator string, createdAt time.Time) *model.Game {
	return &model.Game{
		ID: id, Title: id, OwnerID: creator,
		Status: model.GameReady, Visibility: model.VisibilityPublic,
		ModerationStatus: "approved", CreatedAt: createdAt,
	}
}

func feedFixtureIDs(t *testing.T, srv *Server) []string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/v1/feed?scope=for_you&limit=20", nil)
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("feed returned %d: %s", rec.Code, rec.Body.String())
	}
	var response struct {
		Games []model.Game `json:"games"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode feed: %v", err)
	}
	ids := make([]string, 0, len(response.Games))
	for _, game := range response.Games {
		ids = append(ids, game.ID)
	}
	return ids
}

func TestFeedRankingFixtureAndNewestFirstRollback(t *testing.T) {
	now := time.Now().UTC()
	st := store.NewMemoryStore()
	st.CreateGame(feedFixtureGame("old-engaged-a", "creator-a", now.Add(-48*time.Hour)))
	st.CreateGame(feedFixtureGame("new-b", "creator-b", now.Add(-6*time.Hour)))
	st.CreateGame(feedFixtureGame("newest-a", "creator-a", now.Add(-time.Hour)))
	srv := New(st, pipeline.NewRunner(st))

	// Exactly 20 deterministic engagement signals cross the cold-start
	// threshold. Their score should outweigh freshness for the older game.
	srv.socialMu.Lock()
	for i := 0; i < 20; i++ {
		srv.events = append(srv.events, socialEvent{
			EventID: "fixture-" + string(rune('a'+i)),
			GameID:  "old-engaged-a", Kind: "completion", CreatedAt: now,
		})
	}
	srv.socialMu.Unlock()

	t.Setenv("DQ_FEED_RANKING_ENABLED", "true")
	got := feedFixtureIDs(t, srv)
	if len(got) != 3 {
		t.Fatalf("feed IDs = %v, want 3 games", got)
	}
	if got[0] != "old-engaged-a" {
		t.Fatalf("engagement ranking order = %v, want engaged older game first", got)
	}
	if got[1] != "new-b" {
		t.Fatalf("creator-diversity order = %v, want creator-b after creator-a's top game", got)
	}

	// Rollback disables engagement scoring but keeps creator diversity:
	// newest creator-a game first, then creator-b, then creator-a's older game.
	t.Setenv("DQ_FEED_RANKING_ENABLED", "false")
	got = feedFixtureIDs(t, srv)
	want := []string{"newest-a", "new-b", "old-engaged-a"}
	if len(got) != len(want) {
		t.Fatalf("rollback feed IDs = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("rollback feed IDs = %v, want %v", got, want)
		}
	}
}

func TestFeedRankingIsRepeatableAndHasUniqueGameIDs(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	st := store.NewMemoryStore()
	fixtures := []struct {
		id, creator string
		age         time.Duration
	}{
		{"a-newest", "creator-a", time.Hour},
		{"a-middle", "creator-a", 2 * time.Hour},
		{"a-old", "creator-a", 3 * time.Hour},
		{"b-new", "creator-b", 90 * time.Minute},
		{"c-new", "creator-c", 2 * time.Hour},
		{"d-new", "creator-d", 150 * time.Minute},
	}
	for _, fixture := range fixtures {
		st.CreateGame(feedFixtureGame(fixture.id, fixture.creator, now.Add(-fixture.age)))
	}
	srv := New(st, pipeline.NewRunner(st))
	t.Setenv("DQ_FEED_RANKING_ENABLED", "false")

	first := feedFixtureIDs(t, srv)
	second := feedFixtureIDs(t, srv)
	if len(first) != len(fixtures) {
		t.Fatalf("feed returned %d games, want %d: %v", len(first), len(fixtures), first)
	}
	if len(second) != len(first) {
		t.Fatalf("second feed returned %d games, want %d", len(second), len(first))
	}
	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("feed order is not repeatable: first=%v second=%v", first, second)
		}
	}

	seen := make(map[string]bool, len(first))
	for _, id := range first {
		if seen[id] {
			t.Errorf("duplicate game ID %q in feed: %v", id, first)
		}
		seen[id] = true
	}
	for _, fixture := range fixtures {
		if !seen[fixture.id] {
			t.Errorf("game %q missing from feed: %v", fixture.id, first)
		}
	}
}

// BenchmarkFeedRanking measures end-to-end feed handler cost with fixed-size
// synthetic candidate/event sets. Run with:
// go test ./internal/api -run '^$' -bench '^BenchmarkFeedRanking$' -benchmem
func BenchmarkFeedRanking(b *testing.B) {
	for _, size := range []int{100, 1000} {
		b.Run(strconv.Itoa(size)+"_games", func(b *testing.B) {
			now := time.Now().UTC().Truncate(time.Second)
			st := store.NewMemoryStore()
			for i := 0; i < size; i++ {
				st.CreateGame(feedFixtureGame(
					"game-"+strconv.Itoa(i),
					"creator-"+strconv.Itoa(i%100),
					now.Add(-time.Duration(i)*time.Minute),
				))
			}
			srv := New(st, pipeline.NewRunner(st))
			srv.socialMu.Lock()
			for i := 0; i < size*2; i++ {
				srv.events = append(srv.events, socialEvent{
					EventID:   "bench-" + strconv.Itoa(i),
					GameID:    "game-" + strconv.Itoa(i%size),
					Kind:      "completion",
					CreatedAt: now,
				})
			}
			srv.socialMu.Unlock()
			req := httptest.NewRequest(http.MethodGet, "/v1/feed?scope=for_you&limit=20", nil)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				rec := httptest.NewRecorder()
				srv.Routes().ServeHTTP(rec, req)
				if rec.Code != http.StatusOK {
					b.Fatalf("feed returned %d: %s", rec.Code, rec.Body.String())
				}
			}
		})
	}
}

func TestFeedQualitySignalsAreSmoothedAndPenalizeSlowOrQuickExit(t *testing.T) {
	events := []socialEvent{
		{GameID: "good", Kind: "impression"},
		{GameID: "good", Kind: "impression"},
		{GameID: "good", Kind: "play_start"},
		{GameID: "good", Kind: "completion", DurationMS: 4000, ExpectedMS: 5000, TimeToInteractiveMS: 1000},
		{GameID: "good", Kind: "replay", DurationMS: 3000, ExpectedMS: 5000},
		{GameID: "slow", Kind: "impression"},
		{GameID: "slow", Kind: "play_start"},
		{GameID: "slow", Kind: "quick_exit", TimeToInteractiveMS: 6000},
	}
	signals := feedQualitySignalsByGame(events)
	if signals["good"].Impressions != 2 || signals["good"].Plays != 1 || signals["good"].Completions != 1 {
		t.Fatalf("good-game counters = %+v", signals["good"])
	}
	now := time.Now().UTC()
	good := feedFixtureGame("good", "creator-good", now.Add(-time.Hour))
	slow := feedFixtureGame("slow", "creator-slow", now.Add(-time.Hour))
	scores := map[string]float64{}
	goodScore := feedQualityScore(good, nil, scores, signals["good"])
	slowScore := feedQualityScore(slow, nil, scores, signals["slow"])
	if goodScore <= slowScore {
		t.Fatalf("quality ranking score good=%f slow=%f; expected satisfying fast game to rank higher", goodScore, slowScore)
	}
}

func TestFeedExplorationReservesOneOfFiveSlotsForLowImpressionGames(t *testing.T) {
	games := make([]*model.Game, 10)
	signals := map[string]feedQualitySignals{}
	for i := range games {
		id := "game-" + strconv.Itoa(i)
		games[i] = feedFixtureGame(id, "creator-"+id, time.Now().Add(-time.Duration(i)*time.Minute))
		if i < 5 {
			signals[id] = feedQualitySignals{Impressions: 10}
		} else {
			signals[id] = feedQualitySignals{Impressions: 0}
		}
	}
	got := reserveExplorationSlots(games, signals, 5)
	if len(got) != len(games) {
		t.Fatalf("exploration result length=%d want %d", len(got), len(games))
	}
	if got[4].ID != "game-5" {
		t.Fatalf("fifth slot = %q, want first low-impression candidate game-5", got[4].ID)
	}
	seen := map[string]bool{}
	for _, game := range got {
		if seen[game.ID] {
			t.Fatalf("duplicate game after exploration: %q", game.ID)
		}
		seen[game.ID] = true
	}
}
