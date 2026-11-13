import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { runInNewContext } from 'node:vm';

// Exercise the actual installed runtime's probe timer setup, not a test-only
// reimplementation. A probe is a diagnostic; it must never own a game after
// Play Again tears down its AbortController.
const source = readFileSync(new URL('../../internal/api/web/pixi-runtime.js', import.meta.url), 'utf8');
const begin = source.indexOf('  if (probeMode && !abort.signal.aborted) {\n    emitProbe();\n    // Probe polling is session-owned.');
const end = source.indexOf('  // Playability Kit hook', begin);
assert.ok(begin >= 0 && end > begin, 'trusted runtime probe installer was not found');
const installer = source.slice(begin, end);

test('abort cancels the real probe interval and 64 in-page restarts do not accumulate timers', () => {
  let nextID = 1;
  const timers = new Map();
  let starts = 0;
  let stops = 0;
  for (let generation = 0; generation < 64; generation++) {
    const abort = new AbortController();
    let reports = 0;
    runInNewContext(installer, {
      probeMode: true,
      abort,
      emitProbe() { reports++; },
      setInterval(callback, ms) {
        assert.equal(ms, 2000);
        const id = nextID++;
        timers.set(id, callback);
        starts++;
        return id;
      },
      clearInterval(id) {
        assert.ok(timers.delete(id), 'interval must be cleared exactly once');
        stops++;
      },
    });
    assert.equal(reports, 1, 'initial diagnostic emits immediately');
    assert.equal(timers.size, 1, 'exactly one interval per live session');
    [...timers.values()][0]();
    assert.equal(reports, 2, 'the live session can still report diagnostics');
    abort.abort();
    abort.abort();
    assert.equal(timers.size, 0, 'old diagnostic cannot survive restart');
  }
  assert.equal(starts, 64);
  assert.equal(stops, 64);
});

test('already-aborted async bootstrap cannot resurrect a retired probe interval', () => {
  const abort = new AbortController();
  abort.abort();
  runInNewContext(installer, {
    probeMode: true,
    abort,
    emitProbe() { assert.fail('old boot cannot emit current readiness'); },
    setInterval() { assert.fail('old boot cannot create a timer'); },
    clearInterval() { assert.fail('no timer must be created'); },
  });
});

test('normal non-probe gameplay does not schedule diagnostics', () => {
  const abort = new AbortController();
  runInNewContext(installer, {
    probeMode: false,
    abort,
    emitProbe() { assert.fail('not a probe session'); },
    setInterval() { assert.fail('unexpected interval'); },
    clearInterval() { assert.fail('unexpected interval teardown'); },
  });
});

test('stale probe snapshot is removed only when still owned by aborted session', () => {
  assert.match(source, /const probeSessionAPI = \{/);
  assert.match(source, /window\.__dqTest = probeSessionAPI;/);
  assert.match(source, /if \(window\.__dqTest === probeSessionAPI\) delete window\.__dqTest;/);
  assert.match(source, /abort\.signal\.addEventListener\('abort', \(\) => \{[\s\S]*?window\.__dqTest === probeSessionAPI/);
});
