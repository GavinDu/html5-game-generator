import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
const source = await readFile(new URL('../../internal/api/web/pixi-runtime.js', import.meta.url), 'utf8');

test('active-entity cap protects scene startup before Pixi application creation', () => {
  assert.match(source, /const MAX_ACTIVE_ENTITIES = 512/);
  const validation = source.indexOf("scene exceeds active entity budget before renderer initialization");
  const gpu = source.indexOf('const app = new PIXI.Application()');
  assert.ok(validation > 0 && validation < gpu,
    'fail package preflight before GPU/WASM allocations');
});

test('runtime spawn cap prevents partial Pixi/Rapier allocations', () => {
  const create = source.indexOf('function createEntity(def, x, y, runtimeId)');
  const limit = source.indexOf('entities.size >= MAX_ACTIVE_ENTITIES', create);
  const view = source.indexOf('const view = makeView(def)', create);
  const body = source.indexOf('const { body, collider } = makeBody(def, x, y)', create);
  assert.ok(create > 0 && limit > create && limit < view && view < body);
  assert.match(source, /active entity budget exceeded during spawn/);
});

test('tick exception stops simulation instead of submitting success after resource exhaustion', () => {
  assert.match(source, /recordError\('tick', err\)/);
  assert.match(source, /Game cannot continue safely\. Return to the feed or retry\./);
  assert.match(source, /try \{ app\.ticker\.stop\(\); \} catch \{ \/\* ticker may already have stopped \*\/ \}/);
});
