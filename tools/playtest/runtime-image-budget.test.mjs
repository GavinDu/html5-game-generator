import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';

const runtime = await readFile(new URL('../../internal/api/web/pixi-runtime.js', import.meta.url), 'utf8');

test('image transfer budget applies to streaming and fallback non-streaming responses', () => {
  assert.match(runtime, /const MAX_IMAGE_TRANSFER_BYTES = 8 \* 1024 \* 1024/);
  assert.match(runtime, /const reader = response\.body\.getReader\(\)/);
  assert.match(runtime, /if \(totalBytes > MAX_IMAGE_TRANSFER_BYTES\)/);
  assert.match(runtime, /await reader\.cancel\(\)\.catch/);
  assert.match(runtime, /if \(blob\.size > MAX_IMAGE_TRANSFER_BYTES\)/);
  assert.match(runtime, /imageBlob = await readBoundedImageBlob\(response\)/);
});

test('keep 15s fetch abort active until entire required image body is consumed', () => {
  const consume = runtime.indexOf('imageBlob = await readBoundedImageBlob(response)');
  const abort = runtime.indexOf('clearTimeout(requestTimeout)', consume);
  assert.ok(consume > 0 && abort > consume, 'timeout must include response transfer');
});

test('decoded dimensions, per-image and session pixels are validated before Pixi texture upload', () => {
  assert.match(runtime, /const MAX_IMAGE_DIMENSION = 4096/);
  assert.match(runtime, /const MAX_IMAGE_PIXELS = 16 \* 1024 \* 1024/);
  assert.match(runtime, /const MAX_TOTAL_IMAGE_PIXELS = 32 \* 1024 \* 1024/);
  assert.match(runtime, /pixels > MAX_IMAGE_PIXELS/);
  assert.match(runtime, /decodedImagePixels \+ pixels > MAX_TOTAL_IMAGE_PIXELS/);
  const check = runtime.indexOf('decodedImagePixels + pixels > MAX_TOTAL_IMAGE_PIXELS');
  const upload = runtime.indexOf('const texture = PIXI.Texture.from(img)', check);
  assert.ok(check > 0 && upload > check, 'bounds must precede GPU allocation');
});
