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


// Reusable decoded-success transport primitive, not yet wired to package and
// replay document fetches. Those call sites require a separately ratified
// SERVER-owned per-route byte policy (#700/#701). A caller must supply that
// limit: never reject real published games using a guessed fixed cutoff.
// Fetch streams deliver DECOMPRESSED bytes; Content-Length can describe a
// smaller/different compressed wire representation and is not authoritative.
export async function readBoundedSuccessfulJSON(response, { maxBytes, signal } = {}) {
  const failure = (code) => new Error('runtime document ' + code);
  if (!Number.isSafeInteger(maxBytes) || maxBytes <= 0) {
    throw failure('invalid byte limit');
  }
  if (!response || response.ok !== true) {
    throw failure('unavailable');
  }
  if (signal?.aborted) throw failure('aborted');
  // No fallback to response.json/text/blob/arrayBuffer: those may allocate
  // unlimited untrusted HTTP-200 bodies on older WebKit versions.
  let reader;
  try {
    reader = response.body?.getReader?.();
  } catch {
    throw failure('streaming unavailable');
  }
  if (!reader || typeof reader.read !== 'function' ||
      typeof reader.cancel !== 'function') {
    throw failure('streaming unavailable');
  }
  let completed = false;
  const onAbort = () => {
    try { void Promise.resolve(reader.cancel()).catch(() => {}); }
    catch { /* teardown cannot reveal private response bytes */ }
  };
  try {
    signal?.addEventListener?.('abort', onAbort, { once: true });
    if (signal?.aborted) throw failure('aborted');

    const announced = response.headers?.get?.('content-length');
    const encoding = (response.headers?.get?.('content-encoding') || '')
      .trim().toLowerCase();
    let declared = null;
    if (announced != null && String(announced).trim() !== '') {
      const value = String(announced).trim();
      if (!/^[0-9]+$/.test(value)) throw failure('invalid declared size');
      declared = Number(value);
      if (!Number.isSafeInteger(declared)) {
        throw failure('invalid declared size');
      }
      // A compressed HTTP wire count is not the decoded-stream length.
      if ((!encoding || encoding === 'identity') && declared > maxBytes) {
        throw failure('too large');
      }
    }

    let total = 0;
    const chunks = [];
    while (true) {
      if (signal?.aborted) throw failure('aborted');
      let next;
      try {
        next = await reader.read();
      } catch {
        throw failure(signal?.aborted ? 'aborted' : 'stream failed');
      }
      if (signal?.aborted) throw failure('aborted');
      if (!next || typeof next.done !== 'boolean') {
        throw failure('stream failed');
      }
      if (next.done) break;
      const value = next.value;
      if (!(value instanceof Uint8Array)) throw failure('stream failed');
      if (value.byteLength > maxBytes - total) {
        throw failure('too large');
      }
      if (value.byteLength === 0) continue;
      total += value.byteLength;
      chunks.push(value);
    }
    if (total === 0) throw failure('empty');
    if ((!encoding || encoding === 'identity') &&
        declared !== null && declared !== total) {
      throw failure('declared size mismatch');
    }
    const bytes = new Uint8Array(total);
    let offset = 0;
    for (const chunk of chunks) {
      bytes.set(chunk, offset);
      offset += chunk.byteLength;
    }
    let document;
    try {
      document = JSON.parse(new TextDecoder('utf-8', { fatal: true }).decode(bytes));
    } catch {
      // Browser SyntaxError / TextDecoder messages can include private body
      // fragments or URLs; translate them to a fixed safe category.
      throw failure('invalid JSON');
    }
    completed = true;
    return document;
  } finally {
    signal?.removeEventListener?.('abort', onAbort);
    if (!completed) {
      try { void Promise.resolve(reader.cancel()).catch(() => {}); }
      catch { /* already closed */ }
    }
    try { reader.releaseLock?.(); } catch { /* already canceled */ }
  }
}
