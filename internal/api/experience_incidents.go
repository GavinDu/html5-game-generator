package api

import "fmt"

// Experience incidents (Phase E): quality degradations derived from
// telemetry v2 aggregates — the experience counterpart of runtime
// error incidents. Same honesty rules: rate-based claims need a
// session floor, and an incident is an observation, never an
// automatic action (the tuner proposes; humans apply).

type experienceIncident struct {
	GameID    string  `json:"gameId"`
	Kind      string  `json:"kind"`
	Detail    string  `json:"detail"`
	Value     float64 `json:"value"`
	Threshold float64 `json:"threshold"`
	Sessions  int     `json:"sessions"`
}

const experienceMinSessions = 20

func deriveExperienceIncidents(gameID string, p *perfSummary, firstRewardTargetSec int) []experienceIncident {
	if p == nil {
		return nil
	}
	var out []experienceIncident
	add := func(kind, detail string, value, threshold float64) {
		out = append(out, experienceIncident{
			GameID: gameID, Kind: kind, Detail: detail,
			Value: value, Threshold: threshold, Sessions: p.Sessions,
		})
	}
	if p.Sessions >= experienceMinSessions && p.Frames > 0 && p.JankRate > 0.10 {
		add("experience.jank",
			fmt.Sprintf("jank rate %.0f%% of %d frames over %d sessions (p95 %dms)",
				100*p.JankRate, p.Frames, p.Sessions, p.P95FrameMs),
			p.JankRate, 0.10)
	}
	if p.Runs >= experienceMinSessions && p.DeadInputRate > 0.25 {
		add("experience.dead_inputs",
			fmt.Sprintf("%.0f%% of %d run inputs changed nothing — controls or hit areas are not reading",
				100*p.DeadInputRate, p.Runs),
			p.DeadInputRate, 0.25)
	}
	if p.Runs >= experienceMinSessions && p.WinRate < 0.02 {
		add("experience.no_wins",
			fmt.Sprintf("win rate %.1f%% over %d runs — the win is unreachable in practice or the game is far too hard",
				100*p.WinRate, p.Runs),
			p.WinRate, 0.02)
	}
	if p.Runs >= experienceMinSessions && p.WinRate > 0.98 {
		add("experience.no_challenge",
			fmt.Sprintf("win rate %.1f%% over %d runs — the game presents no challenge at its declared pacing",
				100*p.WinRate, p.Runs),
			p.WinRate, 0.98)
	}
	if firstRewardTargetSec > 0 && p.FirstRewardMedianMs > 2*firstRewardTargetSec*1000 {
		add("experience.slow_first_reward",
			fmt.Sprintf("median first reward %.1fs vs the declared %ds target — the opening feels empty",
				float64(p.FirstRewardMedianMs)/1000, firstRewardTargetSec),
			float64(p.FirstRewardMedianMs)/1000, float64(2*firstRewardTargetSec))
	}
	return out
}

// allExperienceIncidents computes incidents for every game with
// telemetry, using each game's declared first-reward target.
func (s *Server) allExperienceIncidents() []experienceIncident {
	summaries := s.perfSummaries()
	out := []experienceIncident{}
	for id, p := range summaries {
		target := 0
		if game, ok := s.store.GetGame(id); ok && game.GameIR != nil && game.GameIR.Experience != nil {
			target = game.GameIR.Experience.Pacing.FirstRewardWithinSec
		}
		out = append(out, deriveExperienceIncidents(id, p, target)...)
	}
	return out
}
