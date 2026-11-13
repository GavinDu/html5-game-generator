import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { createRunInputRecorder, MAX_VERIFIABLE_RUN_INPUTS,
  createDeadInputDiagnostics, MAX_PENDING_INPUT_DIAGNOSTICS } from '../../internal/api/web/runtime-replay-input.mjs';

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
  assert.equal(trace.failureReason, 'input_limit');
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
    [0, 'pointer', 'x'.repeat(129)],
  ]) {
    const trace = createRunInputRecorder();
    assert.equal(trace.record(...invalid), false);
    assert.equal(trace.complete, false);
    assert.equal(trace.failureReason, 'invalid_event');
    assert.deepEqual(trace.inputs, []);
  }
  for (const type of ['fake', 'entity-pointer']) {
    const trace = createRunInputRecorder();
    assert.equal(trace.record(0, type, 'entity-1'), false);
    assert.equal(trace.failureReason, 'unsupported_input');
    assert.equal(trace.complete, false);
    assert.deepEqual(trace.inputs, []);
  }
});

test('the next run uses independent state; replay uses a single trusted recording path', () => {
  const first = createRunInputRecorder(1);
  assert.equal(first.record(1, 'key', 'ArrowLeft'), true);
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
  assert.match(runtime, /import \{ createRunInputRecorder, createDeadInputDiagnostics \} from '\.\/runtime-replay-input\.mjs'/);
  assert.match(runtime, /const replayInputRecorder = createRunInputRecorder\(\)/);
  assert.match(runtime, /inputLog: replayInputRecorder\.inputs/);
  assert.match(runtime, /replayTraceComplete: play \? play\.replayTraceComplete : null/);
  assert.match(runtime, /deadInputRate: play && play\.replayTraceComplete && play\.deadInputDiagnosticsComplete && play\.inputLog\.length > 0/);
  assert.match(runtime, /deadInputDiagnosticsComplete: play \? play\.deadInputDiagnosticsComplete : null/);
  assert.match(runtime, /function noteInput\(type, detail\) \{\s*replayInputRecorder\.record\(play\.simStep, type, detail \|\| ''\)/);
  assert.match(runtime, /replayInputRecorder\.record\(play\.simStep, 'pointer',/);
  assert.match(runtime, /replayInputRecorder\.record\(play\.simStep, 'pointerup', ''\)/);
  assert.doesNotMatch(runtime, /play\.inputLog\.(?:push|splice|shift)\(/);
  assert.match(runtime, /else if \(play\.shellStarted && replayInputRecorder\.complete\) \{\s*submitRun\(won\)/);
  assert.match(runtime, /else if \(play\.shellStarted && !replayInputRecorder\.complete\) \{/);
  assert.match(runtime, /Not leaderboard-verified: /);
  assert.match(runtime, /'replay input limit reached' : 'unsupported replay input'/);
  assert.match(runtime, /function submitRun\(won\) \{\s*if \(!replayInputRecorder\.complete\) return;/);
});


test('bounded diagnostic queue keeps original 200ms dead-input accounting on valid traces', () => {
  const monitor = createDeadInputDiagnostics();
  assert.equal(MAX_PENDING_INPUT_DIAGNOSTICS, 256);
  assert.equal(monitor.record({ at: 10, x: -1, y: -1 }), true);
  assert.equal(monitor.record({ at: 20, x: -1, y: -1 }), true);
  assert.equal(monitor.drain(209, 0), 0, 'first sample has not yet aged 200 ms');
  assert.equal(monitor.drain(210, 0), 1);
  assert.equal(monitor.drain(220, 15), 1);
  assert.equal(monitor.pending.length, 0);
  assert.equal(monitor.complete, true);
  assert.equal(monitor.record({ at: 221, x: 2, y: 1 }), true);
  assert.equal(monitor.drain(430, 222), 0, 'movement newer than sample prevents false dead input');
});

test('diagnostic overflow or invalid time becomes explicitly incomplete without affecting gameplay', () => {
  for (const limit of [0, -1, 257, Infinity, NaN, 1.5]) {
    assert.throws(() => createDeadInputDiagnostics(limit), RangeError);
  }
  const monitor = createDeadInputDiagnostics();
  for (let i = 0; i < MAX_PENDING_INPUT_DIAGNOSTICS; i++) {
    assert.equal(monitor.record({ at: i, x: 0, y: 0 }), true);
  }
  assert.equal(monitor.pending.length, 256);
  assert.equal(monitor.record({ at: 256, x: 0, y: 0 }), false);
  assert.equal(monitor.complete, false);
  assert.equal(monitor.pending.length, 0, 'do not retain stale diagnostic backlog');
  assert.equal(monitor.record({ at: 257, x: 0, y: 0 }), false);
  assert.equal(monitor.drain(1000, 0), 0);
  const nextGame = createDeadInputDiagnostics();
  assert.equal(nextGame.complete, true);
  assert.equal(nextGame.record({ at: 0, x: 0, y: 0 }), true);
  const invalid = createDeadInputDiagnostics();
  assert.equal(invalid.record({ at: Infinity }), false);
  assert.equal(invalid.complete, false);
});

test('shipping runtime never mutates diagnostic queue except through pure bounded monitor', () => {
  assert.match(runtime, /const deadInputDiagnostics = createDeadInputDiagnostics\(\)/);
  assert.match(runtime, /pendingInputs: deadInputDiagnostics\.pending/);
  assert.match(runtime, /deadInputDiagnostics\.record\(\{ at: play\.visibleMs/);
  assert.match(runtime, /play\.deadInputs \+= deadInputDiagnostics\.drain\(play\.visibleMs, play\.lastActivityMs\)/);
  assert.doesNotMatch(runtime, /play\.pendingInputs\.(?:push|shift|splice)\(/);
});
