package validator

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"doodlequest/internal/gamespec"
)

// ValidateInput is everything the validator needs for one run.
type ValidateInput struct {
	GameID      string // inventory dir name, e.g. "13ui1k"
	GameDir     string // game directory on disk
	DesignInput *gamespec.RequiredInputs
	Attempt     int // repair attempt number, 0-based
	MaxAttempts int // 0 means DefaultMaxAttempts
}

// manifestView is the slice of manifest.json the validator reads.
type manifestView struct {
	Entry    string `json:"entry"`
	Elements []struct {
		ID string `json:"id"`
	} `json:"elements"`
	Sprites []struct {
		ID   string `json:"id"`
		File string `json:"file,omitempty"`
	} `json:"sprites"`
	Music struct {
		Src    string `json:"src"`
		Source string `json:"source"`
	} `json:"music"`
}

// Validate runs the check catalog against one game and returns the report.
// Checks the validator cannot measure yet (bot hook timings, headless
// simulation, capability ABI) are reported as skipped with an explicit
// reason — never as passes.
func Validate(in ValidateInput) (*ValidationReport, error) {
	start := time.Now()
	if in.DesignInput == nil {
		return nil, fmt.Errorf("validator: design input is required")
	}
	if in.MaxAttempts <= 0 {
		in.MaxAttempts = DefaultMaxAttempts
	}
	man, manErr := readManifest(in.GameDir)
	irDoc, irErr := readGameIR(in.GameDir)

	checks := []Check{
		checkDesignInputValid(in.DesignInput),
		checkSpecVersionKnown(in.DesignInput),
		checkAssetsResolved(in.DesignInput, man, manErr),
		checkFilesPresent(in.GameDir, man, manErr),
		checkMusicResolved(in.GameDir, man, manErr),
		checkCapabilitiesFallbacks(),
		checkBudgetSession(in.DesignInput),
		checkBudgetPackage(in.GameDir),
		checkBudgetColdStart(),
		checkHookTimeToInput(),
		checkHookFirstReward(),
		checkHookNoMandatoryTutorial(),
		checkHookRestartTerms(),
		checkSimWinReachable(),
		checkSimNoSoftlock(),
		checkSimLevel1Clearable(),
		checkSimDifficultyRamp(),
		checkSimMemoryBound(),
		checkKidSafe(in.GameDir, man, manErr),
		checkContentScreen(),
		checkAudioComplete(in.DesignInput),
		checkPremiseDeclared(in.DesignInput),
		checkTwistDeclared(in.DesignInput),
		checkMechanicDiet(in.DesignInput),
		checkRightsDeclared(in.DesignInput),
	}
	checks = append(checks, experienceChecks(irDoc, irErr)...)
	checks = append(checks, checkQualityContract(in.GameDir, irDoc, irErr))

	report := &ValidationReport{
		SchemaVersion: ReportSchemaVersion,
		ReportID:      newReportID(),
		CreatedAt:     time.Now().UTC(),
		Subject: Subject{
			GameID:             in.GameID,
			DesignInputVersion: in.DesignInput.SchemaVersion,
			ValidatorVersion:   ValidatorVersion,
		},
		Checks:     checks,
		DurationMs: time.Since(start).Milliseconds(),
	}
	report.Verdict, report.Repair = decide(checks, in.Attempt, in.MaxAttempts)
	report.Summary = summarize(report)
	return report, nil
}

func newReportID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("rpt-%d", time.Now().UnixNano())
	}
	return "rpt-" + hex.EncodeToString(b[:])
}

func readManifest(gameDir string) (*manifestView, error) {
	raw, err := os.ReadFile(filepath.Join(gameDir, "manifest.json"))
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	var m manifestView
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	return &m, nil
}

// mk builds a Check from its catalog entry.
func mk(id, status string, expected, actual any, evidence, location, suggestion string) Check {
	def := catalog[checkIndex[id]]
	return Check{
		CheckID: id, Category: def.category, Status: status,
		Severity: def.severity, Expected: expected, Actual: actual,
		Evidence: evidence, Location: location, Suggestion: suggestion,
		AutoRepairable: def.autoRepairable,
	}
}

func skipped(id, reason string) Check {
	return mk(id, StatusSkip, nil, nil, reason, "", "")
}

// --- schema checks ---

func checkDesignInputValid(di *gamespec.RequiredInputs) Check {
	if err := di.Validate(); err != nil {
		return mk("schema.design_input_valid", StatusFail,
			"valid GameDesignInput v1", "invalid: "+err.Error(),
			"gamespec.RequiredInputs.Validate()", "$",
			"Fix the reported field to satisfy required_inputs.schema.json; do not weaken the schema.")
	}
	return mk("schema.design_input_valid", StatusPass,
		"valid GameDesignInput v1", "valid",
		"gamespec.RequiredInputs.Validate()", "$", "")
}

func checkSpecVersionKnown(di *gamespec.RequiredInputs) Check {
	if di.SchemaVersion != gamespec.RequiredInputsVersion {
		return mk("schema.spec_version_known", StatusFail,
			gamespec.RequiredInputsVersion, di.SchemaVersion,
			"schemaVersion field", "$.schemaVersion",
			fmt.Sprintf("Set schemaVersion to %q or upgrade the validator.", gamespec.RequiredInputsVersion))
	}
	return mk("schema.spec_version_known", StatusPass,
		gamespec.RequiredInputsVersion, di.SchemaVersion,
		"schemaVersion field", "$.schemaVersion", "")
}

// --- asset checks ---

func designRefs(di *gamespec.RequiredInputs) []string {
	// Only packaged-asset references resolve against the manifest.
	// sources[] entries are provenance (raw uploads), not packaged assets.
	var refs []string
	refs = append(refs, di.Characters.Player.DesignRef)
	for _, n := range di.Characters.NPCs {
		refs = append(refs, n.DesignRef)
	}
	if u := di.Visual.Background.SourceUploadID; u != "" {
		refs = append(refs, u)
	}
	return refs
}

func checkAssetsResolved(di *gamespec.RequiredInputs, man *manifestView, manErr error) Check {
	const id = "assets.all_resolved"
	if manErr != nil {
		return mk(id, StatusFail, "all designRefs resolve", "manifest unreadable: "+manErr.Error(),
			"manifest.json", "$", "Restore a valid manifest.json in the game directory.")
	}
	known := map[string]bool{"generated": true}
	for _, e := range man.Elements {
		known[e.ID] = true
	}
	for _, s := range man.Sprites {
		known[s.ID] = true
	}
	var missing []string
	for _, r := range designRefs(di) {
		if r == "" || known[r] {
			continue
		}
		missing = append(missing, r)
	}
	if len(missing) > 0 {
		return mk(id, StatusFail, "all designRefs resolve",
			"unresolved: "+strings.Join(missing, ", "),
			"manifest elements[] and sprites[]", "$.characters / $.sources",
			"Point the designRefs at real manifest element/sprite ids, or add the missing assets to the manifest.")
	}
	return mk(id, StatusPass, "all designRefs resolve", "all resolved",
		"manifest elements[] and sprites[]", "$.characters / $.sources", "")
}

func checkFilesPresent(gameDir string, man *manifestView, manErr error) Check {
	const id = "assets.files_present"
	if manErr != nil {
		return mk(id, StatusFail, "manifest readable", manErr.Error(),
			"manifest.json", "$", "Restore a valid manifest.json.")
	}
	var missing []string
	check := func(rel string) {
		if rel == "" {
			return
		}
		if _, err := os.Stat(filepath.Join(gameDir, rel)); err != nil {
			missing = append(missing, rel)
		}
	}
	check(man.Entry)
	for _, s := range man.Sprites {
		check(s.File)
	}
	if len(missing) > 0 {
		return mk(id, StatusFail, "entry + sprite files on disk",
			"missing: "+strings.Join(missing, ", "),
			"stat of game directory", "$",
			"Restore the missing files or remove their manifest entries; never ship a manifest that references absent files.")
	}
	return mk(id, StatusPass, "entry + sprite files on disk", "all present",
		"stat of game directory", "$", "")
}

func checkMusicResolved(gameDir string, man *manifestView, manErr error) Check {
	const id = "assets.music_resolved"
	if manErr != nil {
		return skipped(id, "manifest unreadable; cannot resolve music")
	}
	if man.Music.Src == "" {
		return mk(id, StatusWarn, "music resolves or is absent by design", "no music declared",
			"manifest music.src", "$.audio", "Declare background music (P10) or confirm silence is intentional.")
	}
	if man.Music.Source == "library" {
		if _, err := os.Stat(filepath.Join(gameDir, man.Music.Src)); err == nil {
			return mk(id, StatusPass, "music resolves", "on-disk: "+man.Music.Src,
				"stat of game directory", "$.audio", "")
		}
		return mk(id, StatusWarn, "music resolves", "library source, not on disk: "+man.Music.Src,
			"manifest music.source=library; file absent from game dir",
			"$.audio",
			"Either vendor the track into the game directory or confirm the build pipeline resolves library music before packaging.")
	}
	if _, err := os.Stat(filepath.Join(gameDir, man.Music.Src)); err != nil {
		return mk(id, StatusFail, "music file on disk", "missing: "+man.Music.Src,
			"stat of game directory", "$.audio",
			"Add the music file or fix manifest music.src.")
	}
	return mk(id, StatusPass, "music resolves", "on-disk: "+man.Music.Src,
		"stat of game directory", "$.audio", "")
}

// --- capability, budget, hook, simulation: mostly harness-gated ---

func checkCapabilitiesFallbacks() Check {
	return skipped("capabilities.fallbacks_complete",
		"capability ABI not yet in RuntimeSpec v1; no REQUIRED declarations to check")
}

func checkBudgetSession(di *gamespec.RequiredInputs) Check {
	const id = "budgets.session_seconds"
	v := di.Constraints.SessionTargetSec
	if v > 90 {
		return mk(id, StatusFail, "<= 90", v, "constraints.sessionTargetSec", "$.constraints",
			"Shorten the target session or split the game into <90s runs.")
	}
	return mk(id, StatusPass, "<= 90", v, "constraints.sessionTargetSec", "$.constraints", "")
}

func dirSizeBytes(dir string) (int64, error) {
	var total int64
	err := filepath.Walk(dir, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	return total, err
}

func checkBudgetPackage(gameDir string) Check {
	const id = "budgets.package_mb"
	const maxBytes = 25 * 1024 * 1024
	size, err := dirSizeBytes(gameDir)
	if err != nil {
		return mk(id, StatusFail, "<= 25 MB", "unmeasurable: "+err.Error(),
			"game directory walk", "$.constraints", "Fix directory permissions so the package can be measured.")
	}
	mb := float64(size) / (1024 * 1024)
	if size > maxBytes {
		return mk(id, StatusFail, "<= 25 MB", fmt.Sprintf("%.1f MB", mb),
			"measured game directory size", "$.constraints",
			"Downscale or lazy-load assets; the 25 MB budget is a hard publish gate.")
	}
	return mk(id, StatusPass, "<= 25 MB", fmt.Sprintf("%.1f MB", mb),
		"measured game directory size", "$.constraints", "")
}

func checkBudgetColdStart() Check {
	return skipped("budgets.cold_start", "requires load-timing harness; cannot measure statically")
}

func checkHookTimeToInput() Check {
	return skipped("hook.time_to_input", "requires bot harness: input-acceptance timing")
}
func checkHookFirstReward() Check {
	return skipped("hook.first_reward", "requires bot harness: reward-event timing")
}
func checkHookNoMandatoryTutorial() Check {
	return skipped("hook.no_mandatory_tutorial", "requires bot harness: pre-input modal scan")
}
func checkHookRestartTerms() Check {
	return skipped("hook.restart_terms", "requires bot harness: death-to-control timing")
}
func checkSimWinReachable() Check {
	return skipped("simulation.win_reachable", "requires headless executor harness")
}
func checkSimNoSoftlock() Check {
	return skipped("simulation.no_softlock", "requires headless executor harness")
}
func checkSimLevel1Clearable() Check {
	return skipped("simulation.level1_clearable", "requires headless executor harness")
}
func checkSimDifficultyRamp() Check {
	return skipped("simulation.difficulty_ramp", "requires headless executor harness")
}
func checkSimMemoryBound() Check {
	return skipped("simulation.memory_bound", "requires headless executor harness")
}

// --- safety checks ---

var externalURLMarkers = []string{"http://", "https://"}

func checkKidSafe(gameDir string, man *manifestView, manErr error) Check {
	const id = "safety.kid_safe"
	if manErr != nil {
		return skipped(id, "manifest unreadable; cannot locate entry script")
	}
	entry := filepath.Join(gameDir, man.Entry)
	raw, err := os.ReadFile(entry)
	if err != nil {
		return mk(id, StatusFail, "entry script readable", err.Error(),
			"stat of entry script", "$", "Restore the entry script.")
	}
	var hits []string
	for i, line := range strings.Split(string(raw), "\n") {
		for _, m := range externalURLMarkers {
			if strings.Contains(line, m) {
				hits = append(hits, fmt.Sprintf("line %d", i+1))
				break
			}
		}
		if len(hits) >= 5 {
			break
		}
	}
	if len(hits) > 0 {
		return mk(id, StatusFail, "no external URLs in game bundle",
			fmt.Sprintf("external URL markers at %s", strings.Join(hits, ", ")),
			"static scan of entry script", "$",
			"Remove external navigation/requests from the game bundle; kid-safe games load zero third-party resources.")
	}
	return mk(id, StatusPass, "no external URLs in game bundle", "none found",
		"static scan of entry script", "$", "")
}

func checkContentScreen() Check {
	return mk("safety.content_screen", StatusWarn, "uploads screened", "not screened",
		"automated upload screening is not implemented; manual review required",
		"$.sources",
		"A human must review uploaded art before publish. This check is never auto-repairable.")
}

// --- design checks (comedy-framework v1.1 generative layer) ---

func checkPremiseDeclared(di *gamespec.RequiredInputs) Check {
	const id = "design.premise_declared"
	p := di.Premise
	switch p.Attitude {
	case "strange", "sucks", "scary", "silly":
	default:
		return mk(id, StatusFail, "attitude in {strange,sucks,scary,silly}", fmt.Sprintf("%q", p.Attitude),
			"design input $.premise", "$.premise",
			"Pick the emotional stance the premise actually takes: strange, sucks, scary, or silly — then write the one-line statement from the source material.")
	}
	if p.Statement == "" {
		return mk(id, StatusFail, "one-line premise statement", "empty",
			"design input $.premise", "$.premise",
			"Write the one-line emotional premise before designing mechanics; a game without a legible attitude is a template with art.")
	}
	return mk(id, StatusPass, "attitude + statement",
		fmt.Sprintf("%s: %.80s", p.Attitude, p.Statement),
		"design input $.premise", "$.premise", "")
}

func checkTwistDeclared(di *gamespec.RequiredInputs) Check {
	const id = "design.twist_declared"
	tw := di.Strategy.Twist
	if tw.Assumption == "" || tw.Reinterpretation == "" {
		return mk(id, StatusFail, "assumption + reinterpretation", "missing",
			"design input $.strategy.twist", "$.strategy.twist",
			"Declare what the player expects (assumption) and what the game actually does (reinterpretation).")
	}
	// A twist that restates the assumption is vacuous — the one semantic
	// check a static validator can honestly make about surprise.
	norm := func(s string) string {
		return strings.ToLower(strings.Join(strings.Fields(s), " "))
	}
	if norm(tw.Assumption) == norm(tw.Reinterpretation) {
		return mk(id, StatusWarn, "reinterpretation differs from assumption", "identical",
			"design input $.strategy.twist", "$.strategy.twist",
			"Rewrite the reinterpretation so it genuinely differs from the player's assumption; a twist that restates the setup surprises no one.")
	}
	return mk(id, StatusPass, "assumption + distinct reinterpretation", "declared",
		"design input $.strategy.twist", "$.strategy.twist", "")
}

func checkMechanicDiet(di *gamespec.RequiredInputs) Check {
	const id = "design.mechanic_diet"
	var unjustified []string
	for _, p := range di.Features.Powerups {
		if p.LoopLink == "" {
			unjustified = append(unjustified, "powerup:"+p.Name)
		}
	}
	for _, m := range di.Features.Modes {
		if m.LoopLink == "" {
			unjustified = append(unjustified, "mode:"+m.Name)
		}
	}
	if len(unjustified) > 0 {
		return mk(id, StatusFail, "every mechanic has a loopLink",
			"unjustified: "+strings.Join(unjustified, ", "),
			"design input $.features", "$.features",
			"Give each mechanic a one-sentence loopLink (how it serves the core loop) or cut it — the mechanic diet.")
	}
	// Heuristic: a sub-90s session cannot carry many distinct mechanics.
	// More than 5 declared powerups+modes is a smell, not a failure.
	n := len(di.Features.Powerups) + len(di.Features.Modes)
	if n > 5 {
		return mk(id, StatusWarn, "<= 5 mechanics for a sub-90s session",
			fmt.Sprintf("%d declared", n),
			"design input $.features", "$.features",
			"Cut or merge mechanics until each one clearly earns its place; filler mechanics dilute the core loop.")
	}
	return mk(id, StatusPass, "all mechanics justified; count fits budget",
		fmt.Sprintf("%d declared", n),
		"design input $.features", "$.features", "")
}

// checkRightsDeclared is the Stage 0 rights gate (questionnaire v2 §B):
// source material may only be published when its rights are known.
// permission_unknown (or unresolved consent) fails; sources that predate
// the rights fields warn so a human reviews them before publication.
func checkRightsDeclared(di *gamespec.RequiredInputs) Check {
	const id = "design.rights_declared"
	if di == nil || len(di.Sources) == 0 {
		return mk(id, StatusPass, "all sources declare known rights",
			"no sources", "design input $.sources", "$.sources", "")
	}
	var unknown, undeclared []string
	for _, s := range di.Sources {
		switch {
		case s.RightsStatus == "permission_unknown":
			unknown = append(unknown, s.UploadID+" (rights permission_unknown)")
		case s.ConsentStatus == "pending" || s.ConsentStatus == "unknown":
			unknown = append(unknown, s.UploadID+" (consent "+s.ConsentStatus+")")
		case s.RightsStatus == "":
			undeclared = append(undeclared, s.UploadID)
		}
	}
	if len(unknown) > 0 {
		return mk(id, StatusFail, "no source with unknown rights or consent",
			strings.Join(unknown, ", "),
			"design input $.sources", "$.sources",
			"Resolve the rights/consent status of these uploads with the creator before publication — unknown rights block the package.")
	}
	if len(undeclared) > 0 {
		return mk(id, StatusWarn, "every source declares rightsStatus",
			"undeclared: "+strings.Join(undeclared, ", "),
			"design input $.sources", "$.sources",
			"Record rightsStatus (user_created|owned|licensed|public_domain) and consentStatus for each upload at intake.")
	}
	return mk(id, StatusPass, "all sources declare known rights",
		fmt.Sprintf("%d sources", len(di.Sources)),
		"design input $.sources", "$.sources", "")
}

// --- audio check ---

func checkAudioComplete(di *gamespec.RequiredInputs) Check {
	const id = "audio.complete"
	var problems []string
	if len(di.Audio.Sfx) == 0 {
		problems = append(problems, "no sfx declared (V7 wants one per player action)")
	}
	if !di.Audio.MuteToggle {
		problems = append(problems, "muteToggle is false (P10: every Poki title had one)")
	}
	// Music resolution itself is covered by assets.music_resolved.
	if len(problems) > 0 {
		return mk(id, StatusWarn, "sfx per action + mute toggle",
			strings.Join(problems, "; "), "design input $.audio",
			"$.audio",
			"Declare an sfx entry per player action and keep muteToggle true.")
	}
	return mk(id, StatusPass, "sfx per action + mute toggle", "complete",
		"design input $.audio", "$.audio", "")
}

// --- verdict ---

func decide(checks []Check, attempt, maxAttempts int) (string, RepairPlan) {
	plan := RepairPlan{Attempt: attempt, MaxAttempts: maxAttempts}
	var blockingFails []string
	var warnings int
	for _, c := range checks {
		switch c.Status {
		case StatusFail:
			if c.Severity == SeverityBlocking {
				blockingFails = append(blockingFails, c.CheckID)
			} else {
				warnings++
			}
		case StatusWarn:
			warnings++
		}
	}
	verdict := VerdictPass
	switch {
	case len(blockingFails) > 0:
		verdict = VerdictFail
	case warnings > 0:
		verdict = VerdictPassWithWarning
	}
	switch {
	case verdict == VerdictPass || verdict == VerdictPassWithWarning:
		plan.NextAction = ActionPublish
	case attempt < maxAttempts && allRepairable(checks, blockingFails):
		plan.NextAction = ActionAutoRepair
	default:
		plan.NextAction = ActionHumanReview
		plan.BlockedBy = blockingFails
	}
	return verdict, plan
}

func allRepairable(checks []Check, ids []string) bool {
	byID := make(map[string]Check, len(checks))
	for _, c := range checks {
		byID[c.CheckID] = c
	}
	for _, id := range ids {
		c, ok := byID[id]
		if !ok || !c.AutoRepairable {
			return false
		}
	}
	return true
}

func summarize(r *ValidationReport) string {
	var p, f, w, s int
	for _, c := range r.Checks {
		switch c.Status {
		case StatusPass:
			p++
		case StatusFail:
			f++
		case StatusWarn:
			w++
		case StatusSkip:
			s++
		}
	}
	return fmt.Sprintf("verdict=%s next=%s (attempt %d/%d): %d pass, %d fail, %d warn, %d skipped (harness-gated)",
		r.Verdict, r.Repair.NextAction, r.Repair.Attempt, r.Repair.MaxAttempts, p, f, w, s)
}
