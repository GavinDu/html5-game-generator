// One trusted, renderer-independent cadence policy for the installed
// Pixi/Rapier Game IR runtime. Do not import executable scripts from Game IR.
// Keep the historical 60 Hz accumulator, 250 ms per-frame input clamp,
// three catch-up ticks and full-backlog shed *exactly* compatible with
// previously published games. This module neither runs physics nor rules.
export const FIXED_STEP_MS = 1000 / 60;
export const MAX_CATCHUP_STEPS = 3;
export const MAX_FRAME_DELTA_MS = 250;

export function createFixedStepClock() {
  let accumulatorMS = 0;
  return {
    stepMS: FIXED_STEP_MS,
    addFrame(deltaMS) {
      // Renderer deltas are positive finite Pixi ticker values. Preserve the
      // original clamp and floating-point operation order for deterministic
      // run and replay compatibility.
      accumulatorMS += Math.min(deltaMS, MAX_FRAME_DELTA_MS);
    },
    hasPendingStep(steps) {
      return accumulatorMS >= FIXED_STEP_MS && steps < MAX_CATCHUP_STEPS;
    },
    completedStep() {
      accumulatorMS -= FIXED_STEP_MS;
    },
    finishFrame(steps) {
      // Deliberately slow semantic time under sustained overload; never
      // run unbounded catch-up after a stall or background transition.
      if (steps === MAX_CATCHUP_STEPS && accumulatorMS >= FIXED_STEP_MS) {
        accumulatorMS = 0;
      }
    },
    reset() { accumulatorMS = 0; },
    snapshot() { return { accumulatorMS }; },
  };
}
