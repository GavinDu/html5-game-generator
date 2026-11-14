import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { createRunInputRecorder, MAX_VERIFIABLE_RUN_INPUTS,
  createDeadInputDiagnostics, MAX_PENDING_INPUT_DIAGNOSTICS, indexValidatedReplayInputs, replayPointerPhase } from '../../internal/api/web/runtime-replay-input.mjs';

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
  assert.match(runtime, /import \{ createRunInputRecorder, createDeadInputDiagnostics, indexValidatedReplayInputs, replayPointerPhase \} from '\.\/runtime-replay-input\.mjs'/);
  assert.match(runtime, /const replayInputRecorder = createRunInputRecorder\(\)/);
  assert.match(runtime, /inputLog: replayInputRecorder\.inputs/);
  assert.match(runtime, /replayTraceComplete: play \? play\.replayTraceComplete : null/);
  assert.match(runtime, /deadInputRate: play && play\.replayTraceComplete && play\.deadInputDiagnosticsComplete && play\.inputLog\.length > 0/);
  assert.match(runtime, /deadInputDiagnosticsComplete: play \? play\.deadInputDiagnosticsComplete : null/);
  assert.match(runtime, /function noteInput\(type, detail\) \{\s*replayInputRecorder\.record\(play\.simStep, type, detail \|\| ''\)/);
  assert.match(runtime, /replayInputRecorder\.record\(play\.simStep, 'pointer',/);
  assert.match(runtime, /noteInput\('pointerup', ''\)/);
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


test('authoritative recorder rejects backwards fixed ticks but preserves same-tick action order', () => {
  const valid = createRunInputRecorder();
  assert.equal(valid.record(7, 'pointer', '10,20'), true);
  assert.equal(valid.record(7, 'pointerup', ''), true);
  assert.equal(valid.record(8, 'key', 'ArrowLeft'), true);
  assert.deepEqual(valid.inputs.map(event => event.tick), [7, 7, 8]);
  assert.equal(valid.complete, true);

  const invalid = createRunInputRecorder();
  assert.equal(invalid.record(12, 'key', 'ArrowRight'), true);
  assert.equal(invalid.record(11, 'keyup', 'ArrowRight'), false);
  assert.equal(invalid.complete, false);
  assert.equal(invalid.failureReason, 'invalid_event');
  assert.deepEqual(invalid.inputs, [{ tick: 12, type: 'key', detail: 'ArrowRight' }]);
  assert.equal(invalid.record(13, 'pointer', '20,30'), false);
  assert.equal(invalid.inputs.length, 1, 'invalid evidence must remain frozen');
});

test('browser detail budgets match UTF-16 code units at Go admission boundaries', () => {
  const cases = [
    ['empty', '', true],
    ['ascii_exact_128', 'x'.repeat(128), true],
    ['ascii_over_129', 'x'.repeat(129), false],
    ['supplementary_exact_128', '🎮'.repeat(64), true],
    ['supplementary_over_130', '🎮'.repeat(65), false],
    ['mixed_exact_128', 'x'.repeat(126) + '🎮', true],
    ['mixed_over_129', 'x'.repeat(127) + '🎮', false],
  ];
  for (const [name, detail, expected] of cases) {
    const trace = createRunInputRecorder();
    assert.equal(trace.record(10, 'key', detail), expected, name);
    assert.equal(trace.complete, expected, name + ' completeness');
    assert.equal(trace.inputs.length, expected ? 1 : 0, name + ' retained count');
    if (!expected) assert.equal(trace.failureReason, 'invalid_event', name);
  }
});

test('replay pointer event grammar is canonical and fail-closed, while v1 keyboard remains compatible', () => {
  const cases = [
    ['pointer', '0,0', true],
    ['pointer', '-12,1000000', true],
    ['pointer', '1000000,-1000000', true],
    ['pointer', '', false],
    ['pointer', '0', false],
    ['pointer', '1,2,3', false],
    ['pointer', ',2', false],
    ['pointer', 'NaN,0', false],
    ['pointer', 'Infinity,3', false],
    ['pointer', '1.2,3', false],
    ['pointer', '01,2', false],
    ['pointer', '-0,2', false],
    ['pointer', '+1,2', false],
    ['pointer', '1000001,2', false],
    ['pointer', '-1000001,0', false],
    ['pointer', ' 1,2', false],
    ['pointerup', '', true],
    ['pointerup', '1,2', false],
    ['key', 'ArrowLeft', true],
    ['keyup', 'ArrowLeft', true],
    ['key', '', true], // Compatibility until versioned input-binding grammar
  ];
  for (const [type, detail, accepted] of cases) {
    const trace = createRunInputRecorder();
    assert.equal(trace.record(7, type, detail), accepted, type + ':' + detail);
    assert.equal(trace.complete, accepted, type + ':' + detail);
    assert.equal(trace.inputs.length, accepted ? 1 : 0, type + ':' + detail);
    if (!accepted) {
      assert.equal(trace.failureReason, 'invalid_event', type + ':' + detail);
      assert.equal(trace.record(8, 'key', 'ArrowLeft'), false, 'corrupt replay prefix must stay frozen');
    }
  }
});


test('recorded replay playback retains same-tick order and server-compatible bounds', () => {
  const inputs = [
    { tick: 7, type: 'pointer', detail: '-10,1000000' },
    { tick: 7, type: 'pointerup', detail: '' },
    { tick: 8, type: 'key', detail: 'ArrowLeft' },
  ];
  const indexed = indexValidatedReplayInputs(inputs, 1000);
  assert.deepEqual(indexed.byStep.get(7), inputs.slice(0, 2));
  assert.deepEqual(indexed.byStep.get(8), inputs.slice(2));
  assert.equal(indexed.eventCount, 3);
  assert.equal(indexed.lastTick, 8);
  assert.deepEqual(indexValidatedReplayInputs(undefined).byStep.size, 0,
    'historic inputless records may omit the field');
  assert.equal(indexValidatedReplayInputs([]).lastTick, 0);

  const atLimit = Array.from({ length: MAX_VERIFIABLE_RUN_INPUTS }, (_, i) =>
    ({ tick: i, type: 'key', detail: 'ArrowRight' }));
  assert.equal(indexValidatedReplayInputs(atLimit, 35_000).eventCount, 2000);
  assert.throws(() => indexValidatedReplayInputs([...atLimit, atLimit[0]], 35_000),
    /invalid recorded replay input count/);
});

test('stored replay events fail closed before map allocation or silent playback skips', () => {
  const valid = [
    { tick: 1, type: 'pointer', detail: '1,2' },
    { tick: 2, type: 'pointerup', detail: '' },
  ];
  const corruptions = [
    [{ tick: 2, type: 'key', detail: 'ArrowLeft' }, { tick: 1, type: 'keyup', detail: 'ArrowLeft' }],
    [{ tick: -1, type: 'key', detail: 'ArrowLeft' }],
    [{ tick: 1.5, type: 'key', detail: 'ArrowLeft' }],
    [{ tick: Infinity, type: 'key', detail: 'ArrowLeft' }],
    [{ tick: 121, type: 'key', detail: 'ArrowLeft' }], // 1s + 1s allowed slack is tick 120
    [{ tick: 1, type: 'entity-pointer', detail: 'player' }],
    [{ tick: 1, type: 'pointer', detail: '01,2' }],
    [{ tick: 1, type: 'pointer', detail: 'NaN,2' }],
    [{ tick: 1, type: 'pointer', detail: '1000001,2' }],
    [{ tick: 1, type: 'pointerup', detail: '1,2' }],
    [{ tick: 1, type: 'key', detail: 'x'.repeat(129) }],
    [{ tick: 1, type: 'key', detail: undefined }],
    [null],
  ];
  for (const bad of corruptions) {
    assert.throws(() => indexValidatedReplayInputs(bad, 1000),
      /invalid recorded replay event/, JSON.stringify(bad));
  }
  for (const invalid of [true, {}, 'bad', 2001]) {
    assert.throws(() => indexValidatedReplayInputs(invalid), /input count/);
  }
  for (const duration of [-1, 0, 999, 1_800_001, 1.1, NaN, '1000']) {
    assert.throws(() => indexValidatedReplayInputs(valid, duration), /replay duration/);
  }
  assert.equal(indexValidatedReplayInputs(valid, 1000).lastTick, 2);
});

test('shipping replay path fails closed on wrong run identity and malformed stored trace', () => {
  assert.match(runtime, /import \{ createRunInputRecorder, createDeadInputDiagnostics, indexValidatedReplayInputs, replayPointerPhase \} from '\.\/runtime-replay-input\.mjs'/);
  assert.match(runtime, /if \(replayRunId\) \{\s*\/\/ An explicit replay must not silently start/);
  assert.match(runtime, /replayTrace\.id !== replayRunId \|\|\s*replayTrace\.gameId !== id/);
  assert.match(runtime, /replayIndex = indexValidatedReplayInputs\(replayTrace\.inputs, replayTrace\.durationMs\)/);
  assert.match(runtime, /const replayByStep = replayIndex\?\.byStep \?\? new Map\(\)/);
  assert.match(runtime, /const replayLastTick = replayIndex\?\.lastTick \?\? 0/);
  assert.doesNotMatch(runtime, /for \(const inp of replayTrace\.inputs\) \{/);
});


test('replay v1 pointer phase handles down, held drag, fast double-tap and duplicate release', () => {
  const events = ['pointer', 'pointer', 'pointer', 'pointerup', 'pointer',
    'pointerup', 'pointerup', 'pointer', 'pointerup'];
  let held = false;
  const phases = events.map((type) => {
    const phase = replayPointerPhase(type, held);
    if (phase === 'down') held = true;
    if (phase === 'up') held = false;
    return phase;
  });
  assert.deepEqual(phases, [
    'down', 'move', 'move', 'up', 'down', 'up', 'redundant-up', 'down', 'up',
  ]);
  assert.equal(phases.filter((p) => p === 'down').length, 3,
    'five pointer coordinates represent only three physical taps');
  assert.equal(replayPointerPhase('key', false), null);
  assert.equal(held, false);
});

test('production replay shares real input-side effects without synthesizing drag taps', () => {
  assert.match(runtime, /function beginPointer\(x, y\)/);
  assert.match(runtime, /beginPointer\(p\.x, p\.y\);\s*\/\/ Persist the physical down/);
  assert.match(runtime, /const phase = replayPointerPhase\(inp\.type, pointer\.down\)/);
  assert.match(runtime, /if \(phase === 'down'\) \{\s*beginPointer\(x, y\);\s*queueGameInput\(playerEnt, undefined, tapActionName\(\)\)/);
  assert.match(runtime, /Consecutive pointer samples while held are movement, not/);
  assert.match(runtime, /if \(!pointer\.down\) return;\s*pointer\.down = false;\s*\/\/ A fast tap/);
  assert.match(runtime, /function beginKeyboardAction\(b\)/);
  assert.match(runtime, /if \(inp\.type === 'key'\) beginKeyboardAction\(b\)/);
  assert.match(runtime, /noteInput\('key', ev\.key\);\s*beginKeyboardAction\(b\)/);
  assert.match(runtime, /if \(replayTrace \|\| !pointer\.down\) return;/);
  assert.doesNotMatch(runtime, /wasDown = false;/,
    'release evidence is recorded on the physical edge, never deferred to the next frame');
});
