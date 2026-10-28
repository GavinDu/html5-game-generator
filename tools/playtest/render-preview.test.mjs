import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';

const sourcePath = fileURLToPath(new URL('./render-preview.mjs', import.meta.url));
const source = await readFile(sourcePath, 'utf8');

test('preview renderer keeps a poster when replay/rendering fails after browser launch', () => {
  assert.match(source, /replay\/render failed; using poster-only fallback/);
  assert.match(source, /await page\.screenshot\(\{ path: poster, fullPage: false \}\)/);
  assert.match(source, /clip: null, poster, clip_fallback: true/);
});

test('poster fallback labels itself and escapes game titles as HTML', () => {
  assert.match(source, /Preview clip unavailable/);
  assert.match(source, /function escapeHTML\(value\)/);
  assert.match(source, /'&': '&amp;'/);
});

test('renderer still reports an error when no browser can produce any poster', () => {
  assert.match(source, /if \(fallbackWritten\)/);
  assert.match(source, /process\.exitCode = 1/);
});
