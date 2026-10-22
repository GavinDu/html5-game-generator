#!/usr/bin/env node
// Merge an evidence-backed AI review into a browser playtest report.
// Usage: node score-report.mjs playtest-report.json [ai-review.json] [out.json]
import { readFile, writeFile } from 'node:fs/promises';
import { scoreGame } from '../../packages/game-quality-score/index.mjs';

const repoRoot = resolve(fileURLToPath(new URL('../..', import.meta.url)));
const [reportPath, reviewPath, outPath] = process.argv.slice(2);
if (!reportPath) {
  console.error('Usage: node score-report.mjs playtest-report.json [ai-review.json] [out.json]');
  process.exit(2);
}
const report = JSON.parse(await readFile(reportPath, 'utf8'));
const reviews = reviewPath ? JSON.parse(await readFile(reviewPath, 'utf8')) : {};
if (!Array.isArray(report.games)) throw new Error('playtest report must contain a games array');
for (const game of report.games) {
  game.quality = scoreGame(game.checks || [], reviews[game.gameId] || null);
}
const output = JSON.stringify(report, null, 2) + '\n';
if (outPath) await writeFile(outPath, output);
else process.stdout.write(output);
