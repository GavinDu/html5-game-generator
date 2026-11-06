#!/usr/bin/env node
// Browser contract test: real feed UI, intercepted social API, no account writes.
import assert from 'node:assert/strict';
import { chromium, webkit } from 'playwright';
const arg = (key, fallback) => { const i = process.argv.indexOf('--' + key); return i >= 0 ? process.argv[i + 1] : fallback; };
const base = arg('base', 'http://localhost:18099');
const engine = arg('browser', 'chromium') === 'webkit' ? webkit : chromium;
const browser = await engine.launch({ headless: true });
try {
  const page = await browser.newPage({ viewport: { width: 390, height: 844 } });
  const calls = [];
  const title = '<img src=x onerror=alert(1)> A drawing';
  await page.route('**/v1/feed?*', route => route.fulfill({ json: {
    games: [{ game_id: 'private', title, owner_id: 'creator' }, { game_id: 'public', title: 'A public expression', owner_id: 'creator' }],
    cards: ['private', 'public'].map(id => ({ game_id: id, creator: { display_name: 'Creator' }, preview: {}, viewer: { liked: false, saved: false }, playable_expression: { source_type: 'drawing', source_visibility: id, intent: id === 'private' ? 'PRIVATE_SOURCE_SECRET' : 'Help a friend', chosen_interpretation: 'Collect hearts' } })),
  } }));
  await page.route('**/v1/games/*/reactions', async route => {
    calls.push({ method: route.request().method(), body: route.request().postDataJSON() });
    await route.fulfill({ status: route.request().headers().authorization === 'Bearer fixture' ? 200 : 401, json: {} });
  });
  await page.route('**/v1/games/*/save', async route => {
    calls.push({ method: route.request().method(), save: true });
    await route.fulfill({ json: {} });
  });
  await page.goto(base + '/player');
  const card = page.locator('.game-card').first();
  await card.waitFor();
  assert.equal(await card.locator('.game-title').textContent(), title);
  assert.equal(await card.locator('.game-title img').count(), 0, 'feed text must not become HTML');
  assert.equal(await page.getByText('PRIVATE_SOURCE_SECRET').count(), 0, 'private intent must not enter the DOM');
  assert.equal(await card.locator('.expression-details').count(), 0);
  const like = card.getByRole('button', { name: 'Like game' });
  await like.click();
  await page.getByRole('status').filter({ hasText: 'Sign in to like' }).first().waitFor();
  assert.equal(await like.getAttribute('aria-pressed'), 'false', 'rejected mutation must not change state');
  await page.evaluate(() => sessionStorage.setItem('dq.session.bearer', 'fixture'));
  await like.click();
  await page.waitForFunction(() => document.querySelector('.social-actions button').getAttribute('aria-pressed') === 'true');
  await like.click();
  await page.waitForFunction(() => document.querySelector('.social-actions button').getAttribute('aria-pressed') === 'false');
  assert.deepEqual(calls.slice(0, 3).map(c => c.method), ['POST', 'POST', 'DELETE']);
  assert.deepEqual(calls[1].body, { reaction: 'like' });
  const save = card.getByRole('button', { name: 'Save game' });
  await save.click();
  await page.waitForFunction(() => document.querySelectorAll('.social-actions button')[1].getAttribute('aria-pressed') === 'true');
  await save.click();
  await page.waitForFunction(() => document.querySelectorAll('.social-actions button')[1].getAttribute('aria-pressed') === 'false');
  assert.deepEqual(calls.filter(c => c.save).map(c => c.method), ['POST', 'DELETE']);
  const publicCard = page.locator('.game-card').nth(1);
  await publicCard.scrollIntoViewIfNeeded();
  await publicCard.locator('summary').click();
  assert.equal(await publicCard.getByText('Creator intent: Help a friend').isVisible(), true);
  assert.equal(await publicCard.getByText('Game interpretation: Collect hearts').isVisible(), true);
  console.log(JSON.stringify({ passed: true, checks: ['safe-text', 'private-source-hidden', 'public-source-labeled', 'rejected-like-preserves-state', 'like-post-delete', 'save-post-delete'] }));
} finally { await browser.close(); }
