// Deterministic, finite interpreter work per 60Hz semantic tick.
// Imported by the sole Pixi Game IR interpreter; no host APIs or input time.
export const SIMULATION_WORK_LIMITS = Object.freeze({
  maxChecks: 32768,
  maxActions: 8192,
});

export function createSimulationWorkBudget(limits = SIMULATION_WORK_LIMITS) {
  if (!Number.isSafeInteger(limits.maxChecks) || limits.maxChecks < 1 ||
      !Number.isSafeInteger(limits.maxActions) || limits.maxActions < 1) {
    throw new RangeError('simulation budget limits must be positive safe integers');
  }
  let checks = 0;
  let actions = 0;
  return Object.freeze({
    reset() { checks = 0; actions = 0; },
    consumeCheck() {
      if (checks >= limits.maxChecks) throw new Error('rule evaluation work budget exceeded');
      checks++;
    },
    consumeAction() {
      if (actions >= limits.maxActions) throw new Error('rule action work budget exceeded');
      actions++;
    },
    snapshot() { return Object.freeze({ checks, actions }); },
  });
}
