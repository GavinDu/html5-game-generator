package validator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"doodlequest/internal/gameir"
)

// quality.contract_matches_ir (scorecard fusion, Phase D): when a
// game carries a quality-rules.json, its generated contract block
// must equal the contract derived from the game's own IR experience
// block, and its prose (core loop / fairness) must not claim
// mechanics the IR contradicts — a judge scoring against a drifted
// contract penalizes a correct game for missing features it never
// had. Mirrors `score-report.mjs --check-contracts`.

type qualityRulesFile struct {
	GameplayContract struct {
		CoreLoop  []string `json:"coreLoop"`
		Fairness  []string `json:"fairness"`
		Generated *struct {
			Archetype      string `json:"archetype"`
			ControlsScheme string `json:"controlsScheme"`
			Objective      *struct {
				Kind   string  `json:"kind"`
				Target float64 `json:"target"`
				Label  string  `json:"label"`
			} `json:"objective"`
			Failure *struct {
				Mode           string `json:"mode"`
				Health         *int   `json:"health"`
				InvulnerableMs *int   `json:"invulnerableMs"`
			} `json:"failure"`
			TargetSessionSec     *int `json:"targetSessionSec"`
			FirstRewardWithinSec *int `json:"firstRewardWithinSec"`
			Milestones           int  `json:"milestones"`
			Scenes               int  `json:"scenes"`
		} `json:"generated"`
	} `json:"gameplayContract"`
}

var (
	missLimitRe = regexp.MustCompile(`miss limit|miss-limit|limited misses`)
	roundsRe    = regexp.MustCompile(`\brounds?\b`)
)

func checkQualityContract(gameDir string, doc *gameir.Document, irErr error) Check {
	raw, err := os.ReadFile(filepath.Join(gameDir, "quality-rules.json"))
	if err != nil {
		return Check{
			CheckID: "quality.contract_matches_ir", Severity: SeverityWarning,
			Status: StatusSkip, Evidence: "no quality-rules.json for this game",
		}
	}
	if irErr != nil || doc == nil || doc.Experience == nil {
		return Check{
			CheckID: "quality.contract_matches_ir", Severity: SeverityWarning,
			Status: StatusSkip, Evidence: "no usable game-ir.json to derive the contract from",
		}
	}
	var rules qualityRulesFile
	if err := json.Unmarshal(raw, &rules); err != nil {
		return Check{
			CheckID: "quality.contract_matches_ir", Severity: SeverityWarning,
			Status: StatusFail, Evidence: fmt.Sprintf("quality-rules.json does not parse: %v", err),
		}
	}
	exp := doc.Experience
	var problems []string
	gen := rules.GameplayContract.Generated
	if gen == nil {
		problems = append(problems, "gameplayContract.generated is missing (run score-report --write-contracts)")
	} else {
		diff := func(field string, a, b any) {
			if fmt.Sprint(a) != fmt.Sprint(b) {
				problems = append(problems, fmt.Sprintf("generated.%s = %v but the IR says %v", field, a, b))
			}
		}
		diff("archetype", strOr(gen.Archetype), exp.Archetype)
		diff("controlsScheme", strOr(gen.ControlsScheme), exp.Controls.Scheme)
		if gen.Objective != nil {
			diff("objective.kind", gen.Objective.Kind, exp.Objective.Kind)
			diff("objective.target", gen.Objective.Target, float64(exp.Objective.Target))
			diff("objective.label", gen.Objective.Label, exp.Objective.Label)
		}
		if gen.Failure != nil {
			diff("failure.mode", gen.Failure.Mode, exp.Failure.Mode)
		}
		diff("targetSessionSec", intOr(gen.TargetSessionSec), exp.Pacing.TargetSessionSec)
		diff("firstRewardWithinSec", intOr(gen.FirstRewardWithinSec), exp.Pacing.FirstRewardWithinSec)
		diff("milestones", gen.Milestones, len(exp.Milestones))
		diff("scenes", gen.Scenes, len(doc.Scenes))
	}
	prose := strings.ToLower(strings.Join(append(
		append([]string{}, rules.GameplayContract.CoreLoop...),
		rules.GameplayContract.Fairness...), " "))
	if missLimitRe.MatchString(prose) && (exp.Failure.Mode == "gentle" || exp.Failure.Mode == "none") {
		problems = append(problems, fmt.Sprintf("core-loop prose claims a miss limit but the IR failure mode is %q", exp.Failure.Mode))
	}
	if roundsRe.MatchString(prose) && len(doc.Scenes) < 2 {
		problems = append(problems, "core-loop prose mentions rounds but the IR has a single scene and no round structure")
	}
	if len(problems) > 0 {
		return Check{
			CheckID: "quality.contract_matches_ir", Severity: SeverityWarning,
			Status: StatusFail, Evidence: strings.Join(problems, "; "),
		}
	}
	return Check{
		CheckID: "quality.contract_matches_ir", Severity: SeverityWarning,
		Status: StatusPass, Evidence: "quality contract generated block and prose agree with the IR experience block",
	}
}

func strOr(s string) string { return s }
func intOr(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}
