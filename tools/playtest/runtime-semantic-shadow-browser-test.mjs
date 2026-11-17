#!/usr/bin/env node
// Epic #202/#486: real unmodified Pixi vs preinstalled *pure* scalar
// reducer SHADOW CONFORMANCE. This is neither production reducer wiring,
// source-authenticated published Game IR nor an attested score.
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { chromium, webkit } from 'playwright';

const arg = (name, fallback) => {
  const i = process.argv.indexOf('--' + name);
  return i >= 0 ? process.argv[i + 1] : fallback;
};
const base = arg('base', 'http://localhost:18099').replace(/\/$/, '');
const browserName = arg('browser', 'chromium');
assert.ok(['chromium', 'webkit'].includes(browserName));
const shots = arg('shots', '/tmp/runtime-semantic-shadow');
const gameId = 'escape-from-injection', seed = 17;
mkdirSync(shots, { recursive: true });

const fixtureBytes = readFileSync(new URL(
  '../../internal/attestation/testdata/start_score_ir_v1.json', import.meta.url
));
const fixtureSHA256 = createHash('sha256').update(fixtureBytes).digest('hex');
// The Go test recomputes this immutable artifact from the typed Go reducer.
// This browser test must not invent a second independently hardcoded score.
const goGolden = JSON.parse(readFileSync(new URL(
  './fixtures/runtime-start-score-go-pixi-golden-v1.json', import.meta.url
), 'utf8'));
assert.equal(goGolden.schema, 'runtime-start-score-go-pixi-golden-v1');
assert.equal(goGolden.fixtureSha256, fixtureSHA256);
assert.equal(goGolden.goProfile, 'go-scalar-start-score-v1');
assert.match(goGolden.goTraceSha256, /^[0-9a-f]{64}$/);
assert.equal(goGolden.attested, false);
assert.equal(goGolden.finalScore, 1);
assert.equal(goGolden.appliedActions, goGolden.steps.length);
const ir = JSON.parse(fixtureBytes.toString('utf8'));
assert.equal(ir.irVersion, '1.0');
assert.equal(ir.game.id, gameId);
assert.deepEqual(ir.capabilities, []);
assert.equal(ir.scenes.length, 1);
assert.deepEqual(ir.scenes[0].entities, []);
assert.deepEqual(ir.inputBindings, []);
assert.deepEqual(ir.rules.map(rule =>
  [rule.id, rule.when.event, rule.actions.map(action => [action.type, action.value])]
), [
  ['start_plus', 'game_start', [['add_score', 3]]],
  ['start_minus', 'game_start', [['add_score', -2]]],
]);
const commands = ir.rules.flatMap(rule => rule.actions)
  .map((action, ordinal) => ({
    ordinal, action: { type: action.type, value: action.value },
  }));
assert.deepEqual(goGolden.steps.map(step => ({
  ordinal: step.ordinal,
  ruleId: step.ruleId,
  ruleIndex: step.ruleIndex,
  actionIndex: step.actionIndex,
  type: 'add_score',
  value: step.delta,
})), ir.rules.flatMap((rule, ruleIndex) => rule.actions.map(
  (action, actionIndex) => ({
    ordinal: ruleIndex + actionIndex,
    ruleId: rule.id, ruleIndex, actionIndex,
    type: action.type, value: action.value,
  })
)), 'Go typed intermediate steps must reflect the authored IR action order');
const browser = await (browserName === 'webkit' ? webkit : chromium)
  .launch({ headless: true });
const results = [];
try {
  for (const device of [
    { name: 'iphone-portrait', width: 390, height: 844 },
    { name: 'ipad-portrait', width: 820, height: 1180 },
  ]) {
    for (const quality of ['low', 'high']) {
      const page = await browser.newPage({
        viewport: { width: device.width, height: device.height },
        hasTouch: true,
      });
      const errors = [];
      let intercepts = 0;
      page.on('pageerror', error => errors.push(String(error)));
      try {
        await page.route('**/v1/games/*/package', async route => {
          const upstream = await route.fetch();
          assert.equal(upstream.status(), 200);
          const pkg = await upstream.json();
          assert.equal(pkg.gameId, gameId);
          pkg.ir = structuredClone(ir);
          pkg.irVersion = ir.irVersion;
          pkg.orientation = ir.game.orientation;
          pkg.assets = ir.assets;
          // This Playwright-only synthetic declarative IR is not the
          // immutable published package served by the upstream API.
          pkg.irHash = '';
          pkg.irSha256 = '';
          intercepts++;
          await route.fulfill({ response: upstream, json: pkg });
        });
        const target = base + '/player?game=' + encodeURIComponent(gameId) +
          '&presentation=play&quality=' + quality +
          '&probe=1&probeStartupRules=1&seed=' + seed;
        const cycles = quality === 'low' ? 2 : 1;
        for (let cycle = 0; cycle < cycles; cycle++) {
          if (cycle === 0) await page.goto(target, { waitUntil: 'load', timeout: 30000 });
          else await page.reload({ waitUntil: 'load', timeout: 30000 });
          await page.waitForFunction(() => {
            const s = window.__dqTest?.snapshot?.();
            return s?.started === true && s?.frames >= 2 && s?.simStep >= 1;
          }, null, { timeout: 30000 });
          const compared = await page.evaluate(async ({ seed, commands }) => {
            const {
              createSemanticState, reduceSemanticStateTick,
              canonicalSemanticState,
            } = await import('/web/runtime-semantic-state-reducer.mjs');
            const initial = createSemanticState({ seed });
            const result = reduceSemanticStateTick(initial, 1, commands);
            const repeated = reduceSemanticStateTick(
              createSemanticState({ seed }), 1, commands
            );
            let invalidFractionRejected = false, invalidTextRejected = false;
            for (const [value, mark] of [[1.5, 'fraction'], ['3', 'text']]) {
              try {
                reduceSemanticStateTick(initial, 1, [
                  { ordinal: 0, action: { type: 'add_score', value } },
                ]);
              } catch {
                if (mark === 'fraction') invalidFractionRejected = true;
                else invalidTextRejected = true;
              }
            }
            const reversed = reduceSemanticStateTick(
              initial, 1, [
                { ordinal: 0, action: commands[1].action },
                { ordinal: 1, action: commands[0].action },
              ]
            );
            return {
              pixi: window.__dqTest.snapshot(),
              pure: {
                score: result.state.score,
                initialScore: initial.score,
                canonical: canonicalSemanticState(result.state),
                repeatedCanonical: canonicalSemanticState(repeated.state),
                effects: result.effects.map(e => ({ ...e })),
                reversedScore: reversed.state.score,
                reversedFirstEffect: reversed.effects[0].value,
                invalidFractionRejected, invalidTextRejected,
              },
            };
          }, { seed, commands });
          assert.equal(compared.pixi.failed, false);
          assert.equal(compared.pixi.started, true);
          assert.equal(compared.pixi.startupScoreTraceComplete, true,
            'real Pixi game_start trace may never be silently truncated');
          assert.deepEqual(compared.pixi.startupScoreTrace, goGolden.steps,
            'ACTUAL Pixi add_score intermediate states must match each Go typed ordinal and score');
          assert.equal(compared.pixi.score, goGolden.finalScore,
            'unmodified Pixi score must match the independently computed Go ledger final score');
          assert.equal(compared.pixi.ended, false);
          assert.equal(compared.pixi.won, false);
          assert.ok(compared.pixi.simStep >= 1 && compared.pixi.frames >= 2);
          assert.ok((await page.locator('#stage canvas').count()) >= 1);
          assert.equal(compared.pixi.startupScoreTrace.at(-1).score, compared.pixi.score);
          assert.equal(await page.evaluate(() => {
            // Changing a snapshot copy never mutates the true game state.
            const tampered = window.__dqTest.snapshot();
            tampered.startupScoreTrace[0].score = 999;
            return window.__dqTest.snapshot().startupScoreTrace[0].score;
          }), goGolden.steps[0].score);
          assert.equal(compared.pure.score, compared.pixi.score,
            'pure ordered startup score must equal genuine Pixi score');
          assert.equal(compared.pure.initialScore, 0);
          assert.equal(compared.pure.canonical, compared.pure.repeatedCanonical);
          assert.deepEqual(compared.pure.effects.map(e => e.value),
            goGolden.steps.map(step => step.score),
            'preinstalled JS pure intermediate effects must match every Go authored score state');
          assert.equal(compared.pure.reversedScore, 1);
          assert.equal(compared.pure.reversedFirstEffect,
            goGolden.steps.at(-1).delta,
            'commutative final score must not hide authored intermediate order');
          assert.equal(compared.pure.invalidFractionRejected, true);
          assert.equal(compared.pure.invalidTextRejected, true);
          const screenshot = shots + '/' + browserName + '-' +
            device.name + '-' + quality + '-' + cycle + '.png';
          await page.screenshot({ path: screenshot, fullPage: true });
          results.push({
            browser: browserName, device: device.name, quality, cycle,
            fixtureSHA256, goProfile: goGolden.goProfile,
            goTraceSHA256: goGolden.goTraceSha256,
            pixiScore: compared.pixi.score,
            pixiStartupScoreTrace: compared.pixi.startupScoreTrace,
            pixiStartupScoreTraceComplete: compared.pixi.startupScoreTraceComplete,
            pixiStepObserved: compared.pixi.simStep,
            pixiFramesObserved: compared.pixi.frames,
            pureScore: compared.pure.score,
            pureOrderedScoreEffects: compared.pure.effects.map(e => e.value),
            pureCanonical: compared.pure.canonical,
            screenshot, passed: true,
          });
        }
        // The optional probe must not leak intermediate action internals to
        // ordinary probe users without the explicit startup trace opt-in.
        let additionalNavigations = 0;
        if (quality === 'low') {
          await page.goto(target.replace('&probeStartupRules=1', ''),
            { waitUntil: 'load', timeout: 30000 });
          await page.waitForFunction(() =>
            window.__dqTest?.snapshot?.()?.started === true, null, { timeout: 30000 });
          const noOptIn = await page.evaluate(() => window.__dqTest.snapshot());
          assert.equal(Object.hasOwn(noOptIn, 'startupScoreTrace'), false);
          assert.equal(Object.hasOwn(noOptIn, 'startupScoreTraceComplete'), false);
          additionalNavigations++;

          // Non-probe playback must never receive the diagnostic snapshot
          // or any additional JS/native bridge merely by playing the game.
          await page.goto(target.replace('&probe=1&probeStartupRules=1', ''),
            { waitUntil: 'load', timeout: 30000 });
          assert.equal(await page.evaluate(() => window.__dqTest === undefined), true);
          additionalNavigations++;
        }
        assert.equal(intercepts, cycles + additionalNavigations,
          'every new game document must fetch the actual route once');
        assert.deepEqual(errors, [], 'unmodified Pixi emitted page errors');
      } finally {
        await page.close();
      }
    }
  }
} finally {
  await browser.close();
}
assert.equal(results.length, 6);
writeFileSync(shots + '/report.json', JSON.stringify({
  schema: 'synthetic-pixi-pure-reducer-shadow-v1',
  fixtureSHA256, sourceGameIR: 'test-only intercepted repository JSON',
  sharedGoGolden: true, goProfile: goGolden.goProfile,
  goTraceSHA256: goGolden.goTraceSha256,
  fullGameReplay: false, publishedPackageAuthenticated: false,
  intermediatePixiStateObserved: false, independentAttestation: false,
  physicalIOSDevice: false,
  browser: browserName, passed: true, results,
}, null, 2));
console.log(JSON.stringify({
  browser: browserName, passed: true, cases: results.length,
  score: goGolden.finalScore, fixtureSHA256,
  goTraceSHA256: goGolden.goTraceSha256,
  semanticPureOnly: true,
}));
