package validator

import (
	"path/filepath"
	"testing"

	"doodlequest/internal/gamespec"
)

func validateBobo(t *testing.T) *ValidationReport {
	t.Helper()
	di, err := gamespec.LoadRequiredInputs(filepath.Join("testdata", "bobos-rainbow-design-input.json"))
	if err != nil {
		t.Fatalf("load design input: %v", err)
	}
	report, err := Validate(ValidateInput{
		GameID:      "13ui1k",
		GameDir:     filepath.Join("..", "..", "inventory", "13ui1k"),
		DesignInput: di,
		Attempt:     0,
		MaxAttempts: DefaultMaxAttempts,
	})
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	return report
}

func checkByID(r *ValidationReport, id string) (Check, bool) {
	for _, c := range r.Checks {
		if c.CheckID == id {
			return c, true
		}
	}
	return Check{}, false
}

// Measured results replace harness skips; unmeasured checks stay skipped.
func TestMergePlaytestReplacesSkips(t *testing.T) {
	report := validateBobo(t)
	if c, _ := checkByID(report, "simulation.win_reachable"); c.Status != StatusSkip {
		t.Fatalf("precondition: win_reachable should start skipped, got %s", c.Status)
	}
	ok := MergePlaytest(report, PlaytestOutcome{
		GameID: "13ui1k",
		Checks: []PlaytestCheck{
			{CheckID: "simulation.win_reachable", Status: StatusPass, Evidence: "bot observed the win overlay"},
			{CheckID: "simulation.no_softlock", Status: StatusPass, Evidence: "state changed during play"},
			{CheckID: "playtest.boot_clean", Status: StatusPass, Evidence: "booted, 0 errors"},
			{CheckID: "playtest.render_nonblank", Status: StatusPass, Evidence: "census shows entities"},
			{CheckID: "budgets.cold_start", Status: StatusPass, Evidence: "812ms"},
		},
	})
	if !ok {
		t.Fatal("merge reported no change")
	}
	for _, id := range []string{"simulation.win_reachable", "simulation.no_softlock", "playtest.boot_clean", "playtest.render_nonblank", "budgets.cold_start"} {
		c, found := checkByID(report, id)
		if !found {
			t.Errorf("check %s missing after merge", id)
			continue
		}
		if c.Status != StatusPass {
			t.Errorf("check %s: want pass, got %s", id, c.Status)
		}
	}
	// Unmeasured harness checks must remain skip — never upgraded silently.
	if c, _ := checkByID(report, "simulation.memory_bound"); c.Status != StatusSkip {
		t.Errorf("memory_bound: want skip, got %s", c.Status)
	}
	if c, _ := checkByID(report, "hook.restart_terms"); c.Status != StatusSkip {
		t.Errorf("restart_terms: want skip, got %s", c.Status)
	}
}

// A measured boot failure is blocking and flips the verdict to fail.
func TestMergePlaytestBootFailureFailsVerdict(t *testing.T) {
	report := validateBobo(t)
	MergePlaytest(report, PlaytestOutcome{
		GameID: "13ui1k",
		Checks: []PlaytestCheck{
			{CheckID: "playtest.boot_clean", Status: StatusFail, Evidence: "TypeError at boot"},
		},
	})
	if report.Verdict != VerdictFail {
		t.Fatalf("verdict: want fail, got %s", report.Verdict)
	}
	if report.Repair.NextAction != ActionHumanReview {
		t.Fatalf("next action: want human_review (boot failures are not auto-repairable), got %s", report.Repair.NextAction)
	}
}

// A worker failure merges nothing.
func TestMergePlaytestWorkerErrorNoOp(t *testing.T) {
	report := validateBobo(t)
	before := report.Summary
	if MergePlaytest(report, PlaytestOutcome{GameID: "13ui1k", WorkerError: "browser missing"}) {
		t.Fatal("merge should report no change on worker error")
	}
	if report.Summary != before {
		t.Fatal("report changed despite worker error")
	}
}
