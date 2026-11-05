import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';

const playerPath = fileURLToPath(new URL('../../internal/api/web/player.html', import.meta.url));
const html = await readFile(playerPath, 'utf8');

test('player shell declares viewport and safe-area support', () => {
  assert.match(html, /viewport-fit=cover/);
  assert.match(html, /env\(safe-area-inset-top\)/);
  assert.match(html, /env\(safe-area-inset-bottom\)/);
  assert.match(html, /min-height:\s*100dvh/);
});

test('player shell preserves responsive gameplay sizing and touch input', () => {
  assert.match(html, /@media \(min-width:\s*760px\)/);
  assert.match(html, /@media \(orientation:\s*landscape\)/);
  assert.match(html, /max-height:\s*min\(72dvh,\s*900px\)/);
  assert.match(html, /touch-action:\s*none/);
  assert.doesNotMatch(html, /user-scalable=no/);
});

test('player shell has accessible focus and reduced-motion fallbacks', () => {
  assert.match(html, /:focus-visible/);
  assert.match(html, /prefers-reduced-motion:\s*reduce/);
  assert.match(html, /outline:\s*3px solid/);
});


test('start and result overlays expose dialog names and descriptions', () => {
  assert.match(html, /id="shell-start"[^>]*role="dialog"[^>]*aria-modal="true"/);
  assert.match(html, /aria-labelledby="shell-start-title"/);
  assert.match(html, /aria-describedby="shell-start-hint"/);
  assert.match(html, /id="shell-result"[^>]*role="dialog"[^>]*aria-modal="true"/);
  assert.match(html, /aria-labelledby="shell-result-title"/);
  assert.match(html, /aria-describedby="shell-result-sub"/);
});

test('score changes are announced politely without making the whole page live', () => {
  assert.match(html, /id="score" aria-live="polite" aria-atomic="true"/);
  assert.doesNotMatch(html, /<body[^>]*aria-live=/);
});


test('tap-controlled catcher playtests send discrete taps instead of drag-only gestures', async () => {
  const probePath = fileURLToPath(new URL('./probe.mjs', import.meta.url));
  const runtimePath = fileURLToPath(new URL('../../internal/api/web/pixi-runtime.js', import.meta.url));
  const [probe, runtime] = await Promise.all([
    readFile(probePath, 'utf8'),
    readFile(runtimePath, 'utf8')
  ]);
  assert.match(runtime, /controlsScheme: play\.shell\.controls\?\.scheme/);
  assert.match(probe, /\['tap', 'tap_x'\]\.includes\(probe\?\.shell\?\.controlsScheme\)/);
  assert.match(probe, /page\.touchscreen\.tap\(target\.x, target\.y\)/);
  assert.match(probe, /hasTouch: true/);\n  assert.match(probe, /fallback\.width \* \(0\.08 \+ \(\(ki % 10\) \* 0\.093\)\)/);
});


test('playtest honors declared auto-start and tap-to-start shell modes', async () => {
  const probePath = fileURLToPath(new URL('./probe.mjs', import.meta.url));
  const runtimePath = fileURLToPath(new URL('../../internal/api/web/pixi-runtime.js', import.meta.url));
  const [probe, runtime] = await Promise.all([
    readFile(probePath, 'utf8'),
    readFile(runtimePath, 'utf8')
  ]);
  assert.match(runtime, /startMode: play\.shell\.start\?\.mode \|\| 'auto'/);
  assert.match(probe, /startMode === 'tap_to_start'/);
  assert.match(probe, /auto-start mode began immediately without a blocking start card/);
  assert.match(probe, /\['tap', 'tap_x'\]\.includes\(probe\?\.shell\?\.controlsScheme\)/);
});
