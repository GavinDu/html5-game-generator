import assert from 'node:assert/strict';
import { Writable } from 'node:stream';
import test from 'node:test';
import { flushReportJSON } from './report-output.mjs';

test('large worker report is complete even when the pipe drains asynchronously', async () => {
  const parts = [];
  let writes = 0;
  const pipe = new Writable({
    highWaterMark: 8,
    write(chunk, encoding, callback) {
      writes++;
      // Exercise delayed completion instead of trusting synchronous stdout.
      setImmediate(() => { parts.push(Buffer.from(chunk)); callback(); });
    }
  });
  const report = { base: 'http://localhost:18099', browser: 'webkit',
    games: [{ gameId: 'g1', verdict: 'PASS', inputTrace:
      Array.from({ length: 24000 }, (_, i) => ({ tick: i, type: 'pointer', detail: '300,720' })) }] };
  await flushReportJSON(pipe, report);
  assert.ok(writes > 0);
  assert.deepEqual(JSON.parse(Buffer.concat(parts).toString('utf8')), report);
});

test('stream write errors fail closed; no bogus PASS report is produced', async () => {
  const pipe = new Writable({
    write(_chunk, _encoding, callback) { callback(new Error('broken stdout pipe')); }
  });
  pipe.on('error', () => {});
  await assert.rejects(flushReportJSON(pipe, { games: [{ verdict: 'PASS' }] }), /broken stdout pipe/);
});
