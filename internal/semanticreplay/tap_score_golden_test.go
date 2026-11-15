package semanticreplay

import (
	"encoding/json"
	"os"
	"testing"

	"doodlequest/internal/gameir"
	"doodlequest/internal/replaytrace"
)

// One cross-language reference corpus, also consumed by the *pure* JS
// state reducer in runtime-tap-score-golden.test.mjs. Neither engine grants
// score attestation or proves parity with the production Pixi interpreter.
func TestTapScoreSharedCrossLanguageGoldenCorpus(t *testing.T) {
	type expectedState struct {
		Score          int64  `json:"score"`
		Terminal       string `json:"terminal"`
		DownEvents     uint32 `json:"downEvents"`
		ActionsApplied uint32 `json:"actionsApplied"`
	}
	type caseRecord struct {
		Name          string        `json:"name"`
		Seed          uint32        `json:"seed"`
		DurationTicks uint32        `json:"durationTicks"`
		ScoreDelta    int64         `json:"scoreDelta"`
		EndGame       *bool         `json:"endGame"`
		TapTicks      []uint32      `json:"tapTicks"`
		Expected      expectedState `json:"expected"`
	}
	type corpus struct {
		Schema string       `json:"schema"`
		Cases  []caseRecord `json:"cases"`
	}
	data, err := os.ReadFile("../../tools/playtest/fixtures/runtime-tap-score-golden-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures corpus
	if err := json.Unmarshal(data, &fixtures); err != nil {
		t.Fatal(err)
	}
	if fixtures.Schema != "runtime-tap-score-golden-v1" || len(fixtures.Cases) < 7 {
		t.Fatalf("bad shared cross-language corpus schema/coverage: %q %d", fixtures.Schema, len(fixtures.Cases))
	}
	names := map[string]bool{}
	for _, tc := range fixtures.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			if tc.Name == "" || names[tc.Name] {
				t.Fatalf("empty or duplicate corpus case %q", tc.Name)
			}
			names[tc.Name] = true
			if tc.DurationTicks == 0 || tc.DurationTicks > replaytrace.MaxDurationTicks ||
				tc.ScoreDelta < -1000 || tc.ScoreDelta > 1000 {
				t.Fatal("golden outside interpreter's declared capability limits")
			}
			ir := &gameir.Document{
				IRVersion: gameir.Version,
				Game: gameir.Game{
					ID: "golden-tap", Title: "Shared Tap Golden",
					EntryScene: "main", Orientation: "any",
				},
				Scenes:        []gameir.Scene{{ID: "main"}},
				InputBindings: []gameir.InputBinding{{Action: "tap", Input: "tap"}},
				Rules: []gameir.Rule{{
					ID:      "score",
					When:    gameir.Condition{Event: "input", InputAction: "tap"},
					Actions: []gameir.Action{{Type: "add_score", Value: float64(tc.ScoreDelta)}},
				}},
			}
			if tc.EndGame != nil {
				ir.Rules[0].Actions = append(ir.Rules[0].Actions,
					gameir.Action{Type: "end_game", Value: *tc.EndGame})
			}
			events := make([]replaytrace.Event, 0, len(tc.TapTicks)*2)
			var previousTick, ordinal uint32
			for i, tick := range tc.TapTicks {
				if tick > tc.DurationTicks || (i != 0 && tick < previousTick) {
					t.Fatal("golden tap ticks must be nondecreasing and bounded")
				}
				if i == 0 || tick != previousTick {
					ordinal = 0
				}
				events = append(events,
					replaytrace.Event{
						Tick: tick, Ordinal: ordinal, Action: "tap", Device: "pointer",
						Phase: "down", Position: &replaytrace.Position{X: 1000, Y: 2000},
					},
					replaytrace.Event{
						Tick: tick, Ordinal: ordinal + 1, Action: "tap", Device: "pointer",
						Phase: "up",
					},
				)
				ordinal += 2
				previousTick = tick
			}
			trace := replaytrace.Trace{
				Schema:        replaytrace.SchemaV2,
				DurationTicks: tc.DurationTicks,
				Events:        events,
			}
			first, err := ReplayDiagnostic(ir, trace, tc.Seed)
			if err != nil {
				t.Fatalf("canonical Go replay failed: %v", err)
			}
			second, err := ReplayDiagnostic(ir, trace, tc.Seed)
			if err != nil || first != second {
				t.Fatalf("Go semantic replay not deterministic: first=%+v second=%+v error=%v", first, second, err)
			}
			if first.Score != tc.Expected.Score ||
				first.Terminal != tc.Expected.Terminal ||
				first.DownEvents != tc.Expected.DownEvents ||
				first.ActionsApplied != tc.Expected.ActionsApplied {
				t.Fatalf("Go replay differs from shared reference golden: %+v vs %+v", first, tc.Expected)
			}
			if first.Attested || first.EvidenceStatus != "unverified" ||
				first.RankingScope != "none" || len(first.StateSHA256) != 64 {
				t.Fatalf("shared conformance must never promote diagnostic score: %+v", first)
			}
		})
	}
}
