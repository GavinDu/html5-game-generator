#!/usr/bin/env node
// Responsive player UI evaluator. Opens one real generated game at phone,
// phone-landscape, iPad portrait, and iPad landscape viewport sizes.
import { mkdirSync } from 'node:fs';
import { chromium, webkit } from 'playwright';

function arg(name, fallback = '') {
  const index = process.argv.indexOf(`--${name}`);
  return index >= 0 && process.argv[index + 1] ? process.argv[index + 1] : fallback;
}

const base = arg('base').replace(/\/$/, '');
const game = arg('game');
const browserName = arg('browser', 'chromium');
const shots = arg('shots', '/tmp/ui-evaluator');
if (!base || !game || !['chromium', 'webkit'].includes(browserName)) {
  console.error('usage: ui-evaluator.mjs --base URL --game ID [--browser chromium|webkit] [--shots DIR]');
  process.exit(2);
}

const viewports = [
  { name: 'iphone-portrait', width: 390, height: 844, deviceScaleFactor: 2, isTablet: false },
  { name: 'iphone-landscape', width: 844, height: 390, deviceScaleFactor: 2, isTablet: false },
  { name: 'ipad-portrait', width: 820, height: 1180, deviceScaleFactor: 2, isTablet: true },
  { name: 'ipad-landscape', width: 1180, height: 820, deviceScaleFactor: 2, isTablet: true },
];

mkdirSync(shots, { recursive: true });
const engine = browserName === 'webkit' ? webkit : chromium;
const browser = await engine.launch({ headless: true });
const results = [];
try {
  for (const viewport of viewports) {
    const context = await browser.newContext({
      viewport: { width: viewport.width, height: viewport.height },
      deviceScaleFactor: viewport.deviceScaleFactor,
      isMobile: !viewport.isTablet,
      hasTouch: true,
    });
    const page = await context.newPage();
    const errors = [];
    page.on('pageerror', error => errors.push(error.message));
    try {
      const url = `${base}/player?game=${encodeURIComponent(game)}&presentation=play&probe=1`;
      await page.goto(url, { waitUntil: 'load', timeout: 30000 });
      await page.waitForFunction(() => window.__dqProbe?.booted === true, null, { timeout: 30000 });
      await page.waitForSelector('#stage canvas', { timeout: 15000 });
      await page.waitForTimeout(300);
      const metrics = await page.evaluate(() => {
        const canvas = document.querySelector('#stage canvas');
        const stage = document.getElementById('stage');
        const wrap = document.getElementById('stage-wrap');
        const rect = canvas?.getBoundingClientRect();
        const stageRect = stage?.getBoundingClientRect();
        const wrapRect = wrap?.getBoundingClientRect();
        const viewportWidth = document.documentElement.clientWidth;
        const viewportHeight = document.documentElement.clientHeight;
        const visible = !!rect && rect.width > 0 && rect.height > 0 &&
          rect.left >= -1 && rect.top >= -1 &&
          rect.right <= viewportWidth + 1 && rect.bottom <= viewportHeight + 1;
        const shellButtons = [...document.querySelectorAll('.shell-card.on button')];
        return {
          viewport: { width: viewportWidth, height: viewportHeight },
          canvas: rect ? { x: rect.x, y: rect.y, width: rect.width, height: rect.height } : null,
          stage: stageRect ? { x: stageRect.x, y: stageRect.y, width: stageRect.width, height: stageRect.height } : null,
          wrap: wrapRect ? { x: wrapRect.x, y: wrapRect.y, width: wrapRect.width, height: wrapRect.height } : null,
          body: { scrollWidth: document.body.scrollWidth, scrollHeight: document.body.scrollHeight },
          overflowElements: [...document.querySelectorAll('body *')].map(element => {
            const box = element.getBoundingClientRect();
            return {
              element: element.id ? '#' + element.id : element.tagName.toLowerCase(),
              className: typeof element.className === 'string' ? element.className : '',
              left: Math.round(box.left),
              right: Math.round(box.right),
              width: Math.round(box.width),
              scrollWidth: element.scrollWidth,
              clientWidth: element.clientWidth
            };
          }).filter(item => item.right > viewportWidth + 1 || item.left < -1 || item.scrollWidth > item.clientWidth + 1)
            .sort((a, b) => Math.max(b.right - viewportWidth, b.scrollWidth - b.clientWidth) - Math.max(a.right - viewportWidth, a.scrollWidth - a.clientWidth))
            .slice(0, 12),
          visible,
          bodyMode: document.body.dataset.presentation || 'feed',
          canvasAspect: rect ? rect.width / rect.height : 0,
          logicalCanvas: canvas ? { width: canvas.width, height: canvas.height } : null,
          shellButtons: shellButtons.map(button => {
            const box = button.getBoundingClientRect();
            return { label: button.textContent.trim(), width: box.width, height: box.height };
          }),
          probe: window.__dqProbe ? { booted: window.__dqProbe.booted, errors: window.__dqProbe.errors?.length || 0 } : null,
        };
      });
      const checks = [
        { id: 'player-presentation-active', pass: metrics.bodyMode === 'play', actual: metrics.bodyMode },
        { id: 'canvas-visible', pass: metrics.visible, actual: metrics.canvas },
        { id: 'no-horizontal-overflow', pass: metrics.body.scrollWidth <= viewport.width + 1, actual: metrics.body.scrollWidth },
        { id: 'no-vertical-overflow', pass: metrics.body.scrollHeight <= viewport.height + 1, actual: metrics.body.scrollHeight },
        { id: 'canvas-rendered', pass: !!metrics.logicalCanvas && metrics.logicalCanvas.width > 0 && metrics.logicalCanvas.height > 0, actual: metrics.logicalCanvas },
        { id: 'no-page-errors', pass: errors.length === 0 && metrics.probe?.errors === 0, actual: errors },
        { id: 'shell-controls-present', pass: metrics.shellButtons.length > 0, actual: metrics.shellButtons.length },
        { id: 'shell-controls-touch-sized', pass: metrics.shellButtons.length > 0 && metrics.shellButtons.every(button => button.width >= 44 && button.height >= 44), actual: metrics.shellButtons },
      ];
      await page.screenshot({ path: `${shots}/${viewport.name}.png`, fullPage: true });
      const failed = checks.filter(check => !check.pass);
      results.push({ viewport: viewport.name, passed: failed.length === 0, metrics, checks });
      console.log(JSON.stringify({ viewport: viewport.name, passed: failed.length === 0, failed }));
    } catch (error) {
      results.push({ viewport: viewport.name, passed: false, error: String(error.message || error), pageErrors: errors });
      console.error(`ui-evaluator: ${viewport.name}: ${error.stack || error}`);
    } finally {
      await context.close();
    }
  }
} finally {
  await browser.close();
}
const report = { game, browser: browserName, viewports: results, passed: results.every(result => result.passed) };
console.log(JSON.stringify(report));
if (!report.passed) process.exitCode = 1;
