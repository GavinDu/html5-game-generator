import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { semanticFingerprintV1 } from '../../internal/api/web/runtime-semantic-fingerprint.mjs';

function sample() {
  return {
    simStep: 120, rngState: 0x12345678,
    score: 15, health: 2, objectiveProgress: 3, depth: 0,
    ended: false, won: false, failed: false,
    variables: { z_lives: 2, a_points: 10 },
    timers: new Map([['z-spawn', 0.5], ['a-pickup', 1 / 3]]),
    collectedPairs: new Set(['p:2', 'p:1']),
    insideRegions: new Set(['zone-b', 'zone-a']),
    firedThresholds: new Set(['r-2', 'r-1']),
    heldKeys: new Set(['ArrowRight', 'Space']),
    pointerDown: false,
    pendingSemanticInputs: 1,
    entities: [
      { runtimeId: 'item-2', defId: 'pickup', def: { kind: 'pickup' },
        x: 22.75, y: 91.125, w: 18, h: 18, closed: false,
        view: { alpha: 0.1, animationFrame: 99 } },
      { runtimeId: 'player-1', defId: 'hero', def: { kind: 'player' },
        x: 180, y: 720, w: 36, h: 44, closed: false,
        view: { alpha: 0.9, animationFrame: 1 } },
    ],
  };
}

test('semantic fingerprint is invariant to Map/Set insertion and entity iteration order', () => {
  const a = sample();
  const b = sample();
  b.variables = { a_points: 10, z_lives: 2 };
  b.timers = new Map([...b.timers].reverse());
  b.collectedPairs = new Set([...b.collectedPairs].reverse());
  b.insideRegions = new Set([...b.insideRegions].reverse());
  b.firedThresholds = new Set([...b.firedThresholds].reverse());
  b.heldKeys = new Set([...b.heldKeys].reverse());
  b.entities.reverse();
  const x = semanticFingerprintV1(a), y = semanticFingerprintV1(b);
  assert.equal(x.schema, 1);
  assert.equal(x.entityCount, 2);
  assert.match(x.hash, /^diagnostic-fnv1a64-v1:[0-9a-f]{16}$/);
  assert.equal(x.canonical, y.canonical);
  assert.equal(x.hash, y.hash);
});

test('visual-only sprite changes and sub-millipixel jitter cannot alter semantics', () => {
  const a = sample(), b = sample();
  b.entities[0].view.alpha = 0;
  b.entities[0].view.animationFrame = 999;
  b.entities[0].x += 0.0001;
  assert.equal(semanticFingerprintV1(a).hash, semanticFingerprintV1(b).hash);
});

test('score, fixed tick, RNG, terminal reason and real movement must change signature', () => {
  const expected = semanticFingerprintV1(sample()).hash;
  for (const mutate of [
    s => { s.score += 1; },
    s => { s.simStep++; },
    s => { s.rngState++; },
    s => { s.ended = true; s.won = true; },
    s => { s.entities[0].x += 0.02; },
    s => { s.timers.set('z-spawn', 0.8); },
    s => { s.pendingSemanticInputs++; },
  ]) {
    const next = sample(); mutate(next);
    assert.notEqual(semanticFingerprintV1(next).hash, expected);
  }
});

test('destroyed entities are excluded and duplicate active entity IDs fail closed', () => {
  const a = sample(), b = sample();
  b.entities.push({ runtimeId: 'old', destroyed: true });
  assert.equal(semanticFingerprintV1(a).hash, semanticFingerprintV1(b).hash);
  b.entities.push({ ...b.entities[0] });
  assert.throws(() => semanticFingerprintV1(b), /duplicate semantic entity ID/);
});

test('fail closed on nonfinite state, unbounded entities, map types and invalid tick', () => {
  for (const mutate of [
    s => { s.score = NaN; },
    s => { s.health = Infinity; },
    s => { s.simStep = -1; },
    s => { s.rngState = 2 ** 32; },
    s => { s.variables.a_points = {}; },
    s => { s.timers.set('extra', NaN); },
    s => { s.entities[0].x = Infinity; },
    s => { s.entities = Array.from({ length: 513 }, (_, i) => ({
      runtimeId: 'e' + i, defId: 'd', def: { kind: 'pickup' },
      x: 0, y: 0, w: 1, h: 1,
    })); },
  ]) {
    const bad = sample(); mutate(bad);
    assert.throws(() => semanticFingerprintV1(bad));
  }
});

test('shipping runtime exposes digest only in read-only probe snapshots', () => {
  const source = readFileSync(new URL('../../internal/api/web/pixi-runtime.js', import.meta.url), 'utf8');
  assert.match(source, /import \{ semanticFingerprintV1 \} from '\.\/runtime-semantic-fingerprint\.mjs'/);
  assert.match(source, /if \(probeMode && !abort\.signal\.aborted\) \{[\s\S]*?const probeSessionAPI = \{/);
  assert.match(source, /semanticFingerprint: semanticFingerprintV1\(\{/);
  assert.match(source, /rngState: sessionRandom\.getState\(\)/);
  assert.match(source, /entities: entities\.values\(\)/);
  assert.doesNotMatch(source, /sendNativeMetric\('semanticFingerprint'/);
});
