// Package validator implements the step-4 Game IR validation gate.
//
// It checks a game's design input (GameDesignInput v1), its packaged assets,
// and its declared budgets, then emits a machine-readable ValidationReport.
// The auto-repair loop consumes the report: every failing check carries a
// concrete suggestion phrased as an edit instruction, and checks the loop
// cannot fix itself are flagged for human review instead of being silently
// weakened.
//
// Checks the validator can measure statically run now. Checks that need the
// bot harness (hook timings, simulation) or the headless executor are
// reported as skipped with an explicit reason — never as passes.
package validator

import "time"

// Report schema version.
const ReportSchemaVersion = "1.0"

// ValidatorVersion pins the validator for reproducible reports.
const ValidatorVersion = "1.0.0"

// DefaultMaxAttempts bounds the auto-repair loop.
const DefaultMaxAttempts = 3

// Verdicts.
const (
	VerdictPass            = "pass"
	VerdictFail            = "fail"
	VerdictPassWithWarning = "pass_with_warnings"
)

// Check statuses.
const (
	StatusPass = "pass"
	StatusFail = "fail"
	StatusWarn = "warn"
	StatusSkip = "skip"
)

// Severities: blocking fails the publish gate; warning ships with a note.
const (
	SeverityBlocking = "blocking"
	SeverityWarning  = "warning"
	SeverityInfo     = "info"
)

// Check categories.
const (
	CatSchema       = "schema"
	CatAssets       = "assets"
	CatCapabilities = "capabilities"
	CatBudgets      = "budgets"
	CatHook         = "hook"
	CatSimulation   = "simulation"
	CatSafety       = "safety"
	CatAudio        = "audio"
)

// Repair actions: the three-state machine from the ValidationReport contract.
const (
	ActionPublish     = "publish"
	ActionAutoRepair  = "auto_repair"
	ActionHumanReview = "human_review"
)

// Check is one validation result.
type Check struct {
	CheckID        string `json:"checkId"`
	Category       string `json:"category"`
	Status         string `json:"status"`
	Severity       string `json:"severity"`
	Expected       any    `json:"expected,omitempty"`
	Actual         any    `json:"actual,omitempty"`
	Evidence       string `json:"evidence,omitempty"`
	Location       string `json:"location,omitempty"`
	Suggestion     string `json:"suggestion,omitempty"`
	AutoRepairable bool   `json:"autoRepairable"`
}

// RepairPlan bounds the auto-repair loop and names the next step.
type RepairPlan struct {
	Attempt     int      `json:"attempt"`
	MaxAttempts int      `json:"maxAttempts"`
	NextAction  string   `json:"nextAction"`
	BlockedBy   []string `json:"blockedBy,omitempty"`
}

// Subject identifies exactly what was validated.
type Subject struct {
	GameID             string `json:"gameId"`
	DesignInputVersion string `json:"designInputVersion,omitempty"`
	IRVersion          string `json:"irVersion,omitempty"`
	ValidatorVersion   string `json:"validatorVersion"`
}

// ValidationReport is the step-4 contract output.
type ValidationReport struct {
	SchemaVersion string     `json:"schemaVersion"`
	ReportID      string     `json:"reportId"`
	CreatedAt     time.Time  `json:"createdAt"`
	Subject       Subject    `json:"subject"`
	Verdict       string     `json:"verdict"`
	Checks        []Check    `json:"checks"`
	Repair        RepairPlan `json:"repair"`
	Summary       string     `json:"summary"`
	DurationMs    int64      `json:"durationMs"`
}
