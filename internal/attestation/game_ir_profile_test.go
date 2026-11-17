package attestation

import (
	"testing"

	"doodlequest/internal/gameir"
)

func minimalStartScoreIR() *gameir.Document {
	return &gameir.Document{
		IRVersion: gameir.Version,
		Game: gameir.Game{
			ID: "minimal_game", Title: "Minimal trusted score diagnostic",
			EntryScene: "scene", Orientation: "any",
		},
		Scenes: []gameir.Scene{{ID: "scene"}},
		Rules: []gameir.Rule{{
			ID: "start_score",
			When: gameir.Condition{Event: "game_start"},
			Actions: []gameir.Action{{Type: "add_score", Value: float64(3)}},
		}},
	}
}

func TestStrictReplayIRStartScoreProfileValidatesMinimalV1Fixture(t *testing.T) {
	base := minimalStartScoreIR()
	if err := gameir.Validate(base); err != nil {
		t.Fatalf("baseline Game IR must be publishable: %v", err)
	}
	got := supportedPublishedIRProfile(base)
	if got.Kind != replayIRAdmissible || got.Capability != replayStartScoreProfileV1 ||
		got.MaxActions != 1 {
		t.Fatalf("minimal one-event/score preflight refused: %+v", got)
	}
	base.Rules = append(base.Rules, gameir.Rule{
		ID: "next_start", When: gameir.Condition{Event: "game_start"},
		Actions: []gameir.Action{{Type: "add_score", Value: float64(-2)}},
	})
	if err := gameir.Validate(base); err != nil {
		t.Fatalf("two-start-rule Game IR must be valid: %v", err)
	}
	got = supportedPublishedIRProfile(base)
	if got.Kind != replayIRAdmissible || got.MaxActions != 2 {
		t.Fatalf("ordered integer/negative score subset should admit: %+v", got)
	}
}

func TestStrictReplayIRRejectsAllExtraPublishedGameplayBranches(t *testing.T) {
	cases := []struct{
		name string
		mutate func(*gameir.Document)
	}{
		{"game_description", func(d *gameir.Document) { d.Game.Description = "story" }},
		{"game_audience", func(d *gameir.Document) { d.Game.Audience = "general" }},
		{"game_duration", func(d *gameir.Document) { d.Game.SessionTargetSeconds = 60 }},
		{"portrait", func(d *gameir.Document) { d.Game.Orientation = "portrait" }},
		{"metadata", func(d *gameir.Document) { d.Metadata.CreatorDisplayName = "creator" }},
		{"caps", func(d *gameir.Document) { d.Capabilities = []string{"input.pointer"} }},
		{"version11", func(d *gameir.Document) { d.IRVersion = gameir.Version11 }},
		{"multiple_scenes", func(d *gameir.Document) { d.Scenes = append(d.Scenes, gameir.Scene{ID: "other"}) }},
		{"background", func(d *gameir.Document) { d.Scenes[0].BackgroundColor = "#000000" }},
		{"gravity", func(d *gameir.Document) { d.Scenes[0].Gravity = []float64{0, 9.8} }},
		{"world", func(d *gameir.Document) { d.Scenes[0].WorldSize = []float64{320, 480} }},
		{"layers", func(d *gameir.Document) { d.Scenes[0].Layers = []gameir.Layer{{ID:"bg"}} }},
		{"physics_bearing_entity", func(d *gameir.Document) {
			d.Scenes[0].Entities = []gameir.Entity{{ID:"player", Kind:"player", Position:[]float64{10,10}}}
		}},
		{"templates", func(d *gameir.Document) {
			d.Scenes[0].Templates = []gameir.Entity{{ID:"future",Kind:"pickup",Position:[]float64{0,0}}}
		}},
		{"collision", func(d *gameir.Document) { d.Rules[0].When.Event = "collision" }},
		{"timer", func(d *gameir.Document) { d.Rules[0].When.Event = "timer_elapsed" }},
		{"scene_entered", func(d *gameir.Document) { d.Rules[0].When.Event = "scene_entered" }},
		{"source_target", func(d *gameir.Document) { d.Rules[0].When.Source = "player" }},
		{"input_action", func(d *gameir.Document) { d.Rules[0].When.InputAction = "jump" }},
		{"when_value", func(d *gameir.Document) { v:=float64(1);d.Rules[0].When.Value=&v }},
		{"predicate", func(d *gameir.Document) { d.Rules[0].Conditions=[]gameir.Predicate{{Variable:"score",Operator:"eq",Value:0}} }},
		{"set_variable", func(d *gameir.Document) { d.Rules[0].Actions[0].Type="set_variable";d.Rules[0].Actions[0].Variable="score" }},
		{"end_game", func(d *gameir.Document) { d.Rules[0].Actions[0].Type="end_game";d.Rules[0].Actions[0].Value=true }},
		{"fraction_score", func(d *gameir.Document) { d.Rules[0].Actions[0].Value=1.5 }},
		{"string_score", func(d *gameir.Document) { d.Rules[0].Actions[0].Value="3" }},
		{"bool_score", func(d *gameir.Document) { d.Rules[0].Actions[0].Value=true }},
		{"huge_score", func(d *gameir.Document) { d.Rules[0].Actions[0].Value=float64(1_000_001) }},
		{"omitted_score", func(d *gameir.Document) { d.Rules[0].Actions[0].Value=nil }},
		{"duration", func(d *gameir.Document) { d.Rules[0].Actions[0].DurationMS=100 }},
		{"bad_target", func(d *gameir.Document) { d.Rules[0].Actions[0].Target="player" }},
		{"no_rules", func(d *gameir.Document) { d.Rules=nil }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d:=minimalStartScoreIR()
			tc.mutate(d)
			got:=supportedPublishedIRProfile(d)
			if got.Kind==replayIRAdmissible {
				t.Fatalf("unsupported published gameplay admitted under scalar profile: %+v", got)
			}
			if got.Capability!="" || got.MaxActions!=0 {
				t.Fatalf("denied Game IR returned an executable capability: %+v", got)
			}
		})
	}
}

func TestStrictReplayIRInvalidIsNotMisreportedAsUnsupportedOrAdmitted(t *testing.T) {
	for _, tc:=range []struct{
		name string
		mutate func(*gameir.Document)
	}{
		{"bad_version",func(d *gameir.Document){d.IRVersion="999"}},
		{"missing_game_id",func(d *gameir.Document){d.Game.ID=""}},
		{"unknown_event",func(d *gameir.Document){d.Rules[0].When.Event="unrecognized"}},
		{"unknown_action",func(d *gameir.Document){d.Rules[0].Actions[0].Type="unrecognized"}},
	} {
		t.Run(tc.name,func(t *testing.T){
			d:=minimalStartScoreIR()
			tc.mutate(d)
			got:=supportedPublishedIRProfile(d)
			if got.Kind!=replayIRInvalid || got.Reason!="game_ir_validation_failed"{
				t.Fatalf("invalid Game IR not distinguished from missing capability: %+v",got)
			}
		})
	}
	got:=supportedPublishedIRProfile(nil)
	if got.Kind!=replayIRInvalid{
		t.Fatalf("nil IR admitted or not marked invalid: %+v",got)
	}
}

func TestStrictReplayIRRejectsUnboundedRulesOrActions(t *testing.T) {
	d:=minimalStartScoreIR()
	for i:=0;i<8;i++ {
		d.Rules=append(d.Rules,gameir.Rule{
			ID: "rule_"+string(rune('a'+i)), When:gameir.Condition{Event:"game_start"},
			Actions:[]gameir.Action{{Type:"add_score",Value:1}},
		})
	}
	if got:=supportedPublishedIRProfile(d);got.Kind==replayIRAdmissible{
		t.Fatalf("rule budget bypass: %+v",got)
	}
	d=minimalStartScoreIR()
	for i:=0;i<8;i++ {
		d.Rules[0].Actions=append(d.Rules[0].Actions,gameir.Action{Type:"add_score",Value:1})
	}
	if got:=supportedPublishedIRProfile(d);got.Kind==replayIRAdmissible{
		t.Fatalf("per-rule action budget bypass: %+v",got)
	}
}
