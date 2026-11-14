import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { indexValidatedReplayInputs } from '../../internal/api/web/runtime-replay-input.mjs';
import { validateStoredReplayPackageIdentity, validateStoredReplayKeyboardGrammar }
  from '../../internal/api/web/runtime-replay-package-guard.mjs';

const runtime = readFileSync(new URL('../../internal/api/web/pixi-runtime.js', import.meta.url), 'utf8');
const full = 'a'.repeat(64);
const pkg = { irHash: full.slice(0, 12), irSha256: full };
const modern = { irHash: pkg.irHash, irSha256: pkg.irSha256 };

test('ordinary and capture replay require current Game IR identity, full digest for modern records', () => {
  assert.deepEqual(validateStoredReplayPackageIdentity(modern, pkg),
    { provenance: 'full-ir-sha256-match', attestable: false });
  assert.deepEqual(validateStoredReplayPackageIdentity({ irHash: pkg.irHash }, pkg),
    { provenance: 'legacy-short-ir-hash-only', attestable: false });
  for (const [name, run, delivered] of [
    ['old version', { ...modern, irHash: 'b'.repeat(12) }, pkg],
    ['missing board hash', { irSha256: full }, pkg],
    ['changed full hash with same short board key',
      { ...modern, irSha256: 'a'.repeat(12) + 'b'.repeat(52) }, pkg],
    ['malformed full sha', { ...modern, irSha256: true }, pkg],
    ['bad package full sha', modern, { ...pkg, irSha256: 'garbage' }],
    ['missing package', modern, undefined],
    ['wrong package hash', modern, { ...pkg, irHash: 'b'.repeat(12) }],
    ['inconsistent stored digest prefix',
      { ...modern, irSha256: 'b'.repeat(64) }, { ...pkg, irSha256: 'b'.repeat(64) }],
  ]) {
    assert.throws(() => validateStoredReplayPackageIdentity(run, delivered),
      /recorded replay/, name);
  }
});

test('historic stored keyboard actions must be declared by current Game IR before playback', () => {
  const ir = { inputBindings: [
    { input: 'keyboard', key: 'ArrowLeft', action: 'move_left' },
    { input: 'tap', action: 'tap' },
  ] };
  const good = indexValidatedReplayInputs([
    { tick: 1, type: 'key', detail: 'ArrowLeft' },
    { tick: 1, type: 'key', detail: 'move_left' },
    { tick: 3, type: 'keyup', detail: 'ArrowLeft' },
    { tick: 3, type: 'pointerup', detail: '' },
  ], 1000);
  assert.doesNotThrow(() => validateStoredReplayKeyboardGrammar(good, ir));
  for (const detail of ['ArrowRight', 'MOVE_LEFT', 'tap', '']) {
    const bad = indexValidatedReplayInputs([{ tick: 1, type: 'key', detail }], 1000);
    assert.throws(() => validateStoredReplayKeyboardGrammar(bad, ir),
      /keyboard input is not declared/, detail);
  }
  const noKeys = indexValidatedReplayInputs([{ tick: 1, type: 'key', detail: 'ArrowLeft' }], 1000);
  assert.throws(() => validateStoredReplayKeyboardGrammar(noKeys, { inputBindings: [] }),
    /keyboard input is not declared/);
  assert.doesNotThrow(() => validateStoredReplayKeyboardGrammar(undefined, ir));
  assert.doesNotThrow(() => validateStoredReplayKeyboardGrammar(
    indexValidatedReplayInputs([{ tick: 1, type: 'pointer', detail: '1,2' }], 1000),
    { inputBindings: [] }));
});

test('shipped player guards game package and replay keys before scene/world allocation', () => {
  assert.match(runtime, /import \{ validateStoredReplayPackageIdentity, validateStoredReplayKeyboardGrammar \} from '\.\/runtime-replay-package-guard\.mjs'/);
  assert.match(runtime, /if \(replayTrace\) validateStoredReplayPackageIdentity\(replayTrace, pkg\)/);
  assert.match(runtime, /if \(replayIndex\) validateStoredReplayKeyboardGrammar\(replayIndex, ir\)/);
  const pkgGuard = runtime.indexOf('if (replayTrace) validateStoredReplayPackageIdentity(replayTrace, pkg)');
  const grammarGuard = runtime.indexOf('if (replayIndex) validateStoredReplayKeyboardGrammar(replayIndex, ir)');
  const scene = runtime.indexOf('const scene = (ir.scenes');
  assert.ok(pkgGuard > 0 && grammarGuard > pkgGuard && grammarGuard < scene);
  assert.doesNotMatch(runtime, /if \(deterministicCapture && replayTrace && replayTrace\.irHash/);
});
