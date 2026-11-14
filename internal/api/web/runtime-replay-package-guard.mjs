// Playback-only provenance checks. The server owns publication eligibility;
// neither a matched hash nor a replayed browser score is independent
// server-owned attestation. Historical runs may carry only the short board key.
const FULL_SHA256 = /^[0-9a-f]{64}$/;
const SHORT_BOARD_HASH = /^[0-9a-f]{12}$/;

export function validateStoredReplayPackageIdentity(run, pkg) {
  if (!run || typeof run !== 'object' || !pkg || typeof pkg !== 'object' ||
      !SHORT_BOARD_HASH.test(pkg.irHash) || !FULL_SHA256.test(pkg.irSha256) ||
      !SHORT_BOARD_HASH.test(run.irHash) || run.irHash !== pkg.irHash) {
    throw new Error('recorded replay game IR differs from the current package');
  }
  if (run.irSha256 == null || run.irSha256 === '') {
    // V1 records did not persist the full digest. Preserve browser-only
    // diagnostic playback by board key, NEVER treat it as attested evidence.
    return { provenance: 'legacy-short-ir-hash-only', attestable: false };
  }
  if (typeof run.irSha256 !== 'string' || !FULL_SHA256.test(run.irSha256) ||
      run.irSha256 !== pkg.irSha256 || run.irSha256.slice(0, 12) !== run.irHash) {
    throw new Error('recorded replay immutable IR digest mismatch');
  }
  return { provenance: 'full-ir-sha256-match', attestable: false };
}

export function validateStoredReplayKeyboardGrammar(replayIndex, ir) {
  if (!replayIndex) return;
  if (!(replayIndex.byStep instanceof Map) ||
      !ir || !Array.isArray(ir.inputBindings ?? [])) {
    throw new Error('invalid recorded replay binding context');
  }
  const bindings = (ir.inputBindings ?? []).filter((b) =>
    b && b.input === 'keyboard');
  const declared = new Set();
  for (const b of bindings) {
    if (typeof b.key === 'string' && b.key) declared.add(b.key);
    if (typeof b.action === 'string' && b.action) declared.add(b.action);
  }
  for (const events of replayIndex.byStep.values()) {
    if (!Array.isArray(events)) throw new Error('invalid recorded replay binding context');
    for (const event of events) {
      if ((event.type === 'key' || event.type === 'keyup') &&
          !declared.has(event.detail)) {
        // Older persisted traces could contain keyboard evidence that the
        // published IR could never produce. Do not silently drop it.
        throw new Error('recorded replay keyboard input is not declared by game IR');
      }
    }
  }
}
