// Probe-only deterministic semantic-state diagnostic, version 1.
// This is *not* a cryptographic attestation or a replacement for a pure
// server-side Game IR interpreter. It deliberately excludes Pixi view state,
// animation frame, GPU objects, wall clock, and output frame pacing.
const MAX_ENTITIES = 512;
const MAX_FIELDS = 512;
const MAX_COORD = 1_000_000_000;
const MASK_64 = 0xffffffffffffffffn;

function text(value) {
  if (typeof value !== 'string' || value.length > 256) {
    throw new Error('invalid semantic state identifier');
  }
  return value;
}
function finiteMilli(value) {
  if (typeof value !== 'number' || !Number.isFinite(value) || Math.abs(value) > MAX_COORD) {
    throw new Error('non-finite/out-of-bounds semantic state number');
  }
  return Math.round(value * 1000);
}
function scalar(value) {
  if (typeof value === 'number') return ['n', finiteMilli(value)];
  if (typeof value === 'string') return ['s', text(value)];
  if (typeof value === 'boolean') return ['b', value];
  throw new Error('unsupported semantic state variable type');
}
function sortedStrings(values) {
  const array = Array.from(values ?? [], text);
  if (array.length > MAX_FIELDS) throw new Error('semantic state set is over budget');
  return array.sort();
}
function sortedPairs(entries) {
  const pairs = Array.from(entries ?? [], ([name, value]) => [text(name), scalar(value)]);
  if (pairs.length > MAX_FIELDS) throw new Error('semantic state map is over budget');
  // Use raw UTF-16 lexical order, not locale-dependent ICU collation.
  pairs.sort((a, b) => a[0] < b[0] ? -1 : a[0] > b[0] ? 1 : 0);
  for (let i = 1; i < pairs.length; i++) {
    if (pairs[i][0] === pairs[i - 1][0]) throw new Error('duplicate semantic map key');
  }
  return pairs;
}
function canonicalEntities(entities) {
  const selected = Array.from(entities ?? []).filter(e => !e.destroyed);
  if (selected.length > MAX_ENTITIES) throw new Error('semantic entity count exceeds budget');
  const output = selected.map(e => [
    text(String(e.runtimeId)), text(String(e.defId)), text(String(e.def?.kind ?? '')),
    finiteMilli(e.x), finiteMilli(e.y), finiteMilli(e.w), finiteMilli(e.h),
    !!e.closed,
  ]);
  output.sort((a, b) => a[0] < b[0] ? -1 : a[0] > b[0] ? 1 : 0);
  for (let i = 1; i < output.length; i++) {
    if (output[i][0] === output[i - 1][0]) throw new Error('duplicate semantic entity ID');
  }
  return output;
}

// FNV-1a over UTF-16 code units is a compact, cross-browser *diagnostic*.
// Collisions are possible. Never authorize a score, asset, or verified result
// from this value; a future independent attestor needs canonical SHA-256.
function diagnosticHash(canonical) {
  let hash = 0xcbf29ce484222325n;
  for (let i = 0; i < canonical.length; i++) {
    hash ^= BigInt(canonical.charCodeAt(i));
    hash = (hash * 0x100000001b3n) & MASK_64;
  }
  return 'diagnostic-fnv1a64-v1:' + hash.toString(16).padStart(16, '0');
}

export function semanticFingerprintV1(sample) {
  if (!sample || !Number.isSafeInteger(sample.simStep) || sample.simStep < 0 ||
      !Number.isInteger(sample.rngState) || sample.rngState < 0 ||
      sample.rngState > 0xffffffff ||
      !Number.isSafeInteger(sample.pendingSemanticInputs) ||
      sample.pendingSemanticInputs < 0 || sample.pendingSemanticInputs > 512) {
    throw new Error('invalid semantic tick, RNG, or input queue state');
  }
  const vars = sample.variables ?? {};
  if (!vars || typeof vars !== 'object' || Array.isArray(vars)) {
    throw new Error('invalid semantic variable map');
  }
  const values = sortedPairs(Object.entries(vars));
  const state = {
    schema: 1,
    tick: sample.simStep,
    rng: sample.rngState,
    score: finiteMilli(sample.score),
    health: finiteMilli(sample.health),
    objective: finiteMilli(sample.objectiveProgress),
    depth: finiteMilli(sample.depth),
    ended: !!sample.ended,
    won: !!sample.won,
    failed: !!sample.failed,
    variables: values,
    timers: sortedPairs(sample.timers),
    collectedPairs: sortedStrings(sample.collectedPairs),
    insideRegions: sortedStrings(sample.insideRegions),
    firedThresholds: sortedStrings(sample.firedThresholds),
    heldKeys: sortedStrings(sample.heldKeys),
    pointerDown: !!sample.pointerDown,
    pendingInputs: sample.pendingSemanticInputs,
    entities: canonicalEntities(sample.entities),
  };
  const canonical = JSON.stringify(state);
  return {
    schema: 1,
    hash: diagnosticHash(canonical),
    canonical,
    entityCount: state.entities.length,
  };
}
