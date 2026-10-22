package validator

// MergePlaytest folds browser-playtest measurements into a static
// ValidationReport. The static validator reports every harness-gated check
// as skipped; the playtest worker (internal/playtest) measures the ones it
// can in a real browser and this merge replaces those skip entries with the
// measured results, then recomputes the verdict with the same decide() the
// static pass used.
//
// Honesty rules (fused detect-and-fix design):
//   - A check the worker did not measure keeps its static status (usually
//     skip with its reason). Unmeasured never becomes pass.
//   - A worker failure merges nothing; the caller surfaces the error.
//   - Measured statuses come from the worker verbatim (pass/fail/skip with
//     evidence); the catalog still owns severity and repairability.

// PlaytestCheck is one measured check result from the playtest worker.
type PlaytestCheck struct {
	CheckID  string `json:"checkId"`
	Status   string `json:"status"` // StatusPass | StatusFail | StatusSkip
	Evidence string `json:"evidence,omitempty"`
	Actual   any    `json:"actual,omitempty"`
}

// PlaytestOutcome is the worker's result for one game.
type PlaytestOutcome struct {
	GameID      string          `json:"gameId"`
	Checks      []PlaytestCheck `json:"checks"`
	WorkerError string          `json:"workerError,omitempty"`
}

// MergePlaytest applies the outcome to the report in place. It returns
// false (and changes nothing) when the worker errored or measured nothing.
func MergePlaytest(report *ValidationReport, outcome PlaytestOutcome) bool {
	if report == nil || outcome.WorkerError != "" || len(outcome.Checks) == 0 {
		return false
	}
	byID := make(map[string]int, len(report.Checks))
	for i, c := range report.Checks {
		byID[c.CheckID] = i
	}
	merged := false
	for _, pc := range outcome.Checks {
		if _, known := checkIndex[pc.CheckID]; !known {
			continue // not a catalogued check; never invent report entries
		}
		status := pc.Status
		if status != StatusPass && status != StatusFail && status != StatusSkip {
			continue
		}
		rebuilt := mk(pc.CheckID, status, nil, pc.Actual, pc.Evidence, "playtest", "")
		if idx, ok := byID[pc.CheckID]; ok {
			report.Checks[idx] = rebuilt
		} else {
			byID[pc.CheckID] = len(report.Checks)
			report.Checks = append(report.Checks, rebuilt)
		}
		merged = true
	}
	if !merged {
		return false
	}
	report.Verdict, report.Repair = decide(report.Checks, report.Repair.Attempt, report.Repair.MaxAttempts)
	report.Summary = summarize(report)
	return true
}
