import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';

const source = await readFile(new URL('../../internal/api/web/pixi-runtime.js', import.meta.url), 'utf8');
test('render error is rejected before first-scene and player-ready telemetry', () => {
  const stamp = source.indexOf('const firstSceneRenderSubmitMs =');
  const rendered = source.lastIndexOf('app.renderer.render(app.stage);', stamp);
  const gate = source.indexOf('if (renderErrLogged) {', rendered);
  const ready = source.indexOf("sendSocialDiagnostic('player_ready', 'ready'", stamp);
  assert.ok(stamp > 0 && rendered > 0 && gate > rendered && gate < stamp && ready > stamp);
  assert.match(source, /throw new Error\('initial scene render failed; gameplay is not ready'\)/);
});
