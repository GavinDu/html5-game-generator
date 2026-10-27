package api

// Feed-ranking evaluation harness for issue #18.
//
// Protocol: docs/FEED_RANKING_EVALUATION.md. Each fixture runs the real feed
// handler twice: baseline (DQ_FEED_RANKING_ENABLED=false, newest-first plus
// creator-diversity round-robin) and candidate (flag=true, quality score
// plus exploration plus diversity), on identical candidate sets and filter
// state. Metrics are computed from the returned order and the fixture's own
// event records — never hand-filled.
//
// Run with:
//   go test ./internal/api -run 'TestFeedRankingEvaluation' -v -count=1
//
// Fixture version: 1. Ages are relative to a fixed per-fixture clock so
// wall-clock drift cannot change expected scores within a run.

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"doodlequest/internal/model"
	"doodlequest/internal/pipeline"
	"doodlequest/internal/store"
)

const evalFixtureVersion = "1"

// evalFixture is one deterministic evaluation scenario. games carry their
// fixture clock in CreatedAt; events and reactions are the signal index.
type evalFixture struct {
	name          string
	now           time.Time
	games         []*model.Game
	events        []socialEvent
	reactions     map[string][]string
	viewer        string
	blocks        map[string]map[string]bool
	notInterested map[string]map[string]bool
	mutedTopics   map[string]map[string]bool
	ineligible    map[string]bool
	runs          int
	// noise, when non-nil, is appended to events for a second candidate-mode
	// run whose order must match the clean candidate order exactly.
	noise []socialEvent
}

func evalGame(id, creator string, age time.Duration, now time.Time, mutate func(*model.Game)) *model.Game {
	g := feedFixtureGame(id, creator, now.Add(-age))
	if mutate != nil {
		mutate(g)
	}
	return g
}

func evalEvents(gameID, kind string, n, dur, exp, tti int, now time.Time) []socialEvent {
	out := make([]socialEvent, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, socialEvent{
			EventID:             fmt.Sprintf("eval-%s-%s-%d", gameID, kind, i),
			GameID:              gameID,
			Kind:                kind,
			DurationMS:          dur,
			ExpectedMS:          exp,
			TimeToInteractiveMS: tti,
			CreatedAt:           now,
		})
	}
	return out
}

func evalReactions(users ...string) []string { return users }

func buildEvalServer(t *testing.T, fx *evalFixture, events []socialEvent) *Server {
	t.Helper()
	st := store.NewMemoryStore()
	for _, g := range fx.games {
		st.CreateGame(g)
	}
	srv := New(st, pipeline.NewRunner(st))
	srv.socialMu.Lock()
	srv.events = append(srv.events, events...)
	for id, users := range fx.reactions {
		m := map[string]string{}
		for _, u := range users {
			m[u] = "like"
		}
		srv.reactions[id] = m
	}
	for creator, viewers := range fx.blocks {
		srv.blocks[creator] = viewers
	}
	for viewer, games := range fx.notInterested {
		srv.notInterested[viewer] = games
	}
	for viewer, topics := range fx.mutedTopics {
		srv.mutedTopics[viewer] = topics
	}
	srv.socialMu.Unlock()
	return srv
}

func evalFeedIDs(t *testing.T, srv *Server, viewer string) ([]string, time.Duration, int) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/v1/feed?scope=for_you&limit=50", nil)
	if viewer != "" {
		req.Header.Set("X-User-ID", viewer)
	}
	rec := httptest.NewRecorder()
	start := time.Now()
	srv.Routes().ServeHTTP(rec, req)
	elapsed := time.Since(start)
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
	return ids, elapsed, rec.Code
}

func runEvalMode(t *testing.T, fx *evalFixture, events []socialEvent, enabled string) ([][]string, float64) {
	t.Helper()
	t.Setenv("DQ_FEED_RANKING_ENABLED", enabled)
	if fx.viewer != "" {
		t.Setenv("DQ_ALLOW_INSECURE_USER_ID", "true")
	}
	runs := fx.runs
	if runs < 1 {
		runs = 1
	}
	orders := make([][]string, 0, runs)
	var totalMs float64
	for i := 0; i < runs; i++ {
		srv := buildEvalServer(t, fx, events)
		ids, elapsed, _ := evalFeedIDs(t, srv, fx.viewer)
		orders = append(orders, ids)
		totalMs += float64(elapsed.Microseconds()) / 1000.0
	}
	return orders, totalMs / float64(runs)
}

func ordersIdentical(orders [][]string) bool {
	for i := 1; i < len(orders); i++ {
		if len(orders[i]) != len(orders[0]) {
			return false
		}
		for j := range orders[i] {
			if orders[i][j] != orders[0][j] {
				return false
			}
		}
	}
	return true
}

func distinctOrders(orders [][]string) int {
	seen := map[string]bool{}
	for _, o := range orders {
		seen[strings.Join(o, ",")] = true
	}
	return len(seen)
}

type evalMetrics struct {
	returned      int
	candidates    int
	eventCount    int
	top10Unique   int
	top20Unique   int
	top10Largest  float64
	top20Largest  float64
	top10HHI      float64
	top20HHI      float64
	medianAge10   float64
	medianAge20   float64
	share24h20    float64
	completions   int
	replays       int
	sharesSaves   int
	quickExits    int
	plays         int
	impressions   int
	weighted      float64
	weightedPerIm float64
	hasImpr       bool
	ineligible    int
	duplicates    int
	deterministic bool
	distinctRuns  int
	meanMs        float64
}

func topN(ids []string, n int) []string {
	if len(ids) < n {
		return ids
	}
	return ids[:n]
}

func medianFloat64(vals []float64) float64 {
	if len(vals) == 0 {
		return math.NaN()
	}
	sorted := append([]float64(nil), vals...)
	sort.Float64s(sorted)
	mid := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[mid]
	}
	return (sorted[mid-1] + sorted[mid]) / 2
}

func computeEvalMetrics(fx *evalFixture, order []string, orders [][]string, meanMs float64) evalMetrics {
	creatorOf := map[string]string{}
	ageOf := map[string]float64{}
	for _, g := range fx.games {
		creatorOf[g.ID] = g.OwnerID
		ageOf[g.ID] = fx.now.Sub(g.CreatedAt).Hours()
	}
	diversity := func(ids []string) (unique int, largest, hhi float64) {
		counts := map[string]int{}
		for _, id := range ids {
			counts[creatorOf[id]]++
		}
		unique = len(counts)
		for _, c := range counts {
			share := float64(c) / float64(len(ids))
			if share > largest {
				largest = share
			}
			hhi += share * share
		}
		return unique, largest, hhi
	}
	m := evalMetrics{returned: len(order), candidates: len(order), eventCount: len(fx.events)}
	for _, users := range fx.reactions {
		m.eventCount += len(users)
	}
	for _, id := range order {
		if fx.ineligible[id] {
			m.ineligible++
		}
	}
	seen := map[string]bool{}
	for _, id := range order {
		if seen[id] {
			m.duplicates++
		}
		seen[id] = true
	}
	m.deterministic = ordersIdentical(orders)
	m.distinctRuns = distinctOrders(orders)
	m.meanMs = meanMs

	top10 := topN(order, 10)
	top20 := topN(order, 20)
	m.top10Unique, m.top10Largest, m.top10HHI = diversity(top10)
	m.top20Unique, m.top20Largest, m.top20HHI = diversity(top20)
	ages10 := make([]float64, 0, len(top10))
	for _, id := range top10 {
		ages10 = append(ages10, ageOf[id])
	}
	ages20 := make([]float64, 0, len(top20))
	for _, id := range top20 {
		ages20 = append(ages20, ageOf[id])
	}
	m.medianAge10 = medianFloat64(ages10)
	m.medianAge20 = medianFloat64(ages20)
	if len(top20) > 0 {
		fresh := 0
		for _, a := range ages20 {
			if a <= 24 {
				fresh++
			}
		}
		m.share24h20 = float64(fresh) / float64(len(top20))
	}
	weightedByGame := feedEngagementByGame(fx.events)
	top10Set := map[string]bool{}
	for _, id := range top10 {
		top10Set[id] = true
	}
	for _, ev := range fx.events {
		if !top10Set[ev.GameID] {
			continue
		}
		switch ev.Kind {
		case "completion":
			m.completions++
		case "replay":
			m.replays++
		case "share", "save":
			m.sharesSaves++
		case "quick_exit":
			m.quickExits++
		case "play_start":
			m.plays++
		case "impression":
			m.impressions++
		}
	}
	for id := range top10Set {
		m.weighted += weightedByGame[id]
	}
	if m.impressions > 0 {
		m.hasImpr = true
		m.weightedPerIm = m.weighted / float64(m.impressions)
	}
	return m
}

func rate(num, den int) string {
	if den == 0 {
		return "n/a"
	}
	return fmt.Sprintf("%.2f", float64(num)/float64(den))
}

func (m evalMetrics) engagementProxy() string {
	if m.hasImpr {
		return fmt.Sprintf("%.2f (%.3f/impr)", m.weighted, m.weightedPerIm)
	}
	return fmt.Sprintf("%.2f (proxy, no impr)", m.weighted)
}

// evalQualitySet builds a per-game event bundle: impressions, plays,
// completions (dur/exp), replays, shares, saves, quick exits, and a TTI
// sample carried on the play_start events when tti > 0.
func evalQualitySet(gameID string, impr, plays, compl, replay, share, save, qe, tti int, now time.Time) []socialEvent {
	var out []socialEvent
	out = append(out, evalEvents(gameID, "impression", impr, 0, 0, 0, now)...)
	out = append(out, evalEvents(gameID, "play_start", plays, 0, 0, tti, now)...)
	out = append(out, evalEvents(gameID, "completion", compl, 4500, 5000, 0, now)...)
	out = append(out, evalEvents(gameID, "replay", replay, 4000, 5000, 0, now)...)
	out = append(out, evalEvents(gameID, "share", share, 0, 0, 0, now)...)
	out = append(out, evalEvents(gameID, "save", save, 0, 0, 0, now)...)
	out = append(out, evalEvents(gameID, "quick_exit", qe, 0, 0, 0, now)...)
	return out
}

func evalFixtures(now time.Time) []*evalFixture {
	h := time.Hour
	cold := &evalFixture{
		name: "cold-start",
		now:  now,
		games: []*model.Game{
			evalGame("c1-a", "creator-1", 1*h, now, nil),
			evalGame("c1-b", "creator-1", 2*h, now, nil),
			evalGame("c2-a", "creator-2", 5*h, now, nil),
			evalGame("c2-b", "creator-2", 26*h, now, nil),
			evalGame("c3-a", "creator-3", 30*h, now, nil),
			evalGame("c3-b", "creator-3", 49*h, now, nil),
		},
		runs: 5,
	}

	var balancedEvents []socialEvent
	balancedSpecs := []struct {
		id, creator                                      string
		age                                              time.Duration
		impr, plays, compl, replay, share, save, qe, tti int
		likes                                            []string
	}{
		{"ca-1", "creator-a", 3 * h, 10, 7, 4, 2, 1, 1, 0, 900, evalReactions("u1", "u2", "u3")},
		{"ca-2", "creator-a", 20 * h, 10, 5, 2, 0, 1, 0, 1, 1200, evalReactions("u1")},
		{"ca-3", "creator-a", 50 * h, 10, 3, 1, 0, 0, 0, 2, 2000, nil},
		{"cb-1", "creator-b", 5 * h, 10, 8, 5, 2, 2, 1, 0, 800, evalReactions("u1", "u2", "u3", "u4")},
		{"cb-2", "creator-b", 25 * h, 10, 4, 2, 0, 0, 0, 1, 1500, nil},
		{"cb-3", "creator-b", 60 * h, 10, 2, 0, 0, 0, 0, 1, 2500, nil},
		{"cc-1", "creator-c", 8 * h, 10, 6, 3, 1, 0, 1, 0, 1000, evalReactions("u1", "u2")},
		{"cc-2", "creator-c", 30 * h, 10, 5, 2, 0, 1, 0, 1, 1300, nil},
		{"cc-3", "creator-c", 65 * h, 10, 3, 1, 0, 0, 0, 1, 1800, nil},
		{"cd-1", "creator-d", 12 * h, 10, 7, 4, 1, 2, 0, 0, 900, evalReactions("u1", "u2")},
		{"cd-2", "creator-d", 35 * h, 10, 4, 2, 0, 0, 0, 1, 1400, nil},
		{"cd-3", "creator-d", 70 * h, 10, 2, 0, 0, 0, 0, 2, 3000, nil},
	}
	balanced := &evalFixture{name: "balanced", now: now, reactions: map[string][]string{}, runs: 5}
	for _, s := range balancedSpecs {
		balanced.games = append(balanced.games, evalGame(s.id, s.creator, s.age, now, nil))
		balancedEvents = append(balancedEvents, evalQualitySet(s.id, s.impr, s.plays, s.compl, s.replay, s.share, s.save, s.qe, s.tti, now)...)
		if len(s.likes) > 0 {
			balanced.reactions[s.id] = s.likes
		}
	}
	balanced.events = balancedEvents

	var burstEvents []socialEvent
	burst := &evalFixture{name: "single-creator-burst", now: now, reactions: map[string][]string{}, runs: 5}
	for i := 1; i <= 4; i++ {
		id := fmt.Sprintf("burst-%d", i)
		burst.games = append(burst.games, evalGame(id, "creator-burst", time.Duration(40+10*i)*h, now, nil))
		burstEvents = append(burstEvents, evalEvents(id, "completion", 20, 4500, 5000, 0, now)...)
		burstEvents = append(burstEvents, evalEvents(id, "replay", 5, 4000, 5000, 0, now)...)
		burstEvents = append(burstEvents, evalEvents(id, "share", 5, 0, 0, 0, now)...)
		burst.reactions[id] = evalReactions("u1", "u2", "u3", "u4", "u5", "u6", "u7", "u8", "u9", "u10")
	}
	for i, creator := range []string{"creator-b", "creator-c", "creator-d", "creator-e"} {
		for j := 1; j <= 2; j++ {
			id := fmt.Sprintf("%s-%d", creator, j)
			burst.games = append(burst.games, evalGame(id, creator, time.Duration(1+i*2+j)*h, now, nil))
			burstEvents = append(burstEvents, evalEvents(id, "completion", 2, 4500, 5000, 0, now)...)
			burstEvents = append(burstEvents, evalEvents(id, "share", 1, 0, 0, 0, now)...)
		}
	}
	burst.events = burstEvents

	fresh := &evalFixture{name: "freshness-conflict", now: now, reactions: map[string][]string{}, runs: 5}
	fresh.games = []*model.Game{
		evalGame("old-engaged", "creator-old", 72*h, now, nil),
		evalGame("new-low", "creator-new", 1*h, now, nil),
		evalGame("mid-1", "creator-mid", 10*h, now, nil),
		evalGame("mid-2", "creator-mid", 20*h, now, nil),
		evalGame("filler-old", "creator-old", 96*h, now, nil),
	}
	fresh.events = append(fresh.events, evalEvents("old-engaged", "completion", 30, 4500, 5000, 0, now)...)
	fresh.events = append(fresh.events, evalEvents("old-engaged", "share", 5, 0, 0, 0, now)...)
	fresh.events = append(fresh.events, evalEvents("mid-1", "completion", 5, 4500, 5000, 0, now)...)
	fresh.events = append(fresh.events, evalEvents("mid-2", "completion", 2, 4500, 5000, 0, now)...)
	fresh.reactions["old-engaged"] = evalReactions("u1", "u2", "u3", "u4", "u5", "u6", "u7", "u8", "u9", "u10")

	cleanGames := []*model.Game{
		evalGame("n-g1", "creator-n1", 4*h, now, nil),
		evalGame("n-g2", "creator-n2", 9*h, now, nil),
		evalGame("n-g3", "creator-n3", 15*h, now, nil),
		evalGame("n-g4", "creator-n1", 30*h, now, nil),
		evalGame("n-g5", "creator-n2", 50*h, now, nil),
	}
	var cleanEvents []socialEvent
	cleanEvents = append(cleanEvents, evalEvents("n-g1", "completion", 10, 4500, 5000, 0, now)...)
	cleanEvents = append(cleanEvents, evalEvents("n-g2", "completion", 5, 4500, 5000, 0, now)...)
	cleanEvents = append(cleanEvents, evalEvents("n-g2", "share", 3, 0, 0, 0, now)...)
	cleanEvents = append(cleanEvents, evalEvents("n-g3", "completion", 2, 4500, 5000, 0, now)...)
	var noise []socialEvent
	noise = append(noise, evalEvents("n-g1", "teleport", 10, 0, 0, 0, now)...)
	noise = append(noise, evalEvents("n-g2", "bounce", 10, 0, 0, 0, now)...)
	noise = append(noise, evalEvents("n-g3", "unknown-kind", 10, 0, 0, 0, now)...)
	noise = append(noise, evalEvents("", "completion", 5, 4500, 5000, 0, now)...)
	noise = append(noise, evalEvents("", "share", 5, 0, 0, 0, now)...)
	noise = append(noise, evalEvents("n-g1", "player_ready", 5, 0, 0, 0, now)...)
	noise = append(noise, evalEvents("n-g2", "player_session_summary", 5, 0, 0, 0, now)...)
	noisy := &evalFixture{name: "noisy-events", now: now, games: cleanGames, events: cleanEvents, noise: noise, runs: 5}

	var sparseEvents []socialEvent
	sparse := &evalFixture{name: "sparse-creators", now: now, runs: 5}
	for i := 1; i <= 6; i++ {
		for j, creator := range []string{"creator-x", "creator-y"} {
			id := fmt.Sprintf("sparse-%s-%d", creator, i)
			sparse.games = append(sparse.games, evalGame(id, creator, time.Duration(i*7+j)*h, now, nil))
			sparseEvents = append(sparseEvents, evalEvents(id, "completion", i, 4500, 5000, 0, now)...)
			sparseEvents = append(sparseEvents, evalEvents(id, "share", 1, 0, 0, 0, now)...)
		}
	}
	sparse.events = sparseEvents

	filter := &evalFixture{
		name:          "filter-interaction",
		now:           now,
		viewer:        "eval-viewer",
		blocks:        map[string]map[string]bool{"blocked-creator": {"eval-viewer": true}, "eval-viewer": {"mean-creator": true}},
		notInterested: map[string]map[string]bool{"eval-viewer": {"ni-game": true}},
		mutedTopics:   map[string]map[string]bool{"eval-viewer": {"muted": true}},
		ineligible: map[string]bool{
			"priv-game": true, "quar-game": true, "pend-game": true, "rej-game": true,
			"draft-game": true, "blocked-game": true, "mean-game": true, "ni-game": true, "muted-game": true,
		},
		runs: 5,
	}
	for i := 1; i <= 4; i++ {
		// Distinct ages per game: equal ages plus equal events would tie
		// scores and exercise the store tie-break instead of the filters.
		for j, creator := range []string{"creator-ok-a", "creator-ok-b"} {
			id := fmt.Sprintf("ok-%s-%d", creator, i)
			age := time.Duration(i*6)*h + time.Duration(j)*30*time.Minute
			filter.games = append(filter.games, evalGame(id, creator, age, now, nil))
			filter.events = append(filter.events, evalEvents(id, "completion", 4, 4500, 5000, 0, now)...)
		}
	}
	filter.games = append(filter.games,
		evalGame("priv-game", "creator-ok-a", 2*h, now, func(g *model.Game) { g.Visibility = model.VisibilityPrivate }),
		evalGame("quar-game", "creator-ok-a", 2*h, now, func(g *model.Game) { g.Quarantined = true }),
		evalGame("pend-game", "creator-ok-a", 2*h, now, func(g *model.Game) { g.ModerationStatus = "pending" }),
		evalGame("rej-game", "creator-ok-a", 2*h, now, func(g *model.Game) { g.ModerationStatus = "rejected" }),
		evalGame("draft-game", "creator-ok-a", 2*h, now, func(g *model.Game) { g.Status = model.GameDraft }),
		evalGame("blocked-game", "blocked-creator", 2*h, now, nil),
		evalGame("mean-game", "mean-creator", 2*h, now, nil),
		evalGame("ni-game", "creator-ok-b", 2*h, now, nil),
		evalGame("muted-game", "creator-ok-b", 2*h, now, func(g *model.Game) { g.Topics = []string{"#muted"} }),
	)

	tie := &evalFixture{name: "tie-timestamps", now: now, runs: 20}
	for i := 1; i <= 4; i++ {
		tie.games = append(tie.games,
			evalGame(fmt.Sprintf("tie-a-%d", i), "creator-tie-a", 5*h, now, nil),
			evalGame(fmt.Sprintf("tie-b-%d", i), "creator-tie-b", 5*h, now, nil),
		)
	}

	preview := &evalFixture{name: "preview-quality", now: now, runs: 5}
	preview.games = []*model.Game{
		evalGame("good", "creator-good", 6*h, now, nil),
		evalGame("slow", "creator-slow", 6*h+5*time.Minute, now, nil),
		evalGame("fill-1", "creator-f1", 3*h, now, nil),
		evalGame("fill-2", "creator-f2", 12*h, now, nil),
		evalGame("fill-3", "creator-f3", 24*h, now, nil),
		evalGame("fill-4", "creator-f4", 48*h, now, nil),
	}
	preview.events = append(preview.events, evalQualitySet("good", 10, 8, 6, 2, 0, 0, 0, 800, now)...)
	preview.events = append(preview.events, evalQualitySet("slow", 10, 8, 1, 0, 0, 0, 5, 6000, now)...)
	for _, id := range []string{"fill-1", "fill-2", "fill-3", "fill-4"} {
		preview.events = append(preview.events, evalQualitySet(id, 10, 4, 2, 0, 0, 0, 0, 1200, now)...)
	}

	explore := &evalFixture{name: "exploration", now: now, runs: 5}
	for i := 1; i <= 10; i++ {
		id := fmt.Sprintf("hi-%02d", i)
		explore.games = append(explore.games, evalGame(id, "creator-"+id, time.Duration(2*i)*h, now, nil))
		explore.events = append(explore.events, evalQualitySet(id, 10, 5, 2, 0, 0, 0, 0, 1000, now)...)
	}
	for i := 1; i <= 5; i++ {
		id := fmt.Sprintf("lo-%02d", i)
		explore.games = append(explore.games, evalGame(id, "creator-"+id, time.Duration(2*i-1)*h, now, nil))
		explore.events = append(explore.events, evalEvents(id, "completion", 1, 4500, 5000, 0, now)...)
	}

	return []*evalFixture{cold, balanced, burst, fresh, noisy, sparse, filter, tie, preview, explore}
}

func indexOf(ids []string, want string) int {
	for i, id := range ids {
		if id == want {
			return i
		}
	}
	return -1
}

func TestFeedRankingEvaluation(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	fixtures := evalFixtures(now)
	t.Logf("feed ranking evaluation fixture version %s (command: go test ./internal/api -run 'TestFeedRankingEvaluation' -v -count=1)", evalFixtureVersion)
	t.Logf("%-22s %-10s %5s %5s %7s %7s %9s %9s %22s %5s %5s %6s %8s", "fixture", "mode", "top10u", "top20u", "large10", "hhi10", "medAge10", "share24h", "engProxy(top10)", "inel", "dup", "determ", "ms")
	type agg struct{ uniq10, largest, medAge, share24, wpi float64 }
	aggs := map[string]*agg{"baseline": {}, "candidate": {}}
	counts := map[string]int{}
	wpiCounts := map[string]int{}
	for _, fx := range fixtures {
		baseOrders, baseMs := runEvalMode(t, fx, fx.events, "false")
		candOrders, candMs := runEvalMode(t, fx, fx.events, "true")
		base := computeEvalMetrics(fx, baseOrders[0], baseOrders, baseMs)
		cand := computeEvalMetrics(fx, candOrders[0], candOrders, candMs)
		for mode, m := range map[string]evalMetrics{"baseline": base, "candidate": cand} {
			t.Logf("%-22s %-10s %5d %5d %7.2f %7.3f %9.1f %9.2f %22s %5d %5d %6v %8.3f",
				fx.name, mode, m.top10Unique, m.top20Unique, m.top10Largest, m.top10HHI,
				m.medianAge10, m.share24h20, m.engagementProxy(), m.ineligible, m.duplicates, m.deterministic, m.meanMs)
			t.Logf("  %-22s %-10s top10 events: compl=%d replay=%d share+save=%d quick_exit=%d plays=%d impr=%d rates compl/play=%s replay/compl=%s qe/play=%s tap/impr=%s",
				fx.name, mode, m.completions, m.replays, m.sharesSaves, m.quickExits, m.plays, m.impressions,
				rate(m.completions, m.plays), rate(m.replays, m.completions), rate(m.quickExits, m.plays), rate(m.plays, m.impressions))
			t.Logf("  %-22s %-10s order=%v", fx.name, mode, map[string][]string{"baseline": baseOrders[0], "candidate": candOrders[0]}[mode])
			a := aggs[mode]
			a.uniq10 += float64(m.top10Unique)
			a.largest += m.top10Largest
			a.medAge += m.medianAge10
			a.share24 += m.share24h20
			if m.hasImpr {
				a.wpi += m.weightedPerIm
				wpiCounts[mode]++
			}
			counts[mode]++
		}
		eligible := len(fx.games) - len(fx.ineligible)
		for mode, m := range map[string]evalMetrics{"baseline": base, "candidate": cand} {
			if m.returned != eligible {
				t.Errorf("%s %s: returned %d games, want %d eligible", fx.name, mode, m.returned, eligible)
			}
			if m.ineligible != 0 {
				t.Errorf("%s %s: %d ineligible games in feed, want 0", fx.name, mode, m.ineligible)
			}
			if m.duplicates != 0 {
				t.Errorf("%s %s: %d duplicate game IDs in feed, want 0", fx.name, mode, m.duplicates)
			}
			if !m.deterministic {
				t.Errorf("%s %s: feed order not deterministic across %d runs (%d distinct orders); tie-break instability is in ListGames (unstable sort over map iteration), not the ranker — see results doc",
					fx.name, mode, len(map[string][][]string{"baseline": baseOrders, "candidate": candOrders}[mode]), m.distinctRuns)
			}
		}
		if len(baseOrders[0]) != len(candOrders[0]) {
			t.Errorf("%s: baseline returned %d games, candidate %d", fx.name, len(baseOrders[0]), len(candOrders[0]))
		}
		switch fx.name {
		case "cold-start":
			if strings.Join(baseOrders[0], ",") != strings.Join(candOrders[0], ",") {
				t.Errorf("cold-start: candidate order %v != baseline order %v, want exact parity", candOrders[0], baseOrders[0])
			}
		case "freshness-conflict":
			if indexOf(candOrders[0], "old-engaged") > indexOf(candOrders[0], "new-low") {
				t.Errorf("freshness-conflict: candidate ranks new-low above old-engaged: %v", candOrders[0])
			}
			if indexOf(baseOrders[0], "new-low") > indexOf(baseOrders[0], "old-engaged") {
				t.Errorf("freshness-conflict: baseline ranks old-engaged above new-low: %v", baseOrders[0])
			}
		case "preview-quality":
			if indexOf(candOrders[0], "good") > indexOf(candOrders[0], "slow") {
				t.Errorf("preview-quality: candidate ranks slow above good: %v", candOrders[0])
			}
		case "exploration":
			for _, idx := range []int{4, 9, 14} {
				if idx < len(candOrders[0]) && !strings.HasPrefix(candOrders[0][idx], "lo-") {
					t.Errorf("exploration: candidate slot %d = %q, want a low-impression game: %v", idx+1, candOrders[0][idx], candOrders[0])
				}
			}
		case "filter-interaction":
			for _, id := range []string{"ok-creator-ok-a-1", "ok-creator-ok-b-1"} {
				if indexOf(baseOrders[0], id) < 0 || indexOf(candOrders[0], id) < 0 {
					t.Errorf("filter-interaction: eligible game %q missing from a feed", id)
				}
			}
		}
		if fx.noise != nil {
			noisy := append(append([]socialEvent(nil), fx.events...), fx.noise...)
			noisyBase, _ := runEvalMode(t, fx, noisy, "false")
			noisyCand, _ := runEvalMode(t, fx, noisy, "true")
			if strings.Join(noisyBase[0], ",") != strings.Join(baseOrders[0], ",") {
				t.Errorf("noisy-events: baseline order changed with noise: clean=%v noisy=%v", baseOrders[0], noisyBase[0])
			}
			if strings.Join(noisyCand[0], ",") != strings.Join(candOrders[0], ",") {
				t.Errorf("noisy-events: candidate order changed with noise: clean=%v noisy=%v", candOrders[0], noisyCand[0])
			}
		}
	}
	for mode, a := range aggs {
		n := float64(counts[mode])
		wpiMean := math.NaN()
		if wpiCounts[mode] > 0 {
			wpiMean = a.wpi / float64(wpiCounts[mode])
		}
		t.Logf("AGGREGATE %-10s mean top10 unique=%.2f mean largest share=%.2f mean median age=%.1fh mean 24h share=%.2f mean weighted/impr=%.3f (over %d fixtures with impressions)",
			mode, a.uniq10/n, a.largest/n, a.medAge/n, a.share24/n, wpiMean, wpiCounts[mode])
	}
}
