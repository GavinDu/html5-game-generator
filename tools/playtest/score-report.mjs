#!/usr/bin/env node
// Score browser evidence with per-game policies and optional AI review JSON.
// Usage: node score-report.mjs playtest-report.json [ai-review.json|-] [out.json]
import { readFile, readdir, writeFile } from 'node:fs/promises';
import { resolve, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { scoreGame, DEFAULT_RULES } from '../../packages/game-quality-score/index.mjs';

const repoRoot = resolve(fileURLToPath(new URL('../..', import.meta.url)));
const [reportPath, reviewPath, outPath] = process.argv.slice(2);
if (!reportPath) {
  console.error('Usage: node score-report.mjs playtest-report.json [ai-review.json|-] [out.json]');
  process.exit(2);
}
const report = JSON.parse(await readFile(resolve(reportPath), 'utf8'));
const reviews = reviewPath && reviewPath !== '-' ? JSON.parse(await readFile(resolve(reviewPath), 'utf8')) : {};
if (!Array.isArray(report.games)) throw new Error('playtest report must contain a games array');

async function readJsonIfPresent(path) {
  try { return JSON.parse(await readFile(path, 'utf8')); }
  catch (error) { if (error?.code === 'ENOENT') return null; throw error; }
}
async function findGameRules(game) {
  if (game.rulesPath) return JSON.parse(await readFile(resolve(repoRoot, game.rulesPath), 'utf8'));
  const inventoryRoot = resolve(repoRoot, 'inventory');
  const entries = await readdir(inventoryRoot, { withFileTypes: true });
  const id = game.gameId || game.id;
  for (const entry of entries.filter(e => e.isDirectory())) {
    const gameDir = join(inventoryRoot, entry.name);
    const manifest = await readJsonIfPresent(join(gameDir, 'manifest.json'));
    if (!manifest || (manifest.game_id !== id && manifest.title !== game.title)) continue;
    const rules = await readJsonIfPresent(join(gameDir, 'quality-rules.json'));
    if (rules) return rules;
  }
  return (await readJsonIfPresent(resolve(repoRoot, 'config/game-quality/default.json'))) || DEFAULT_RULES;
}
for (const game of report.games) {
  const rules = await findGameRules(game);
  const review = reviews[game.gameId] || reviews[game.title] || null;
  game.quality = scoreGame(game.checks || [], review, rules);
  game.quality.rulesFile = rules.profile === 'default'
    ? 'config/game-quality/default.json'
    : `inventory/<game-id>/quality-rules.json`;
}
const output = JSON.stringify(report, null, 2) + '\n';
if (outPath) await writeFile(resolve(outPath), output);
else process.stdout.write(output);
