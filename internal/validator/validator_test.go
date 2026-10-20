package validator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"doodlequest/internal/gamespec"
)

// TestValidateBobosRainbow runs the full validation gate end to end on a
// shipped game: design input -> checks -> ValidationReport -> verdict.
func TestValidateBobosRainbow(t *testing.T) {
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

	byID := make(map[string]Check, len(report.Checks))
	for _, c := range report.Checks {
		byID[c.CheckID] = c
	}
	must := func(id, wantStatus string) {
		t.Helper()
		c, ok := byID[id]
		if !ok {
			t.Fatalf("check %s missing from report", id)
		}
		if c.Status != wantStatus {
			t.Errorf("check %s: want %s, got %s (%s)", id, wantStatus, c.Status, c.Evidence)
		}
	}

	// Static gates that must pass on the real game.
	must("schema.design_input_valid", StatusPass)
	must("schema.spec_version_known", StatusPass)
	must("assets.all_resolved", StatusPass)
	must("assets.files_present", StatusPass)
	must("budgets.session_seconds", StatusPass)
	must("budgets.package_mb", StatusPass)
	must("safety.kid_safe", StatusPass)
	must("audio.complete", StatusPass)
	must("design.rights_declared", StatusPass)

	// Honest findings on the sample: library music isn't vendored, and
	// upload screening needs a human.
	must("assets.music_resolved", StatusWarn)
	must("safety.content_screen", StatusWarn)

	// Harness-gated checks report skip, never pass.
	for _, id := range []string{
		"hook.time_to_input", "hook.first_reward",
		"simulation.win_reachable", "simulation.no_softlock",
		"capabilities.fallbacks_complete",
	} {
		must(id, StatusSkip)
	}

	// Two warnings, no blocking failures: ships with a note.
	if report.Verdict != VerdictPassWithWarning {
		t.Errorf("verdict: want %s, got %s", VerdictPassWithWarning, report.Verdict)
	}
	if report.Repair.NextAction != ActionPublish {
		t.Errorf("nextAction: want %s, got %s", ActionPublish, report.Repair.NextAction)
	}
	if report.Repair.Attempt != 0 || report.Repair.MaxAttempts != DefaultMaxAttempts {
		t.Errorf("repair budget not carried through: %+v", report.Repair)
	}

	// The report must round-trip as JSON (it is the API contract).
	raw, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("marshal report: %v", err)
	}
	var back ValidationReport
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal report: %v", err)
	}
	if back.Verdict != report.Verdict || len(back.Checks) != len(report.Checks) {
		t.Errorf("report did not round-trip")
	}
}

// TestValidateRejectsBadInput proves the gate actually blocks: a design
// input violating budget maximums must fail blocking checks.
func TestValidateRejectsBadInput(t *testing.T) {
	di, err := gamespec.LoadRequiredInputs(filepath.Join("testdata", "bobos-rainbow-design-input.json"))
	if err != nil {
		t.Fatalf("load design input: %v", err)
	}
	bad := *di
	bad.Constraints.SessionTargetSec = 600 // violates the <= 90 maximum
	report, err := Validate(ValidateInput{
		GameID:      "13ui1k",
		GameDir:     filepath.Join("..", "..", "inventory", "13ui1k"),
		DesignInput: &bad,
		Attempt:     0,
		MaxAttempts: DefaultMaxAttempts,
	})
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if report.Verdict != VerdictFail {
		t.Errorf("verdict: want %s, got %s", VerdictFail, report.Verdict)
	}
	if report.Repair.NextAction != ActionAutoRepair {
		t.Errorf("nextAction: want %s, got %s (blocking failure is auto-repairable, attempt 0/3)",
			ActionAutoRepair, report.Repair.NextAction)
	}
	found := false
	for _, c := range report.Checks {
		if c.CheckID == "schema.design_input_valid" && c.Status == StatusFail {
			found = true
		}
	}
	if !found {
		t.Errorf("expected schema.design_input_valid to fail on the bad input")
	}
}

// TestRightsDeclared covers the Stage 0 rights gate: declared known rights
// pass, undeclared rights warn (legacy inputs), explicitly unknown rights
// or unresolved consent fail.
func TestRightsDeclared(t *testing.T) {
	mk := func(sources ...gamespec.UploadSource) *gamespec.RequiredInputs {
		return &gamespec.RequiredInputs{Sources: sources}
	}
	cases := []struct {
		name string
		di   *gamespec.RequiredInputs
		want string
	}{
		{"no sources", mk(), StatusPass},
		{"user created", mk(gamespec.UploadSource{UploadID: "u1", Kind: "image",
			RightsStatus: "user_created", ConsentStatus: "not_applicable"}), StatusPass},
		{"undeclared warns", mk(gamespec.UploadSource{UploadID: "u1", Kind: "image"}), StatusWarn},
		{"unknown rights fail", mk(gamespec.UploadSource{UploadID: "u1", Kind: "image",
			RightsStatus: "permission_unknown"}), StatusFail},
		{"pending consent fails", mk(gamespec.UploadSource{UploadID: "u1", Kind: "image",
			RightsStatus: "owned", ConsentStatus: "pending"}), StatusFail},
	}
	for _, tc := range cases {
		if got := checkRightsDeclared(tc.di); got.Status != tc.want {
			t.Errorf("%s: want %s, got %s (%s)", tc.name, tc.want, got.Status, got.Evidence)
		}
	}
}

// TestMain guards the suite: the sample game directory must exist.
func TestMain(m *testing.M) {
	if _, err := os.Stat(filepath.Join("..", "..", "inventory", "13ui1k", "manifest.json")); err != nil {
		panic("validator tests require inventory/13ui1k from the repo root")
	}
	os.Exit(m.Run())
}
