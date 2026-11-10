#!/usr/bin/env node
import { mkdirSync, writeFileSync } from 'node:fs';
import { chromium, webkit } from 'playwright';
import { PNG } from 'pngjs';
import { evaluateFeedPreviewReadiness, hasCompleteViewportCoverage } from './feed-media-readiness.mjs';
import { evaluateSettledPager } from './feed-pager-geometry.mjs';

const arg = (key, fallback) => { const i = process.argv.indexOf('--' + key); return i >= 0 ? (process.argv[i + 1] || fallback) : fallback; };
const base = arg('base', '').replace(/\/$/, '');
const shots = arg('shots', '/tmp/feed-ui-evaluator');
const browserName = arg('browser', 'chromium');
if (!base || !['chromium', 'webkit'].includes(browserName)) throw new Error('--base and a supported --browser are required');
mkdirSync(shots, { recursive: true });
const browser = await (browserName === 'webkit' ? webkit : chromium).launch({ headless: true });
const reports = [];
try {
  for (const [name, width, height] of [['iphone-portrait', 390, 844], ['iphone-landscape', 844, 390], ['ipad-portrait', 820, 1180], ['ipad-landscape', 1180, 820]]) {
    const context = await browser.newContext({ viewport: { width, height }, hasTouch: true, isMobile: name.startsWith('iphone') });
    const page = await context.newPage();
    const errors = [];
    page.on('pageerror', e => errors.push(e.message));
    try {
      await page.goto(base + '/player', { waitUntil: 'networkidle' });
      const cards = page.locator('.game-card');
      await cards.first().waitFor();
      if (await cards.count() < 2) throw new Error('Two real feed games are required to verify paging');
      const metrics = await page.evaluate(() => {
        const feed = document.getElementById('list');
        const card = feed.querySelector('.game-card');
        const a = feed.getBoundingClientRect(), b = card.getBoundingClientRect();
        const targets = [...card.querySelectorAll('button, a.play-link')].map(el => {
          const r = el.getBoundingClientRect();
          return { name: el.getAttribute('aria-label') || el.textContent, x: r.x, y: r.y, width: r.width, height: r.height };
        });
        const actionRail = card.querySelector('.social-actions')?.getBoundingClientRect();
        const playLink = card.querySelector('a.play-link')?.getBoundingClientRect();
        const overlaps = (x, y) => x && y && x.left < y.right && x.right > y.left && x.top < y.bottom && x.bottom > y.top;
        return {
          feedHeight: a.height, cardHeight: b.height,
          viewport: { width: innerWidth, height: innerHeight },
          scrollWidth: document.body.scrollWidth, targets,
          rightRail: actionRail ? { left: actionRail.left, right: actionRail.right, top: actionRail.top, bottom: actionRail.bottom } : null,
          railRightOfCenter: !!actionRail && actionRail.left > b.left + b.width / 2,
          railInsideCard: !!actionRail && actionRail.right <= b.right + 1 && actionRail.top >= b.top - 1,
          railDoesNotCoverPlay: !!actionRail && !!playLink && !overlaps(actionRail, playLink),
        };
      });
      // A card must show either real, successfully loaded gameplay artwork
      // or an explicit no-preview placeholder. An empty gradient is failure.
      const previewSurface = () => page.evaluate(() => {
        const feed = document.getElementById('list');
        const mid = feed.getBoundingClientRect().top + feed.clientHeight / 2;
        const card = [...feed.querySelectorAll('.game-card')].find(el => {
          const r = el.getBoundingClientRect();
          return r.top <= mid && r.bottom > mid;
        });
        if (!card) return { valid: false, type: 'missing-card' };
        const img = card.querySelector('img.feed-poster');
        if (img && img.complete && img.naturalWidth > 0) {
          return { valid: true, type: 'verified-poster' };
        }
        const fallback = card.querySelector('.feed-preview-absent');
        return {
          valid: !!fallback &&
            fallback.textContent.includes('Gameplay preview coming soon') &&
            fallback.textContent.includes('Play now'),
          type: fallback ? 'honest-placeholder' : 'unexplained-blank',
        };
      });
      // Regressions can retain the correct card height and still reveal a
      // neighboring title below the settled page. Inspect the real scrollport
      // before and after the swipe, not the outer app/window geometry.
      const pagerGeometry = async () => {
        const rects = await page.evaluate(() => {
          const feed = document.getElementById('list');
          const view = feed.getBoundingClientRect();
          return {
            viewport: { top: view.top, bottom: view.bottom },
            cards: [...feed.querySelectorAll('.game-card')].map(el => {
              const r = el.getBoundingClientRect();
              return { gameID: el.dataset.gameId, top: r.top, bottom: r.bottom };
            }),
          };
        });
        return evaluateSettledPager(rects.viewport, rects.cards);
      };
      const beforeGeometry = await pagerGeometry();
      const beforePreview = await previewSurface();
      const before = await page.screenshot({ path: shots + '/' + name + '-before.png' });
      const firstID = await cards.first().getAttribute('data-game-id');
      const feed = page.locator('#list');
      // Chromium's CDP dispatches trusted touchscreen input, rather than
      // treating mouse wheel/keyboard paging as evidence of a mobile swipe.
      // Playwright WebKit does not expose native swipe injection; keep its
      // accessible alternatives explicit in the report, not mislabeled.
      const inputMethod = browserName === 'chromium' ? 'native-touch-swipe'
        : name.startsWith('iphone') ? 'keyboard-alternative' : 'wheel-alternative';
      if (inputMethod === 'native-touch-swipe') {
        const bounds = await feed.boundingBox();
        if (!bounds || bounds.height < 100) throw new Error('touch swipe requires a visible feed');
        const x = bounds.x + bounds.width * .5;
        const startY = bounds.y + bounds.height * .79;
        const endY = bounds.y + bounds.height * .17;
        const cdp = await context.newCDPSession(page);
        try {
          await cdp.send('Input.dispatchTouchEvent', {
            type: 'touchStart', touchPoints: [{ x, y: startY, id: 1 }]
          });
          for (let step = 1; step <= 12; step++) {
            const y = startY + (endY - startY) * (step / 12);
            await cdp.send('Input.dispatchTouchEvent', {
              type: 'touchMove', touchPoints: [{ x, y, id: 1 }]
            });
            await page.waitForTimeout(18);
          }
          await cdp.send('Input.dispatchTouchEvent', {
            type: 'touchEnd', touchPoints: []
          });
        } finally {
          await cdp.detach();
        }
      } else if (inputMethod === 'keyboard-alternative') {
        await feed.focus();
        await page.keyboard.press('ArrowDown');
      } else {
        await feed.hover();
        await page.mouse.wheel(0, metrics.feedHeight);
      }
      await page.waitForTimeout(1200);
      const afterGeometry = await pagerGeometry();
      const afterID = afterGeometry.visibleCount === 1 ? afterGeometry.visibleGameIDs[0] : null;
      const afterPreview = await previewSurface();
      const after = await page.screenshot({ path: shots + '/' + name + '-after.png' });
      const a = PNG.sync.read(before), b = PNG.sync.read(after);
      let changed = 0;
      for (let i = 0; i < a.data.length; i += 4) {
        if (Math.abs(a.data[i] - b.data[i]) + Math.abs(a.data[i + 1] - b.data[i + 1]) + Math.abs(a.data[i + 2] - b.data[i + 2]) > 36) changed++;
      }
      const changedPixelRatio = changed / (a.width * a.height);
      const checks = {
        // A rotated or mislabeled screenshot cannot certify this profile.
        screenshotDimensions: a.width === width && a.height === height &&
          b.width === width && b.height === height,
        onePostPerViewport: Math.abs(metrics.feedHeight - metrics.cardHeight) <= 2,
        oneSettledPostPerPager: beforeGeometry.aligned && afterGeometry.aligned,
        noHorizontalOverflow: metrics.scrollWidth <= width + 1,
        touchTargets: metrics.targets.length >= 4 && metrics.targets.every(t => t.width >= 44 && t.height >= 44 && t.x >= 0 && t.x + t.width <= width + 1 && t.y >= 0 && t.y + t.height <= height + 1),
        socialActionRail: metrics.railRightOfCenter && metrics.railInsideCard && metrics.railDoesNotCoverPlay,
        pagingAdvances: !!afterID && firstID !== afterID,
        screenshotChanged: changedPixelRatio > 0.005,
        noErrors: errors.length === 0,
        previewSurfaceLabeled: beforePreview.valid && afterPreview.valid,
      };
      // The browser player has a feed exit; the native host keeps its own
      // controls because it never sends entry=feed.
      const visibleCard = page.locator(`.game-card[data-game-id="${afterID}"]`);
      const playURL = new URL(await visibleCard.locator('.play-link').getAttribute('href'), base);
      checks.immersivePlayLink = playURL.searchParams.get('presentation') === 'play' && playURL.searchParams.get('entry') === 'feed';
      reports.push({ viewport: name, passed: Object.values(checks).every(Boolean), checks, metrics, firstID, afterID, beforeGeometry, afterGeometry, beforePreview, afterPreview, changedPixelRatio, inputMethod, errors });
    } catch (error) { reports.push({ viewport: name, passed: false, error: error.message, errors }); }
    finally { await context.close(); }
  }
} finally { await browser.close(); }
const functionalPass = hasCompleteViewportCoverage(reports) && reports.every(r => r.passed);
const mediaReadiness = evaluateFeedPreviewReadiness(reports);
const report = { browser: browserName, passed: functionalPass, mediaReadiness, viewports: reports };
console.log(JSON.stringify({ browser: browserName, functionalPass, mediaReadiness }));
writeFileSync(shots + '/report.json', JSON.stringify(report, null, 2));
console.log(JSON.stringify(report));
if (!report.passed) process.exitCode = 1;
