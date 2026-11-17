#!/usr/bin/env node
// Epic #202 / atomic #484: exercise the *actual* preinstalled Pixi
// native-message producer in Chromium and WebKit phone/tablet profiles.
// The document-start postMessage sink observes payloads only; it never
// forges a runtime metric, score, probe, document nonce or game rule.
// The inventory package is local real app content, NOT a published creator
// fixture. No physical WKWebView provenance or GPU pixels are claimed.
import assert from 'node:assert/strict';
import { mkdirSync, writeFileSync } from 'node:fs';
import { chromium, webkit } from 'playwright';

const arg = (name, fallback) => {
  const i = process.argv.indexOf('--' + name);
  return i >= 0 ? process.argv[i + 1] : fallback;
};
const base = arg('base', 'http://localhost:18099').replace(/\/$/, '');
const browserName = arg('browser', 'chromium');
assert.ok(['chromium', 'webkit'].includes(browserName));
const shots = arg('shots', '/tmp/runtime-native-v2-first-packet');
mkdirSync(shots, { recursive: true });
const gameId = 'escape-from-injection';
const scenarios = [
  { name: 'iphone-valid-v2', width: 390, height: 844,
    installed: { schema: 2, documentNonce: 'a'.repeat(32) }, expectedSchema: 2 },
  { name: 'ipad-fresh-valid-v2', width: 820, height: 1180,
    installed: { schema: 2, documentNonce: 'b'.repeat(32) }, expectedSchema: 2 },
  { name: 'iphone-malformed-installed', width: 390, height: 844,
    installed: { schema: 2, documentNonce: 'bad' }, expectedSchema: null },
  { name: 'ipad-legacy-no-bridge', width: 820, height: 1180,
    expectedSchema: 1 },
];
const browser = await (browserName === 'webkit' ? webkit : chromium)
  .launch({ headless: true });
const report = [];
try {
  for (const scenario of scenarios) {
    const page = await browser.newPage({
      viewport: { width: scenario.width, height: scenario.height },
      hasTouch: true,
    });
    const pageErrors = [];
    page.on('pageerror', error => pageErrors.push(String(error)));
    try {
      await page.addInitScript(({ installed, hasInstalled }) => {
        // Execute before the shipped runtime JS, just like the native host's
        // document-start bridge installation. No monkey-patching Pixi.
        window.__observedNativePackets = [];
        const sink = {
          postMessage(packet) {
            window.__observedNativePackets.push(
              JSON.parse(JSON.stringify(packet))
            );
          },
        };
        const host = { messageHandlers: { hephaestusRunResult: sink } };
        // WebKit Playwright normally has no real WKScriptMessageHandler.
        // A browser-provided read-only namespace is a failing prerequisite
        // rather than permission to alter the runtime message builder.
        Object.defineProperty(window, 'webkit', {
          value: host, configurable: true, writable: false,
        });
        if (hasInstalled) {
          window.__hephaestusNativeBridge = Object.freeze(installed);
        }
      }, {
        installed: scenario.installed ?? null,
        hasInstalled: Object.hasOwn(scenario, 'installed'),
      });
      const target = base + '/player?game=' + encodeURIComponent(gameId) +
        '&presentation=play&probe=1&seed=17';
      await page.goto(target, { waitUntil: 'load', timeout: 30000 });
      await page.waitForFunction(() => {
        const state = window.__dqTest?.snapshot?.();
        return state?.started === true && state?.frames >= 2 &&
          state?.simStep >= 1;
      }, null, { timeout: 30000 });
      // Give the genuine first renderer-submission metric a chance to be
      // delivered after boot. For malformed bridge, the scene still boots
      // but no native message is allowed; do not insert a fake packet.
      if (scenario.expectedSchema !== null) {
        await page.waitForFunction(() =>
          (window.__observedNativePackets ?? []).some(
            x => x.type === 'player_metric' &&
              x.metric === 'first_scene_render_submit'
          ), null, { timeout: 15000 });
      }
      const result = await page.evaluate(() => ({
        state: window.__dqTest.snapshot(),
        packets: window.__observedNativePackets ?? [],
      }));
      assert.equal(result.state.failed, false);
      assert.equal(result.state.started, true);
      assert.ok(result.state.frames >= 2 && result.state.simStep >= 1);
      assert.ok((await page.locator('#stage canvas').count()) >= 1);
      assert.deepEqual(pageErrors, [], 'real Pixi emitted a page error');

      if (scenario.expectedSchema === null) {
        assert.deepEqual(result.packets, [],
          'PRESENT malformed native v2 bridge must never downgrade to v1');
      } else {
        assert.ok(result.packets.length >= 1,
          'real Pixi must send at least one runtime-owned metric');
        let previousMetricSequence = 0;
        for (const packet of result.packets) {
          assert.ok(['player_metric', 'verified_run_available']
            .includes(packet.type));
          assert.equal(packet.schema, scenario.expectedSchema);
          assert.equal(packet.gameId, gameId);
          assert.match(packet.sessionId, /^[!-~]{1,128}$/);
          if (packet.type === 'player_metric') {
            assert.ok(Number.isSafeInteger(packet.sequence));
            assert.ok(packet.sequence > previousMetricSequence,
              'native metric sequence cannot move backwards');
            previousMetricSequence = packet.sequence;
          }
          if (scenario.expectedSchema === 2) {
            assert.equal(packet.documentNonce,
              scenario.installed.documentNonce,
              'native message must carry the current document-start nonce');
          } else {
            assert.ok(!Object.hasOwn(packet, 'documentNonce'),
              'legacy v1 cannot pretend a v2 document identity');
          }
        }
        const firstScene = result.packets.find(p =>
          p.type === 'player_metric' &&
          p.metric === 'first_scene_render_submit');
        assert.ok(firstScene, 'real Pixi scene was submitted');
        assert.ok(Number.isSafeInteger(firstScene.ms));
        assert.ok(firstScene.ms >= 0 && firstScene.ms <= 3600000);
      }
      const screenshot = shots + '/' + browserName + '-' +
        scenario.name + '.png';
      await page.screenshot({ path: screenshot, fullPage: true });
      report.push({
        scenario: scenario.name, browser: browserName,
        expectedSchema: scenario.expectedSchema,
        observedSchemas: [...new Set(result.packets.map(x => x.schema))],
        observedNativeCount: result.packets.length,
        observedFirstScene: result.packets.some(
          p => p.metric === 'first_scene_render_submit'),
        currentDocumentNonce: scenario.installed?.documentNonce ?? null,
        actualFixedTicks: result.state.simStep,
        actualDisplayFrames: result.state.frames,
        screenshot, passed: true,
      });
    } finally {
      await page.close();
    }
  }
} finally {
  await browser.close();
}
assert.equal(report.length, scenarios.length);
writeFileSync(shots + '/report.json', JSON.stringify({
  schema: 'native-v2-producer-browser-diagnostic-v1',
  browser: browserName, producerOnly: true,
  physicalWKWebView: false, publishedCreatorFixture: false,
  authenticatedNativeMessage: false,
  passed: report.length === scenarios.length, report,
}, null, 2));
console.log(JSON.stringify({
  browser: browserName, passed: report.length === scenarios.length,
  scenarios: report.map(r => ({
    name: r.scenario, expectedSchema: r.expectedSchema,
    packets: r.observedNativeCount, rendered: r.observedFirstScene,
  })),
}));
