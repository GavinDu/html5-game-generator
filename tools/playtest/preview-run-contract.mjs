// A preview is evidence of a specific recorded run, not arbitrary gameplay.
// Reject stale, mismatched or malformed traces before starting a browser.
export function validateRecordedRun(record, { game, run, irHash }) {
  if (!record || typeof record !== 'object' || Array.isArray(record)) {
    throw new Error('recorded replay response is not an object');
  }
  if (record.id !== run) {
    throw new Error('recorded replay ID does not match requested run');
  }
  if (record.gameId !== game) {
    throw new Error('recorded replay belongs to a different game');
  }
  if (!irHash || record.irHash !== irHash) {
    throw new Error('recorded replay IR hash does not match the package version');
  }
  if (!Number.isSafeInteger(record.seed) || record.seed < 0 || record.seed > 0xffffffff) {
    throw new Error('recorded replay has no valid deterministic seed');
  }
  if (record.inputs !== undefined && !Array.isArray(record.inputs)) {
    throw new Error('recorded replay has no valid input trace');
  }
  return record;
}
