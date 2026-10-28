#!/usr/bin/env node
import { readFile } from 'node:fs/promises';
import { compareFrameHashes } from './frame-hash-comparison.mjs';

function arg(name) {
  const index = process.argv.indexOf(`--${name}`);
  return index >= 0 ? process.argv[index + 1] : '';
}

const firstPath = arg('first');
const secondPath = arg('second');
if (!firstPath || !secondPath) {
  console.error('usage: node compare-preview-hashes.mjs --first render-a.json --second render-b.json');
  process.exit(2);
}

try {
  const [first, second] = await Promise.all([
    readFile(firstPath, 'utf8').then(JSON.parse),
    readFile(secondPath, 'utf8').then(JSON.parse),
  ]);
  const result = compareFrameHashes(first.frame_sha256, second.frame_sha256);
  console.log(JSON.stringify(result));
  if (!result.match) process.exitCode = 1;
} catch (error) {
  console.error(`compare-preview-hashes.mjs: ${error.message}`);
  process.exitCode = 2;
}
