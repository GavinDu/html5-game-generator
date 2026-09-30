// Headless playtest for "飘带补天 Ribbon Mends the Sky".
// Stubs the GameShim v1 runtime, loads the real game.js, and drives it with
// a lawnmower tap bot through the genuine Shim.input('tap') path.
// Asserts: no exceptions, music started, all 6 holes stitched, Shim.end won.
//
// Usage: node verify.js   (run from this folder)
'use strict';
const fs = require('fs');
const path = require('path');

const code = fs.readFileSync(path.join(__dirname, 'game.js'), 'utf8');

let tapCb = null;
let loopFn = null;
let endResult = null;
const tones = [];
let musicSrc = null;
const texts = {};
const sprites = [];

const Shim = {
  init(o) { this._init = o; },
  sprite(id, o) {
    o = o || {};
    const s = { id, x: o.x || 0, y: o.y || 0, w: o.w || 64, h: o.h || 64, visible: o.visible !== false };
    sprites.push(s);
    return s;
  },
  text(id, str, o) { texts[id] = String(str); },
  input(verb, cb) { if (verb === 'tap') tapCb = cb; },
  audio: {
    music(src) { musicSrc = src; },
    tone(o) { tones.push(o); },
  },
  loop(fn) { loopFn = fn; },
  end(res) { endResult = res; },
};

new Function('Shim', code)(Shim);

if (!tapCb || !loopFn) { console.error('FAIL: tap/loop not registered'); process.exit(1); }
if (musicSrc !== 'audio/theme.wav') { console.error('FAIL: music not started: ' + musicSrc); process.exit(1); }

// Lawnmower bot: sweep taps across the play area; stitching needs the hero
// within 60px of a hole, bonking needs the tap within 65px of a demon.
const xs = [80, 160, 240, 320];
const ys = [200, 350, 500, 650];
let tapIdx = 0;
const dt = 1 / 60;
let frames = 0;
const maxFrames = 60 * 240; // 4 simulated minutes

while (!endResult && frames < maxFrames) {
  if (frames % 20 === 0) {
    const x = xs[tapIdx % xs.length];
    const y = ys[Math.floor(tapIdx / xs.length) % ys.length];
    tapIdx++;
    tapCb(x, y);
  }
  loopFn(dt);
  frames++;
}

const fails = [];
if (!endResult) fails.push('Shim.end never called within ' + (frames / 60) + 's sim');
if (endResult && endResult.won !== true) fails.push('end.won !== true: ' + JSON.stringify(endResult));
if (!/6\/6/.test(texts.score || '')) fails.push('score never reached 6/6: ' + texts.score);
if (tones.length < 6) fails.push('too few tones: ' + tones.length);
if (fails.length) { console.error('FAIL: ' + fails.join('; ')); process.exit(1); }

console.log('PASS: won in ' + (frames / 60).toFixed(1) + 's sim, score="' + texts.score +
  '", tones=' + tones.length + ', sprites=' + sprites.length);
