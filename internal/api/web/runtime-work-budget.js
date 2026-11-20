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


// Opt-in diagnostic-only witness for the *actual* fixed-step input queue
// commit, not the earlier DOM timestamp or a server-authenticated replay.
// Holds neither input source/target objects nor arbitrary event detail.
// On any invalid, out-of-order or over-budget event, clear the prefix and
// mark it incomplete; never silently publish a clipped "complete" history.
export const MAX_COMMITTED_INPUT_WITNESS_EVENTS = 128;
const COMMITTED_ACTION_ID = /^[A-Za-z0-9_-]{1,128}$/;
export function createCommittedInputWitness(capacity = MAX_COMMITTED_INPUT_WITNESS_EVENTS) {
  if (!Number.isSafeInteger(capacity) || capacity < 1 ||
      capacity > MAX_COMMITTED_INPUT_WITNESS_EVENTS) {
    throw new RangeError('invalid committed input witness budget');
  }
  let complete = true;
  let failureReason = null;
  let lastTick = -1;
  let lastOrdinal = -1;
  let accepted = 0;
  const entries = [];
  const fail = (reason) => {
    complete = false;
    failureReason = reason;
    entries.length = 0;
    return false;
  };
  return Object.freeze({
    record(tick, action) {
      if (!complete) return false;
      if (!Number.isSafeInteger(tick) || tick < 0 || tick > 108_000 ||
          typeof action !== 'string' || !COMMITTED_ACTION_ID.test(action)) {
        return fail('invalid_commit');
      }
      if (tick < lastTick) return fail('backwards_tick');
      if (entries.length >= capacity) return fail('witness_limit');
      const ordinal = tick === lastTick ? lastOrdinal + 1 : 0;
      entries.push({ tick, ordinal, action });
      lastTick = tick;
      lastOrdinal = ordinal;
      accepted++;
      return true;
    },
    snapshot() {
      return {
        complete, failureReason, accepted,
        entries: entries.map(({ tick, ordinal, action }) => ({ tick, ordinal, action })),
      };
    },
  });
}


// One lifecycle predicate for the Pixi simulation/render ticker. This
// does not advance virtual time, resume audio, or accept untrusted Game IR.
// A terminal, errored, hidden, orientation-blocked, or context-lost session
// must not resurrect simulation on an unrelated focus/resize event.
// Deterministic capture is driven only by its explicit test step controller.
export function shouldRunSemanticTicker({
  shellStarted = false, hidden = true, orientationBlocked = false,
  contextLost = false, ended = false, failed = false,
  renderFailed = false, deterministicCapture = false,
} = {}) {
  return shellStarted === true && hidden === false &&
    !orientationBlocked && !contextLost && !ended && !failed &&
    !renderFailed && !deterministicCapture;
}
