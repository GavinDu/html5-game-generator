import test from 'node:test';
import assert from 'node:assert/strict';
import { readBoundedSuccessfulJSON } from '../../internal/api/web/runtime-replay-package-guard.mjs';

const utf8 = (value) => new TextEncoder().encode(value);
const success = (body, headers = {}) =>
  new Response(body, { status: 200, headers });

function streamed(bytes, chunkBytes, onCancel = () => {}) {
  let offset = 0;
  return new ReadableStream({
    pull(controller) {
      if (offset >= bytes.byteLength) {
        controller.close();
        return;
      }
      const end = Math.min(bytes.byteLength, offset + chunkBytes);
      controller.enqueue(bytes.slice(offset, end));
      offset = end;
    },
    cancel() { onCancel(); },
  });
}

test('160 actual fragmented decoded JSON success bodies honor exact byte limit', async () => {
  const expected = { gameId: 'public-test', title: '企鹅 🐧 rainbow', n: 123 };
  const bytes = utf8(JSON.stringify(expected));
  for (let chunkSize = 1; chunkSize <= 160; chunkSize++) {
    const parsed = await readBoundedSuccessfulJSON(
      success(streamed(bytes, chunkSize), { 'content-length': String(bytes.length) }),
      { maxBytes: bytes.length },
    );
    assert.deepEqual(parsed, expected);
  }
});

test('announced and cumulative decoded sizes reject at limit plus one without body leakage', async () => {
  const secret = 'owner-private-token-credentials-NOT-FOR-LOGS';
  const bytes = utf8(JSON.stringify({ privateGame: secret }));
  let canceled = false;
  const premature = success(
    streamed(bytes, 1, () => { canceled = true; }),
    { 'content-length': String(bytes.length) },
  );
  await assert.rejects(
    readBoundedSuccessfulJSON(premature, { maxBytes: bytes.length - 1 }),
    /runtime document too large/,
  );
  assert.equal(canceled, true);
  let overflowCanceled = false;
  const forced = success(new ReadableStream({
    start(controller) { controller.enqueue(bytes); },
    cancel() { overflowCanceled = true; },
  }));
  await assert.rejects(
    readBoundedSuccessfulJSON(forced, { maxBytes: bytes.length - 1 }),
    (err) => err.message === 'runtime document too large' && !err.message.includes(secret),
  );
  assert.equal(overflowCanceled, true);
});

test('identity Content-Length cannot forge a shorter or longer successful body', async () => {
  const bytes = utf8('{"ok":true}');
  for (const announced of [bytes.length - 1, bytes.length + 1]) {
    await assert.rejects(
      readBoundedSuccessfulJSON(
        success(streamed(bytes, 2), { 'content-length': String(announced) }),
        { maxBytes: bytes.length + 10 },
      ),
      /runtime document declared size mismatch/,
    );
  }
  for (const announced of ['garbage', '-1', '999999999999999999999999999999999']) {
    await assert.rejects(
      readBoundedSuccessfulJSON(
        success(streamed(bytes, 2), { 'content-length': announced }),
        { maxBytes: bytes.length + 10 },
      ),
      /runtime document invalid declared size/,
    );
  }
});

test('a compressed-wire Content-Length is not the decoded JSON length', async () => {
  const bytes = utf8('{"result":"ok"}');
  const parsed = await readBoundedSuccessfulJSON(
    success(streamed(bytes, 1), {
      'content-encoding': 'gzip',
      'content-length': '1',
    }),
    { maxBytes: bytes.length },
  );
  assert.deepEqual(parsed, { result: 'ok' });
});

test('missing stream never falls back to unbounded json/text/blob/arrayBuffer', async () => {
  let unboundedCalled = false;
  const fake = {
    ok: true,
    headers: new Headers(),
    body: null,
    async json() { unboundedCalled = true; return {}; },
    async text() { unboundedCalled = true; return '{}'; },
    async arrayBuffer() { unboundedCalled = true; return new ArrayBuffer(1); },
  };
  await assert.rejects(
    readBoundedSuccessfulJSON(fake, { maxBytes: 32 }),
    /runtime document streaming unavailable/,
  );
  assert.equal(unboundedCalled, false);
});

test('empty, nonpositive or invalid limits reject safely without parsing', async () => {
  for (const maxBytes of [undefined, null, -1, 0, 1.2, Infinity, NaN, '100']) {
    await assert.rejects(
      readBoundedSuccessfulJSON(success(streamed(utf8('{}'), 2)), { maxBytes }),
      /runtime document invalid byte limit/,
    );
  }
  await assert.rejects(
    readBoundedSuccessfulJSON(success(streamed(utf8(''), 1)), { maxBytes: 10 }),
    /runtime document empty/,
  );
});

test('malformed JSON and invalid UTF-8 never expose private response fragments', async () => {
  const secret = 'private-user-authentication-secret';
  for (const bytes of [utf8('{"password":"' + secret), new Uint8Array([0xc3, 0x28])]) {
    await assert.rejects(
      readBoundedSuccessfulJSON(success(streamed(bytes, 1)), { maxBytes: 200 }),
      (err) => err.message === 'runtime document invalid JSON' &&
        !err.message.includes(secret) && !err.message.includes('password'),
    );
  }
});

test('run-owned AbortSignal cancels a slow in-flight reader and rejects stale success', async () => {
  let wasCanceled = false;
  const abort = new AbortController();
  const body = new ReadableStream({
    start(controller) {
      controller.enqueue(utf8('{"partial":'));
      // Remain pending until the owning signal cancels the document.
    },
    cancel() { wasCanceled = true; },
  });
  const pending = readBoundedSuccessfulJSON(success(body), {
    maxBytes: 400,
    signal: abort.signal,
  });
  await Promise.resolve();
  abort.abort();
  await assert.rejects(pending, /runtime document aborted/);
  assert.equal(wasCanceled, true);
});

test('invalid stream chunks, non-success responses and already aborted signals fail closed', async () => {
  await assert.rejects(
    readBoundedSuccessfulJSON({ ok: false }, { maxBytes: 200 }),
    /runtime document unavailable/,
  );
  await assert.rejects(
    readBoundedSuccessfulJSON({
      ok: true, headers: new Headers(),
      body: { getReader() { return {
        async read() { return { done: false, value: 'untrusted owner secret' }; },
        async cancel() {}, releaseLock() {},
      }; } },
    }, { maxBytes: 200 }),
    /runtime document stream failed/,
  );
  const abort = new AbortController();
  abort.abort();
  await assert.rejects(
    readBoundedSuccessfulJSON(success(streamed(utf8('{}'), 1)), {
      maxBytes: 2, signal: abort.signal,
    }),
    /runtime document aborted/,
  );
});
