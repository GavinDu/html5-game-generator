// Authoritative gameplay replay evidence — trusted installed runtime only.
// Never drop the beginning of a trace: Go rejects more than 2000 inputs, and
// a truncated trace can falsely describe a run with the wrong initial state.
// This recorder has an explicit terminal completeness flag. Gameplay itself
// continues when evidence is exhausted; only leaderboard submission stops.
export const MAX_VERIFIABLE_RUN_INPUTS = 2000;
const ALLOWED_INPUT_TYPES = new Set(['pointer', 'pointerup', 'key', 'keyup', 'entity-pointer']);

export function createRunInputRecorder(maxEvents = MAX_VERIFIABLE_RUN_INPUTS) {
  if (!Number.isSafeInteger(maxEvents) || maxEvents < 1 || maxEvents > MAX_VERIFIABLE_RUN_INPUTS) {
    throw new RangeError('invalid replay input trace budget');
  }
  const inputs = [];
  let complete = true;
  return {
    inputs,
    get complete() { return complete; },
    record(tick, type, detail = '') {
      if (!complete) return false;
      if (!Number.isSafeInteger(tick) || tick < 0 ||
          !ALLOWED_INPUT_TYPES.has(type) ||
          typeof detail !== 'string' || detail.length > 128 ||
          inputs.length >= maxEvents) {
        complete = false;
        return false;
      }
      inputs.push({ tick, type, detail });
      return true;
    },
  };
}
