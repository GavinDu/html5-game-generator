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
