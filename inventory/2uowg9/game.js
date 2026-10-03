// Escape from the Injection — GameShim v1
//
// From a kid's drawing and handwritten plot:
//   1. "Inject in the butt"         — the blob is about to get a shot
//   2. "escape further and further" — flee through new places, faster each time
//   3. "finally, he is caught and Injected with tears"
//
// You are the blob. Tap the LEFT / RIGHT side of the screen to switch lanes.
// Dodge the syringes, grab courage stars, and keep ahead of the chaser.
// Every 40 seconds you escape somewhere new and everything gets quicker.
// Get caught and it's a shot in the butt... with tears.

Shim.init({ width: 390, height: 844, background: '#d8f3e3' });

var LANES = [90, 195, 300];
var LEVEL_SECS = 40;
var LEVEL_NAMES = ['Clinic', 'Hallway', 'Playground', 'Park', 'Beach', 'Mall'];
var LEVEL_EMOJI = ['🏥', '🏃', '🛝', '🌳', '🏖️', '🛍️'];
var GAP_START = 220, GAP_MAX = 250, GAP_CAUGHT = 55;

// Backdrops first so the characters draw on top.
var bgs = [];
for (var bi = 0; bi < 3; bi++) {
  bgs.push(Shim.sprite('bg' + bi, { x: 195, y: 422, w: 390, h: 844 }));
}
bgs[1].visible = false;
bgs[2].visible = false;

var lane = 1;
var blob = Shim.sprite('blob', { x: LANES[1], y: 680, w: 112, h: 128 });
var chaser = Shim.sprite('chaser', { x: LANES[1], y: 680 + GAP_START, w: 105, h: 122 });

Shim.text('title', '💉 Escape from the Injection', { x: 16, y: 14, size: 24, color: '#123046' });
Shim.text('meta', 'LV 1 · Clinic 🏥', { x: 16, y: 46, size: 22, color: '#1e7b3a' });
Shim.text('score', 'Score: 0', { x: 296, y: 14, size: 24, color: '#123046' });
Shim.text('hint', 'Tap ◀ left / right ▶ to dodge!\nTip: stay in the middle lane.', { x: 16, y: 78, size: 18, color: '#123046' });
Shim.text('banner', '', { x: 16, y: 140, size: 24, color: '#b3402e' });

Shim.audio.music('audio/theme.wav', { loop: true, volume: 0.45 });

var items = [];           // { s, kind: 'syringe' | 'star', lane }
var gap = GAP_START;
var score = 0;
var elapsed = 0, levelT = 0, spawnT = 0, starT = 3, hintT = 0, bannerT = 0;
var metaLevel = 1, metaMult = 1;
var over = false, ended = false, cried = false, catchT = 0;

function levelName(n) {
  if (n <= LEVEL_NAMES.length) return LEVEL_NAMES[n - 1] + ' ' + LEVEL_EMOJI[n - 1];
  return 'Nowhere Left 😱';
}

function spawnSyringe() {
  // Fairness: never build an unavoidable 3-lane wall. If two lanes already
  // have "young" syringes (spawned <0.6s ago, arriving together), stack the
  // new one into one of those lanes so the third lane stays escapable.
  var young = {};
  for (var i = 0; i < items.length; i++) {
    var it = items[i];
    if (it.kind === 'syringe' && elapsed - it.born < 0.6) young[it.lane] = true;
  }
  var lanes = Object.keys(young);
  var l = lanes.length >= 2
    ? parseInt(lanes[Math.floor(Math.random() * lanes.length)], 10)
    : Math.floor(Math.random() * 3);
  var s = Shim.sprite('syringe', { x: LANES[l], y: -60, w: 56, h: 84 });
  items.push({ s: s, kind: 'syringe', lane: l, born: elapsed });
}

function spawnStar() {
  var l = Math.floor(Math.random() * 3);
  var s = Shim.sprite('star', { x: LANES[l], y: -40, w: 52, h: 52 });
  items.push({ s: s, kind: 'star', lane: l });
}

function removeItem(i) {
  items[i].s.visible = false;
  items.splice(i, 1);
}

function setScore() {
  Shim.text('score', 'Score: ' + Math.round(score), { x: 296, y: 14, size: 24, color: '#123046' });
}

Shim.input('tap', function (x, y) {
  if (over) return;
  if (x < 195 && lane > 0) {
    lane--;
    Shim.audio.tone({ freq: 520, dur: 0.06, type: 'sine' });
  } else if (x >= 195 && lane < 2) {
    lane++;
    Shim.audio.tone({ freq: 620, dur: 0.06, type: 'sine' });
  }
});

Shim.loop(function (dt) {
  if (ended) return;

  // ---- caught sequence: the chaser lunges, then the tears ----
  if (over) {
    catchT += dt;
    gap = Math.max(0, gap - 420 * dt);
    chaser.x += (blob.x - chaser.x) * Math.min(1, dt * 10);
    chaser.y = 680 + gap;
    if (catchT > 0.55 && !cried) {
      cried = true;
      Shim.text('banner', '😭 Injected in the butt...', { x: 16, y: 140, size: 24, color: '#b3402e' });
      Shim.audio.tone({ freq: 220, dur: 0.5, type: 'sawtooth' });
    }
    if (catchT > 1.7) {
      ended = true;
      Shim.end({ won: false, score: Math.round(score),
                 stats: { level: metaLevel, time_s: Math.round(elapsed) } });
    }
    return;
  }

  elapsed += dt;
  score += dt * 10;
  setScore();

  if (hintT < 4) {
    hintT += dt;
    if (hintT >= 4) Shim.text('hint', '', { x: 16, y: 78, size: 18, color: '#123046' });
  }
  if (bannerT > 0) {
    bannerT -= dt;
    if (bannerT <= 0) Shim.text('banner', '', { x: 16, y: 140, size: 24, color: '#b3402e' });
  }

  // ---- escape to a new place every LEVEL_SECS: quicker and quicker ----
  levelT += dt;
  if (levelT >= LEVEL_SECS) {
    levelT = 0;
    metaLevel++;
    metaMult *= 1.12;
    var li = (metaLevel - 1) % 3;
    for (var k = 0; k < 3; k++) {
      bgs[k].visible = (k === li);
    }
    Shim.text('meta', 'LV ' + metaLevel + ' · ' + levelName(metaLevel),
              { x: 16, y: 46, size: 22, color: '#1e7b3a' });
    Shim.text('banner', 'LEVEL ' + metaLevel + '! Faster! ' + levelName(metaLevel),
              { x: 16, y: 140, size: 24, color: '#b3402e' });
    bannerT = 2.5;
    Shim.audio.tone({ freq: 880, dur: 0.25, type: 'square' });
  }

  // ---- the blob slides toward its lane; the chaser trails behind ----
  var tx = LANES[lane];
  blob.x += (tx - blob.x) * Math.min(1, dt * 12);
  chaser.x += (blob.x - chaser.x) * Math.min(1, dt * 5);
  chaser.y = 680 + gap;

  // ---- spawn syringes and stars ----
  var capped = Math.min(metaMult, 3.2);
  var speed = 210 * capped;
  spawnT += dt;
  if (spawnT >= Math.max(0.55, 1.3 / capped)) {
    spawnT = 0;
    spawnSyringe();
  }
  starT += dt;
  if (starT >= 2.5) {
    starT = 0;
    spawnStar();
  }

  // ---- move everything down; collisions ----
  for (var i = items.length - 1; i >= 0; i--) {
    var it = items[i];
    it.s.y += speed * dt;
    if (it.kind === 'syringe') {
      if (Math.abs(it.s.y - 680) < 60 && Math.abs(it.s.x - blob.x) < 50) {
        gap -= 45;
        Shim.audio.tone({ freq: 150, dur: 0.2, type: 'sawtooth' });
        removeItem(i);
        continue;
      }
    } else {
      if (Math.abs(it.s.y - 680) < 60 && Math.abs(it.s.x - blob.x) < 55) {
        score += 25;
        gap = Math.min(GAP_MAX, gap + 12);
        Shim.audio.tone({ freq: 990, dur: 0.12, type: 'sine' });
        removeItem(i);
        setScore();
        continue;
      }
    }
    if (it.s.y > 920) removeItem(i);
  }

  // ---- clean running slowly rebuilds the lead ----
  gap = Math.min(GAP_MAX, gap + 6 * dt);

  // ---- caught? the plot says he is finally caught ----
  if (gap <= GAP_CAUGHT) {
    over = true;
    catchT = 0;
    Shim.text('banner', '💉 CAUGHT!', { x: 16, y: 140, size: 24, color: '#b3402e' });
    Shim.text('hint', '', { x: 16, y: 78, size: 18, color: '#123046' });
    Shim.audio.tone({ freq: 180, dur: 0.4, type: 'sawtooth' });
  }
});
