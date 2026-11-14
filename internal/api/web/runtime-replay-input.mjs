// Authoritative gameplay replay evidence — trusted installed runtime only.
// Never drop the beginning of a trace: Go rejects more than 2000 inputs, and
// a truncated trace can falsely describe a run with the wrong initial state.
// An unsupported event type is also not server-replayable (including the
// current entity-pointer tap, which is not in Go's accepted input grammar).
// Gameplay continues when evidence is exhausted; leaderboard submission stops.
export const MAX_VERIFIABLE_RUN_INPUTS = 2000;
const ALLOWED_INPUT_TYPES = new Set(['pointer', 'pointerup', 'key', 'keyup']);

// Version 1.x replay pointer details are canonical signed integer logical
// coordinates in the preinstalled 2D player's coordinate space. Permit
// off-canvas captured drags but reject ambiguous/unbounded numbers: the
// playback adapter otherwise silently ignores malformed pointer records.
const MAX_REPLAY_POINTER_COORD = 1_000_000;
const CANONICAL_POINTER_COORD = /^(?:0|-?[1-9][0-9]*)$/;

function validReplayDetailShape(type, detail) {
  if (type === 'pointerup') return detail === '';
  if (type !== 'pointer') return true; // v1.x keys are matched by IR bindings
  const coords = detail.split(',');
  return coords.length === 2 && coords.every((text) =>
    CANONICAL_POINTER_COORD.test(text) &&
    Number.isSafeInteger(Number(text)) &&
    Math.abs(Number(text)) <= MAX_REPLAY_POINTER_COORD);
}

export function createRunInputRecorder(maxEvents = MAX_VERIFIABLE_RUN_INPUTS) {
  if (!Number.isSafeInteger(maxEvents) || maxEvents < 1 || maxEvents > MAX_VERIFIABLE_RUN_INPUTS) {
    throw new RangeError('invalid replay input trace budget');
  }
  const inputs = [];
  let complete = true;
  let failureReason = null;
  return {
    inputs,
    get complete() { return complete; },
    get failureReason() { return failureReason; },
    record(tick, type, detail = '') {
      if (!complete) return false;
      if (!Number.isSafeInteger(tick) || tick < 0 ||
          typeof detail !== 'string' || detail.length > 128) {
        complete = false;
        failureReason = 'invalid_event';
        return false;
      }
      if (!ALLOWED_INPUT_TYPES.has(type)) {
        complete = false;
        failureReason = 'unsupported_input';
        return false;
      }
      if (!validReplayDetailShape(type, detail)) {
        complete = false;
        failureReason = 'invalid_event';
        return false;
      }
      // Events are recorded in semantic tick order. Multiple inputs at
      // the same 60Hz step are legal, but going backwards would create an
      // ambiguous replay and must permanently invalidate this evidence.
      if (inputs.length && tick < inputs[inputs.length - 1].tick) {
        complete = false;
        failureReason = 'invalid_event';
        return false;
      }
      if (inputs.length >= maxEvents) {
        complete = false;
        failureReason = 'input_limit';
        return false;
      }
      inputs.push({ tick, type, detail });
      return true;
    },
  };
}


// Replayed inputs cross a server-to-browser persistence boundary. Even when
// the submission endpoint enforces a bounded, ordered grammar, older records
// and storage corruption must not allocate an unbounded step map or silently
// skip malformed actions. This is a *client playback guard*, never server
// attestation or proof of a score.
export function indexValidatedReplayInputs(rawInputs, durationMs) {
  // Historic inputless records may omit the JSON field (Go omitempty).
  const inputs = rawInputs == null ? [] : rawInputs;
  if (!Array.isArray(inputs) || inputs.length > MAX_VERIFIABLE_RUN_INPUTS) {
    throw new Error('invalid recorded replay input count');
  }
  if (durationMs !== undefined && durationMs !== null &&
      (!Number.isSafeInteger(durationMs) || durationMs < 1000 || durationMs > 1_800_000)) {
    throw new Error('invalid recorded replay duration');
  }
  // Go permits one second of fixed-step slack for legacy captures. Without
  // the optional duration field, still bound the trace to a 30-minute run.
  const maxTick = durationMs == null
    ? 1_800_000 * 60 / 1000 + 60
    : Math.floor(durationMs * 60 / 1000) + 60;
  const byStep = new Map();
  let lastTick = 0;
  let previousTick = -1;
  for (const entry of inputs) {
    if (!entry || !Number.isSafeInteger(entry.tick) || entry.tick < previousTick ||
        entry.tick < 0 || entry.tick > maxTick ||
        !ALLOWED_INPUT_TYPES.has(entry.type) ||
        typeof entry.detail !== 'string' || entry.detail.length > 128 ||
        !validReplayDetailShape(entry.type, entry.detail)) {
      throw new Error('invalid recorded replay event');
    }
    const eventsAtStep = byStep.get(entry.tick) || [];
    eventsAtStep.push(entry);
    byStep.set(entry.tick, eventsAtStep);
    previousTick = entry.tick;
    lastTick = entry.tick;
  }
  return { byStep, lastTick, eventCount: inputs.length };
}


// Interpret version-1 pointer samples as a single physical contact: the
// first sample after release is DOWN, subsequent samples are MOVE, and
// pointerup ends that contact. Live recording historically reused 'pointer'
// for both down and drag samples; replaying every sample as a tap invented
// gameplay inputs that never occurred in the original run.
export function replayPointerPhase(type, wasDown) {
  if (type === 'pointer') return wasDown ? 'move' : 'down';
  if (type === 'pointerup') return wasDown ? 'up' : 'redundant-up';
  return null;
}


// The dead-input-rate probe is diagnostic only. A frame is not guaranteed to
// advance while backgrounded or waiting for a user Start tap, so a flood of
// inputs cannot be allowed to grow this queue forever. When exhausted the
// result is *unknown*, not an invented zero/bad-input quality measurement.
export const MAX_PENDING_INPUT_DIAGNOSTICS = 256;

export function createDeadInputDiagnostics(maxPending = MAX_PENDING_INPUT_DIAGNOSTICS) {
  if (!Number.isSafeInteger(maxPending) || maxPending < 1 || maxPending > MAX_PENDING_INPUT_DIAGNOSTICS) {
    throw new RangeError('invalid pending input diagnostic budget');
  }
  const pending = [];
  let complete = true;
  return {
    pending,
    get complete() { return complete; },
    record(sample) {
      if (!complete) return false;
      if (!sample || !Number.isFinite(sample.at) || sample.at < 0) {
        pending.length = 0;
        complete = false;
        return false;
      }
      if (pending.length >= maxPending) {
        pending.length = 0;
        complete = false;
        return false;
      }
      pending.push(sample);
      return true;
    },
    drain(visibleMS, lastActivityMS) {
      if (!complete) return 0;
      let dead = 0;
      while (pending.length && visibleMS - pending[0].at >= 200) {
        const input = pending.shift();
        if (lastActivityMS < input.at) dead++;
      }
      return dead;
    },
  };
}
