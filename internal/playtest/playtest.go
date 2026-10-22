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

// Run executes the worker and parses its JSON report. The worker exits 1
// when a game fails the gate — that is still a successful run with a real
// report. Exit code 2 (worker failure) or unparseable output is an error.
func Run(ctx context.Context, cfg Config, gameIDs []string) (*Report, error) {
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("playtest: BaseURL is required")
	}
	cmd := cfg.WorkerCmd
	if len(cmd) == 0 {
		cmd = []string{"node", "tools/playtest/probe.mjs"}
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
	return &report, nil
}
