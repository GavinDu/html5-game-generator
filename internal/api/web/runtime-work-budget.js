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


// Browser pointer/key events can arrive between render frames. Preserve their
// arrival order while executing game-affecting rules only on the next fixed
// simulation step, under the same CPU budget as physics/timers. Overrun is an
// explicit error (never a silent dropped input or a fabricated winning run).
export const MAX_QUEUED_SEMANTIC_INPUTS = 128;
export function createSemanticInputQueue(capacity = MAX_QUEUED_SEMANTIC_INPUTS) {
  if (!Number.isSafeInteger(capacity) || capacity < 1) {
    throw new RangeError('semantic input queue capacity must be a positive safe integer');
  }
  const events = [];
  return Object.freeze({
    enqueue(event) {
      if (events.length >= capacity) throw new Error('semantic input queue budget exceeded');
      events.push(event);
    },
    drain(consumer) {
      // Snapshot and clear first. Any reentrant input is reserved for the next
      // tick, not inserted into a currently executing cascade.
      const ordered = events.splice(0);
      for (const event of ordered) consumer(event);
      return ordered.length;
    },
    clear() { events.length = 0; },
    get size() { return events.length; },
  });
}
