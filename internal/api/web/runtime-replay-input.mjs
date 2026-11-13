// Authoritative gameplay replay evidence — trusted installed runtime only.
// Never drop the beginning of a trace: Go rejects more than 2000 inputs, and
// a truncated trace can falsely describe a run with the wrong initial state.
// An unsupported event type is also not server-replayable (including the
// current entity-pointer tap, which is not in Go's accepted input grammar).
// Gameplay continues when evidence is exhausted; leaderboard submission stops.
export const MAX_VERIFIABLE_RUN_INPUTS = 2000;
const ALLOWED_INPUT_TYPES = new Set(['pointer', 'pointerup', 'key', 'keyup']);

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
