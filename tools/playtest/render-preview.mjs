#!/usr/bin/env node
// Render a muted preview clip and poster from a recorded deterministic run.
// Usage:
//   node render-preview.mjs --base http://localhost:8080 --game GAME \
//     --run RUN_ID --out /tmp/preview.mp4 --poster /tmp/poster.png
import { chromium, webkit } from 'playwright';
import { mkdirSync, rmSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { spawn } from 'node:child_process';

function arg(name, fallback = '') {
  const index = process.argv.indexOf(`--${name}`);
  return index >= 0 && process.argv[index + 1] ? process.argv[index + 1] : fallback;
}

const base = arg('base').replace(/\/$/, '');
const game = arg('game');
const run = arg('run');
const out = arg('out');
const poster = arg('poster');
const browserName = arg('browser', 'chromium');
const durationMs = Math.max(1, Number(arg('duration-ms', '6000')));
const fps = Math.max(1, Math.min(30, Number(arg('fps', '12'))));
const startDelayMs = Math.max(0, Number(arg('start-delay-ms', '0')));

if (!base || !game || !run || !out || !poster) {
  console.error('usage: render-preview.mjs --base URL --game ID --run RUN_ID --out CLIP --poster PNG');
  process.exit(2);
}
if (!['chromium', 'webkit'].includes(browserName)) {
  console.error('render-preview.mjs: --browser must be chromium or webkit');
  process.exit(2);
}

const frameDir = `${out}.frames`;
mkdirSync(frameDir, { recursive: true });
mkdirSync(dirname(out), { recursive: true });
mkdirSync(dirname(poster), { recursive: true });
const frameCount = Math.max(1, Math.ceil(durationMs * fps / 1000));
const frameInterval = 1000 / fps;
let browser;
let usedFallback = false;

function runFFmpeg(args) {
  return new Promise((resolve, reject) => {
    const child = spawn('ffmpeg', args, { stdio: ['ignore', 'ignore', 'pipe'] });
    let stderr = '';
    child.stderr.on('data', (chunk) => { stderr += chunk.toString(); });
    child.on('error', reject);
    child.on('close', (code) => code === 0 ? resolve() : reject(new Error(stderr.slice(-1000))));
  });
}

try {
  const launch = browserName === 'webkit' ? webkit : chromium;
  browser = await launch.launch({ headless: true });
  const page = await browser.newPage({ viewport: { width: 420, height: 900 }, deviceScaleFactor: 1 });
  const url = `${base}/player?game=${encodeURIComponent(game)}&probe=1&replay=${encodeURIComponent(run)}`;
  await page.goto(url, { waitUntil: 'load', timeout: 30000 });
  await page.waitForFunction(() => window.__dqProbe?.booted === true, null, { timeout: 30000 });
  if (startDelayMs) await page.waitForTimeout(startDelayMs);

  for (let index = 0; index < frameCount; index += 1) {
    const framePath = join(frameDir, `frame-${String(index).padStart(5, '0')}.png`);
    await page.screenshot({ path: framePath });
    await page.waitForTimeout(frameInterval);
  }
  const peakFrame = join(frameDir, `frame-${String(Math.floor(frameCount / 2)).padStart(5, '0')}.png`);
  writeFileSync(poster, await (await import('node:fs/promises')).readFile(peakFrame));

  try {
    await runFFmpeg([
      '-y', '-framerate', String(fps), '-i', join(frameDir, 'frame-%05d.png'),
      '-an', '-c:v', 'libx264', '-pix_fmt', 'yuv420p', '-movflags', '+faststart', out,
    ]);
  } catch (error) {
    usedFallback = true;
    console.error(`render-preview.mjs: clip encode failed; poster retained: ${error.message}`);
  }

  const result = {
    game,
    run,
    duration_ms: durationMs,
    frames: frameCount,
    fps,
    browser: browserName,
    clip: out,
    poster,
    clip_fallback: usedFallback,
  };
  console.log(JSON.stringify(result));
} catch (error) {
  console.error(`render-preview.mjs: ${error.stack || error}`);
  process.exitCode = 1;
} finally {
  await browser?.close();
  rmSync(frameDir, { recursive: true, force: true });
}
