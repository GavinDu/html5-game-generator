import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';

const source = await readFile(new URL('../../internal/api/web/pixi-runtime.js', import.meta.url), 'utf8');
const goSource = await readFile(new URL('../../internal/gameir/ir.go', import.meta.url), 'utf8');

test('Go compiler and iOS-hosted WebKit interpreter share exact capability registry', () => {
  const goList = goSource.match(/capabilityKinds\s*=\s*enum\(([^)]*)\)/)?.[1];
  const webList = source.match(/const supportedCapabilities = new Set\(\[([^]*?)\]\);/)?.[1];
  assert.ok(goList, 'missing Go allowlist');
  assert.ok(webList, 'missing runtime fail-closed allowlist');
  const goCapabilities = [...goList.matchAll(/"([^"]+)"/g)].map(x => x[1]).sort();
  const webCapabilities = [...webList.matchAll(/'([^']+)'/g)].map(x => x[1]).sort();
  assert.deepEqual(webCapabilities, goCapabilities, 'compiler/runtime capability lists differ');
});

test('capability negotiation occurs before scene construction and GPU allocation', () => {
  const registry = source.indexOf("const supportedCapabilities = new Set([");
  const scene = source.indexOf("const sceneKey = 'dq.scene.' + id;");
  const gpu = source.indexOf('const app = new PIXI.Application();');
  assert.ok(registry > 0 && scene > registry && gpu > scene);
  assert.match(source, /!supportedCapabilities\.has\(capability\)/);
  assert.match(source, /checkedCapabilities\.has\(capability\)/);
  assert.match(source, /declaredCapabilities\.length > 32/);
  assert.match(source, /ir\.irVersion === '1\.2' && !checkedCapabilities\.has\('shell\.v1'\)/);
  assert.match(source, /unsupported or duplicate Game IR capability/);
});
