// Package playtest runs the browser playtest worker (tools/playtest/
// probe.mjs) against a live server and returns its measurements in the
// validator's vocabulary, ready for validator.MergePlaytest.
//
// The worker is the Phase-1 harness of the fused detect-and-fix flow: it
// boots each game in a real browser, reads the player's ?probe=1 verdict,
// and plays the game with bot input. Nothing in here fakes a result — a
// worker that cannot run is an error, and checks the bot could not measure
// arrive as skip with their reason.
package playtest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"doodlequest/internal/validator"
)

// Config controls one worker run.
type Config struct {
	// WorkerCmd is the command that runs the worker; the runner appends
	// --base/--games/--browser flags. Default: node tools/playtest/probe.mjs.
	WorkerCmd []string
	BaseURL   string
	Browser   string // chromium (default) | webkit
	ShotsDir  string // when set, the worker writes screenshots + its report there
	Timeout   time.Duration
}

// GameResult is the worker's per-game report.
type GameResult struct {
	GameID  string                    `json:"gameId"`
	Verdict string                    `json:"verdict"` // PASS | FAIL (gate checks only)
	Checks  []validator.PlaytestCheck `json:"checks"`
	Probe   json.RawMessage           `json:"probe,omitempty"`
}

// Report is one full worker run.
type Report struct {
	Base    string       `json:"base"`
	Browser string       `json:"browser"`
	Games   []GameResult `json:"games"`
}

// Failed reports whether any game failed the gate checks.
func (r *Report) Failed() bool {
	for _, g := range r.Games {
		if g.Verdict == "FAIL" {
			return true
		}
	}
	return false
}

// OutcomeFor maps one game's result onto the validator merge input.
func (r *Report) OutcomeFor(gameID string) (validator.PlaytestOutcome, bool) {
	for _, g := range r.Games {
		if g.GameID == gameID {
			return validator.PlaytestOutcome{GameID: g.GameID, Checks: g.Checks}, true
		}
	}
	return validator.PlaytestOutcome{}, false
}

// validateReport rejects syntactically valid but incomplete worker output.
// An empty or ambiguous report must never be interpreted as a successful gate.
func validateReport(report *Report) error {
	if strings.TrimSpace(report.Base) == "" {
		return fmt.Errorf("missing base URL")
	}
	if strings.TrimSpace(report.Browser) == "" {
		return fmt.Errorf("missing browser")
	}
	if len(report.Games) == 0 {
		return fmt.Errorf("report contains no games")
	}
	seen := make(map[string]struct{}, len(report.Games))
	for i, game := range report.Games {
		id := strings.TrimSpace(game.GameID)
		if id == "" {
			return fmt.Errorf("game %d has no gameId", i)
		}
		if _, exists := seen[id]; exists {
			return fmt.Errorf("duplicate gameId %q", id)
		}
		seen[id] = struct{}{}
		if game.Verdict != "PASS" && game.Verdict != "FAIL" {
			return fmt.Errorf("game %q has invalid verdict %q", id, game.Verdict)
		}
		if len(game.Checks) == 0 {
			return fmt.Errorf("game %q has no measured checks", id)
		}
		seenChecks := make(map[string]struct{}, len(game.Checks))
		for checkIndex, check := range game.Checks {
			checkID := strings.TrimSpace(check.CheckID)
			if checkID == "" {
				return fmt.Errorf("game %q check %d has no checkId", id, checkIndex)
			}
			if _, exists := seenChecks[checkID]; exists {
				return fmt.Errorf("game %q has duplicate checkId %q", id, checkID)
			}
			seenChecks[checkID] = struct{}{}
			switch check.Status {
			case validator.StatusPass, validator.StatusFail, validator.StatusSkip:
			default:
				return fmt.Errorf("game %q check %q has invalid status %q", id, check.CheckID, check.Status)
			}
		}
		// A PASS verdict is publishable only when every hard delivery gate
		// was measured and passed. Missing or skipped gates are inconclusive,
		// not evidence of success. FAIL reports may be incomplete because a
		// boot failure can prevent later checks from running.
		if game.Verdict == "PASS" {
			gateStatuses := map[string]string{}
			for _, check := range game.Checks {
				switch check.CheckID {
				case "playtest.boot_clean", "playtest.render_nonblank", "simulation.no_softlock":
					gateStatuses[check.CheckID] = check.Status
				case "shell.start_card", "shell.restart_in_place":
					if check.Status == validator.StatusFail {
						return fmt.Errorf("game %q has PASS verdict but exit-gate %q failed", id, check.CheckID)
					}
				}
			}
			for _, gate := range []string{"playtest.boot_clean", "playtest.render_nonblank", "simulation.no_softlock"} {
				status, measured := gateStatuses[gate]
				if !measured || status != validator.StatusPass {
					return fmt.Errorf("game %q has PASS verdict but hard gate %q is missing or not passed", id, gate)
				}
			}
		}
	}
	return nil
}

// Run executes the worker and parses its JSON report. The worker exits 1
// when a game fails the gate — that is still a successful run with a real
// report. Exit code 2 (worker failure) or unparseable output is an error.
func Run(ctx context.Context, cfg Config, gameIDs []string) (*Report, error) {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return nil, fmt.Errorf("playtest: BaseURL is required")
	}
	// The worker accepts a comma-separated game list. Normalize identifiers
	// before building that argument so whitespace cannot cause false missing
	// results, and reject ambiguous identifiers before starting a subprocess.
	if len(gameIDs) > 0 {
		normalized := make([]string, 0, len(gameIDs))
		seen := make(map[string]struct{}, len(gameIDs))
		for i, rawID := range gameIDs {
			id := strings.TrimSpace(rawID)
			if id == "" {
				return nil, fmt.Errorf("playtest: requested game %d has an empty ID", i)
			}
			if strings.Contains(id, ",") {
				return nil, fmt.Errorf("playtest: requested game ID %q contains reserved separator \",\"", id)
			}
			if _, exists := seen[id]; exists {
				return nil, fmt.Errorf("playtest: duplicate requested game ID %q", id)
			}
			seen[id] = struct{}{}
			normalized = append(normalized, id)
		}
		gameIDs = normalized
	}
	cmd := cfg.WorkerCmd
	if len(cmd) == 0 {
		cmd = []string{"node", "tools/playtest/probe.mjs"}
	}
	if strings.TrimSpace(cmd[0]) == "" {
		return nil, fmt.Errorf("playtest: worker command is empty")
	}
	args := append([]string{}, cmd[1:]...)
	args = append(args, "--base", cfg.BaseURL)
	if cfg.Browser != "" {
		args = append(args, "--browser", cfg.Browser)
	}
	if cfg.ShotsDir != "" {
		args = append(args, "--shots", cfg.ShotsDir)
	}
	if len(gameIDs) > 0 {
		joined := gameIDs[0]
		for _, id := range gameIDs[1:] {
			joined += "," + id
		}
		args = append(args, "--games", joined)
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	c := exec.CommandContext(runCtx, cmd[0], args...)
	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr
	err := c.Run()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 && stdout.Len() > 0 {
			// Gate failure with a valid report: fall through and parse.
			err = nil
		} else {
			return nil, fmt.Errorf("playtest: worker failed: %v: %s", err, stderr.String())
		}
	}
	var report Report
	if jerr := json.Unmarshal(stdout.Bytes(), &report); jerr != nil {
		return nil, fmt.Errorf("playtest: worker output is not a report: %v", jerr)
	}
	if err := validateReport(&report); err != nil {
		return nil, fmt.Errorf("playtest: invalid worker report: %w", err)
	}
	if cfg.Browser != "" && report.Browser != cfg.Browser {
		return nil, fmt.Errorf("playtest: worker reported browser %q, want %q", report.Browser, cfg.Browser)
	}
	// A report is not a publish gate for the requested batch unless every
	// requested game has an explicit result. Otherwise a worker that silently
	// drops a game could make an incomplete batch look successful.
	if len(gameIDs) > 0 {
		requested := make(map[string]struct{}, len(gameIDs))
		for _, gameID := range gameIDs {
			requested[gameID] = struct{}{}
		}
		reported := make(map[string]struct{}, len(report.Games))
		for _, game := range report.Games {
			if _, ok := requested[game.GameID]; !ok {
				return nil, fmt.Errorf("playtest: report contains unrequested game %q", game.GameID)
			}
			reported[game.GameID] = struct{}{}
		}
		for _, gameID := range gameIDs {
			if _, ok := reported[gameID]; !ok {
				return nil, fmt.Errorf("playtest: report is missing requested game %q", gameID)
			}
		}
	}
	return &report, nil
}
