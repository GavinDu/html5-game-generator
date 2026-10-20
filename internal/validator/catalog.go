package validator

// checkDef is the static catalog entry behind one checkId.
type checkDef struct {
	id             string
	category       string
	severity       string
	description    string
	autoRepairable bool
}

// catalog is the full check suite. Statuses are assigned at run time;
// severities and repairability are policy and live here.
var catalog = []checkDef{
	// Schema: the design input and IR must be well-formed data.
	{id: "schema.design_input_valid", category: CatSchema, severity: SeverityBlocking,
		description: "GameDesignInput passes structural validation (required fields, closed enums, budget maximums).",
		autoRepairable: true},
	{id: "schema.spec_version_known", category: CatSchema, severity: SeverityBlocking,
		description: "designInput.schemaVersion is a supported version.",
		autoRepairable: true},
	// Assets: every referenced asset must resolve to a real packaged file.
	{id: "assets.all_resolved", category: CatAssets, severity: SeverityBlocking,
		description: "Every designRef/uploadId in the design input resolves to a manifest element or sprite.",
		autoRepairable: true},
	{id: "assets.files_present", category: CatAssets, severity: SeverityBlocking,
		description: "The entry script and every sprite file exist on disk in the game directory.",
		autoRepairable: true},
	{id: "assets.music_resolved", category: CatAssets, severity: SeverityWarning,
		description: "The declared music source resolves (on-disk file or library source).",
		autoRepairable: true},
	// Capabilities: the degradation guarantee. Skipped until the capability
	// ABI lands in RuntimeSpec.
	{id: "capabilities.fallbacks_complete", category: CatCapabilities, severity: SeverityBlocking,
		description: "Every REQUIRED capability declares a working fallback.",
		autoRepairable: true},
	// Budgets: declared and measured limits.
	{id: "budgets.session_seconds", category: CatBudgets, severity: SeverityBlocking,
		description: "constraints.sessionTargetSec <= 90.",
		autoRepairable: true},
	{id: "budgets.package_mb", category: CatBudgets, severity: SeverityBlocking,
		description: "Measured game directory size <= 25 MB.",
		autoRepairable: true},
	{id: "budgets.cold_start", category: CatBudgets, severity: SeverityBlocking,
		description: "Measured cold start to playable <= 3s.",
		autoRepairable: true},
	// Hook: the first-60-seconds contract. Bot-measured; skipped until the
	// bot harness exists.
	{id: "hook.time_to_input", category: CatHook, severity: SeverityBlocking,
		description: "Bot accepts input within 5s of Play.",
		autoRepairable: true},
	{id: "hook.first_reward", category: CatHook, severity: SeverityBlocking,
		description: "Bot observes a first reward event within 15s.",
		autoRepairable: true},
	{id: "hook.no_mandatory_tutorial", category: CatHook, severity: SeverityBlocking,
		description: "No blocking tutorial text before first input.",
		autoRepairable: true},
	{id: "hook.restart_terms", category: CatHook, severity: SeverityWarning,
		description: "Death restores control within 2s and at most 1 click, with no menu navigation.",
		autoRepairable: true},
	// Simulation: headless executor gates. Skipped until the harness exists.
	{id: "simulation.win_reachable", category: CatSimulation, severity: SeverityBlocking,
		description: "Bot reaches the win condition within the session budget.",
		autoRepairable: true},
	{id: "simulation.no_softlock", category: CatSimulation, severity: SeverityBlocking,
		description: "Fuzz run finds no unrecoverable states.",
		autoRepairable: true},
	{id: "simulation.level1_clearable", category: CatSimulation, severity: SeverityBlocking,
		description: "A weak baseline bot clears level 1 (fairness gate).",
		autoRepairable: true},
	{id: "simulation.difficulty_ramp", category: CatSimulation, severity: SeverityWarning,
		description: "Fail rate rises across early play; the game escalates, never plateaus.",
		autoRepairable: true},
	{id: "simulation.memory_bound", category: CatSimulation, severity: SeverityBlocking,
		description: "Peak memory stays under the declared bound during simulation.",
		autoRepairable: true},
	// Safety: kid-safety is blocking and static; content screening needs a human.
	{id: "safety.kid_safe", category: CatSafety, severity: SeverityBlocking,
		description: "No external URLs, no real-money hooks, no chat/UGC primitives in the game bundle.",
		autoRepairable: true},
	{id: "safety.content_screen", category: CatSafety, severity: SeverityWarning,
		description: "Uploaded art passed content screening.",
		autoRepairable: false},
	// Audio: completeness of the feedback layer.
	{id: "audio.complete", category: CatAudio, severity: SeverityWarning,
		description: "SFX declared per player action, mute toggle present, no autoplay before first input.",
		autoRepairable: true},
	// Design: the comedy-framework v1.1 generative layer. These are quality
	// signals (warnings, never publish blockers): they force the AI to find
	// the emotional core, the mechanical surprise, and to justify every
	// mechanic — the disciplines that separate an invented game from a
	// template with art.
	{id: "design.premise_declared", category: CatDesign, severity: SeverityWarning,
		description: "Premise carries a valid attitude (strange|sucks|scary|silly) and a one-line emotional statement (Judy Carter).",
		autoRepairable: true},
	{id: "design.twist_declared", category: CatDesign, severity: SeverityWarning,
		description: "Strategy declares a mechanical twist whose reinterpretation differs from the player's assumption (Greg Dean).",
		autoRepairable: true},
	{id: "design.mechanic_diet", category: CatDesign, severity: SeverityWarning,
		description: "Every powerup/mode justifies itself with a loopLink, and the mechanic count fits the session budget (Li Dan).",
		autoRepairable: true},
	// Rights gate (questionnaire v2 §B / Stage 0): every uploaded source
	// declares who owns it and what may be done with it. Explicitly unknown
	// rights block publication; undeclared rights warn and route to human
	// review. Rights are a creator/legal fact — never auto-repairable.
	{id: "design.rights_declared", category: CatDesign, severity: SeverityBlocking,
		description: "Every source declares rightsStatus/consentStatus; permission_unknown rights block publication (questionnaire v2 §B).",
		autoRepairable: false},
}

// checkIndex maps checkId -> catalog position.
var checkIndex = func() map[string]int {
	m := make(map[string]int, len(catalog))
	for i, c := range catalog {
		m[c.id] = i
	}
	return m
}()
