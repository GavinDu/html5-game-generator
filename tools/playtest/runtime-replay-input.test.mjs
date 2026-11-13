import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { createRunInputRecorder, MAX_VERIFIABLE_RUN_INPUTS } from '../../internal/api/web/runtime-replay-input.mjs';

const runtime = readFileSync(new URL('../../internal/api/web/pixi-runtime.js', import.meta.url), 'utf8');

test('exactly 2000 replay events retain the complete ordered prefix and remain verifiable', () => {
  const trace = createRunInputRecorder();
  for (let i = 0; i < MAX_VERIFIABLE_RUN_INPUTS; i++) {
    assert.equal(trace.record(i, 'pointer', `${i % 390},${i % 844}`), true);
  }
  assert.equal(trace.complete, true);
  assert.equal(trace.inputs.length, 2000);
  assert.deepEqual(trace.inputs[0], { tick: 0, type: 'pointer', detail: '0,0' });
  assert.deepEqual(trace.inputs[1999], {
    tick: 1999, type: 'pointer', detail: `${1999 % 390},${1999 % 844}`,
  });
});

test('2001st mixed normal or held-pointer sample fails closed and never evicts earlier actions', () => {
  const trace = createRunInputRecorder();
  trace.record(0, 'key', 'ArrowRight');
  for (let i = 1; i < 1999; i++) trace.record(i, 'pointer', '10,20');
  trace.record(1999, 'keyup', 'ArrowRight');
  const oldHead = { ...trace.inputs[0] };
  assert.equal(trace.record(2000, 'pointerup', ''), false);
  assert.equal(trace.complete, false);
  assert.equal(trace.inputs.length, 2000);
  assert.deepEqual(trace.inputs[0], oldHead, 'must not discard the beginning of the replay');
  assert.deepEqual(trace.inputs.at(-1), { tick: 1999, type: 'keyup', detail: 'ArrowRight' });
  for (let i = 2001; i < 40000; i++) trace.record(i, 'pointer', '20,40');
  assert.equal(trace.inputs.length, 2000, 'long held-pointer run has a constant memory ceiling');
  assert.equal(trace.complete, false);
});

test('malformed input and invalid budgets become explicitly unverifiable, never silently truncated', () => {
  for (const limit of [0, 2001, -1, NaN, Infinity, 0.5, Number.MAX_SAFE_INTEGER + 1]) {
    assert.throws(() => createRunInputRecorder(limit), RangeError);
  }
  for (const invalid of [
    [-1, 'pointer', '10,20'],
    [1.5, 'pointer', '10,20'],
    [0, 'fake', ''],
    [0, 'pointer', String(repeatingSymbol(), 'x')],
  ]) {
    const trace = createRunInputRecorder();
    assert.equal(trace.record(...invalid), false);
    assert.equal(trace.complete, false);
    assert.deepEqual(trace.inputs, []);
  }
  function repeatingSymbol() { return 'x'.repeat(129); }
});

test('the next run uses independent state; replay uses a single trusted recording path', () => {
  const first = createRunInputRecorder(1);
  assert.equal(first.record(1, 'entity-pointer', 'entity-1'), true);
  assert.equal(first.record(2, 'pointerup', ''), false);
  const second = createRunInputRecorder(2);
  assert.equal(second.record(0, 'pointer', '1,2'), true);
  assert.equal(second.record(0, 'pointerup', ''), true);
  assert.equal(second.complete, true);
  assert.equal(first.complete, false);
  assert.equal(first.inputs.length, 1);
  assert.equal(second.inputs.length, 2);
});

test('production runtime unifies all event sources and blocks partial leaderboard submission', () => {
  assert.match(runtime, /import \{ createRunInputRecorder \} from '\.\/runtime-replay-input\.mjs'/);
  assert.match(runtime, /const replayInputRecorder = createRunInputRecorder\(\)/);
  assert.match(runtime, /inputLog: replayInputRecorder\.inputs/);
  assert.match(runtime, /function noteInput\(type, detail\) \{\s*replayInputRecorder\.record\(play\.simStep, type, detail \|\| ''\)/);
  assert.match(runtime, /replayInputRecorder\.record\(play\.simStep, 'pointer',/);
  assert.match(runtime, /replayInputRecorder\.record\(play\.simStep, 'pointerup', ''\)/);
  assert.doesNotMatch(runtime, /play\.inputLog\.(?:push|splice|shift)\(/);
  assert.match(runtime, /else if \(play\.shellStarted && replayInputRecorder\.complete\) \{\s*submitRun\(won\)/);
  assert.match(runtime, /else if \(play\.shellStarted && !replayInputRecorder\.complete\) \{/);
  assert.match(runtime, /Not leaderboard-verified: replay input limit reached/);
  assert.match(runtime, /function submitRun\(won\) \{\s*if \(!replayInputRecorder\.complete\) return;/);
});
