// 飘带补天 Ribbon Mends the Sky v4 — endless meta-levels + leaderboard.
// Fly into dark sky-holes so the ribbons stitch them shut. Mend all 6 and
// you advance to the next 关, faster and faster. Score accumulates across
// rounds. Lose all 3 lives and the run ends — your 关 goes on the leaderboard.
// Tap Play Again for another run.
Shim.init({ width: 390, height: 844, background: '#3a4a7a' });

var HOLES_TO_WIN = 6;
var MAX_LIVES = 3;
var MAX_COMBO = 5;
var holes = [];   // {open, closed, x, y, ty, stitched}
var demons = [];  // {s, x, y, sway}
var shards = [];  // {s, x, y}
var sparks = [];  // {s, x, y, t}
var score = 0;
var stitched = 0;
var combo = 1;
var lives = MAX_LIVES;
var shardsGot = 0;
var elapsed = 0;
var holesSpawned = 0;
var spawnHoleT = 0;
var spawnDemonT = 3;
var spawnShardT = 1.5;
var over = false;
var winT = -1;
var winTones = [];
var hitT = 0; // brief invulnerability after a demon hit
var level = 1; // wave within the round
var metaLevel = 1, metaMult = 1; // win a round -> next 关, quicker

// Sunset cloud-sea world, drawn under everything.
Shim.sprite('bg', { x: 195, y: 422, w: 390, h: 844 });

var hero = Shim.sprite('hero', { x: 195, y: 660, w: 150, h: 96 });
var hx = 195, hy = 660, tx = 195, ty = 660;

// Sparkle pool for stitch/collect/bonk bursts.
for (var sp = 0; sp < 6; sp++) {
  sparks.push({ s: Shim.sprite('spark', { x: -50, y: -50, w: 46, h: 46, visible: false }), x: 0, y: 0, t: 0 });
}

Shim.audio.music('audio/theme.wav', { loop: true, volume: 0.5 });

Shim.text('title', '飘带补天', { x: 16, y: 12, size: 30, color: '#ffffff' });
Shim.text('meta', '第1关', { x: 150, y: 12, size: 24, color: '#ffe27a' });
Shim.text('score', '🌈 0/6  ⭐ 0', { x: 16, y: 48, size: 24, color: '#ffffff' });
Shim.text('combo', '', { x: 16, y: 80, size: 20, color: '#ffe27a' });
Shim.text('lives', '❤❤❤', { x: 282, y: 12, size: 24, color: '#ff6b81' });
Shim.text('banner', '', { x: 16, y: 108, size: 22, color: '#ffe27a' });
Shim.text('hint1', '👆 点哪里，云朵就飞到哪里', { x: 16, y: 796, size: 17, color: '#ffffff' });
Shim.text('hint2', '🎀 钻过黑洞缝补天空  🔨 点风妖敲它', { x: 16, y: 818, size: 17, color: '#ffffff' });

function setScore() {
  Shim.text('score', '🌈 ' + stitched + '/' + HOLES_TO_WIN + '  ⭐ ' + score,
    { x: 16, y: 48, size: 24, color: '#ffffff' });
}
function setCombo() {
  Shim.text('combo', combo > 1 ? ('连击 x' + combo + '!') : '',
    { x: 16, y: 80, size: 20, color: '#ffe27a' });
}
function setLives() {
  var s = '';
  for (var i = 0; i < lives; i++) s += '❤';
  Shim.text('lives', s, { x: 282, y: 12, size: 24, color: '#ff6b81' });
}
function setMeta() {
  Shim.text('meta', '第' + metaLevel + '关', { x: 150, y: 12, size: 24, color: '#ffe27a' });
}
function starStr(n) {
  var s = '';
  for (var i = 0; i < 3; i++) s += (i < n ? '★' : '☆');
  return s;
}

function burst(x, y) {
  for (var i = 0; i < 3; i++) {
    var p = sparks[(Math.floor(Math.random() * sparks.length))];
    p.x = x + (Math.random() * 70 - 35);
    p.y = y + (Math.random() * 70 - 35);
    p.t = 0.5;
    p.s.w = 46; p.s.h = 46;
    p.s.visible = true;
  }
}

function nextRound() {
  metaLevel += 1;
  metaMult *= 1.25;
  stitched = 0; holes = []; demons = []; shards = [];
  lives = MAX_LIVES; combo = 1; level = 1; holesSpawned = 0;
  spawnHoleT = 0; spawnDemonT = 2; spawnShardT = 1;
  winT = -1; winTones = [];
  setMeta(); setCombo(); setLives(); setScore();
  Shim.text('banner', '第 ' + metaLevel + ' 关！更快了！',
    { x: 16, y: 108, size: 24, color: '#ffe27a' });
  Shim.audio.tone({ freq: 880, dur: 0.2, type: 'triangle' });
  Shim.audio.tone({ freq: 1175, dur: 0.3, type: 'triangle' });
}

function endGame() {
  over = true;
  var n = Math.min(3, metaLevel);
  Shim.text('banner', starStr(n) + ' 到达第' + metaLevel + '关！',
    { x: 16, y: 108, size: 24, color: '#ffe27a' });
  Shim.audio.tone({ freq: 220, dur: 0.5, type: 'triangle' });
  Shim.end({ score: score, won: false,
    stats: { time_s: Math.round(elapsed), stars: n, level: metaLevel } });
}

function spawnHole() {
  holesSpawned += 1;
  var x = 60 + Math.random() * 270;
  var open = Shim.sprite('hole', { x: x, y: -70, w: 110, h: 110 });
  var closed = Shim.sprite('hole_closed', { x: x, y: -70, w: 110, h: 110, visible: false });
  holes.push({ open: open, closed: closed, x: x, y: -70, ty: 170 + Math.random() * 380, stitched: false });
}

function spawnDemon() {
  var x = 50 + Math.random() * 290;
  var s = Shim.sprite('demon', { x: x, y: -50, w: 64, h: 78 });
  demons.push({ s: s, x: x, y: -50, sway: Math.random() * 6 });
}

function spawnShard() {
  var x = 50 + Math.random() * 290;
  var s = Shim.sprite('shard', { x: x, y: -40, w: 44, h: 90 });
  shards.push({ s: s, x: x, y: -40 });
}

function stitchHole(h) {
  h.stitched = true;
  h.open.visible = false;
  h.closed.visible = true;
  stitched += 1;
  score += 50 * combo;
  combo = Math.min(MAX_COMBO, combo + 1);
  burst(h.x, h.y);
  setScore();
  setCombo();
  Shim.audio.tone({ freq: 520 + stitched * 60, dur: 0.15, type: 'sine' });
  if (stitched >= HOLES_TO_WIN) {
    winT = 0;
    winTones = [
      { at: 0, freq: 523 }, { at: 0.22, freq: 659 },
      { at: 0.44, freq: 784 }, { at: 0.66, freq: 1047 }
    ];
  } else {
    // Wave up every 2 stitches: quicker within the round too.
    var nl = Math.min(3, Math.floor(stitched / 2) + 1);
    if (nl > level) {
      level = nl;
      Shim.text('banner', '第 ' + level + ' 波！更快了！',
        { x: 16, y: 108, size: 24, color: '#ffe27a' });
      Shim.audio.tone({ freq: 700 + level * 120, dur: 0.25, type: 'triangle' });
    } else {
      Shim.text('banner', '补好啦！ ' + stitched + '/' + HOLES_TO_WIN + ' 🎀',
        { x: 16, y: 108, size: 22, color: '#ffe27a' });
    }
  }
}

function bonkDemon(d) {
  var i = demons.indexOf(d);
  if (i >= 0) demons.splice(i, 1);
  d.s.visible = false;
  score += 100;
  setScore();
  burst(d.x, d.y);
  Shim.audio.tone({ freq: 200, dur: 0.15, type: 'triangle' });
}

function demonHitsHero(d) {
  var i = demons.indexOf(d);
  if (i >= 0) demons.splice(i, 1);
  d.s.visible = false;
  lives -= 1;
  combo = 1;
  hitT = 1.5;
  setLives();
  setCombo();
  burst(hx, hy);
  Shim.text('banner', '哎呀！被风妖撞到了！', { x: 16, y: 108, size: 22, color: '#ff9aa5' });
  Shim.audio.tone({ freq: 150, dur: 0.3, type: 'sawtooth' });
  if (lives <= 0) endGame();
}

Shim.input('tap', function (x, y) {
  if (over) return;
  // A tap that lands on a demon bonks it with the staff.
  var best = null, bestD = 65 * 65;
  for (var i = 0; i < demons.length; i++) {
    var d = demons[i];
    var dx = d.x - x, dy = d.y - y;
    var dd = dx * dx + dy * dy;
    if (dd < bestD) { bestD = dd; best = d; }
  }
  if (best) bonkDemon(best);
  // Otherwise she flies to the tap point.
  tx = Math.max(40, Math.min(350, x));
  ty = Math.max(120, Math.min(780, y));
});

Shim.loop(function (dt) {
  if (over) return;
  elapsed += dt;
  if (hitT > 0) hitT -= dt;

  // Cloud flight: glide toward the tap point, with a gentle bob.
  var k = Math.min(1, dt * (5 + 0.5 * (level - 1)));
  hx += (tx - hx) * k;
  hy += (ty - hy) * k;
  hero.x = hx;
  hero.y = hy + Math.sin(elapsed * 4) * 6;

  // Sky-holes drift in — quicker every wave and every 关.
  if (holesSpawned < HOLES_TO_WIN) {
    spawnHoleT += dt;
    if (spawnHoleT > 2.2 / Math.pow(1.25, level - 1) / metaMult) { spawnHoleT = 0; spawnHole(); }
  }
  for (var i = 0; i < holes.length; i++) {
    var h = holes[i];
    if (h.y < h.ty) {
      h.y += 22 * dt;
      if (h.y > h.ty) h.y = h.ty;
      h.open.y = h.y;
      h.closed.y = h.y;
    }
    if (!h.stitched) {
      var dx = hx - h.x, dy = hy - h.y;
      if (dx * dx + dy * dy < 60 * 60) stitchHole(h);
    }
  }

  // Wind demons bumble down — quicker and bolder every wave and 关.
  spawnDemonT += dt;
  var maxDemons = 2 + level;
  if (spawnDemonT > 4 / Math.pow(1.3, level - 1) / metaMult && demons.length < maxDemons) {
    spawnDemonT = 0; spawnDemon();
  }
  for (var i = demons.length - 1; i >= 0; i--) {
    var d = demons[i];
    d.y += 55 * Math.pow(1.25, level - 1) * metaMult * dt;
    d.sway += dt * 3;
    d.x += Math.sin(d.sway) * 30 * dt;
    d.s.x = d.x;
    d.s.y = d.y;
    if (d.y > 900) { d.s.visible = false; demons.splice(i, 1); continue; }
    var hx2 = hx - d.x, hy2 = hy - d.y;
    if (hitT <= 0 && hx2 * hx2 + hy2 * hy2 < 55 * 55) demonHitsHero(d);
  }

  // Rainbow shards drift by.
  spawnShardT += dt;
  if (spawnShardT > 3 && shards.length < 4) { spawnShardT = 0; spawnShard(); }
  for (var i = shards.length - 1; i >= 0; i--) {
    var sh = shards[i];
    sh.y += 75 * dt;
    sh.s.y = sh.y;
    var dx = hx - sh.x, dy = hy - sh.y;
    if (dx * dx + dy * dy < 50 * 50) {
      sh.s.visible = false;
      shards.splice(i, 1);
      shardsGot += 1;
      score += 25;
      if (shardsGot % 3 === 0) {
        score += 100;
        Shim.text('banner', '幸运星星 +100! ⭐', { x: 16, y: 108, size: 22, color: '#ffe27a' });
      }
      setScore();
      burst(sh.x, sh.y);
      Shim.audio.tone({ freq: 740 + score, dur: 0.12, type: 'sine' });
    } else if (sh.y > 900) {
      sh.s.visible = false;
      shards.splice(i, 1);
    }
  }

  // Sparkle bursts grow and fade.
  for (var i = 0; i < sparks.length; i++) {
    var p = sparks[i];
    if (p.t > 0) {
      p.t -= dt;
      var grow = 1 + (0.5 - p.t) * 1.6;
      p.s.w = 46 * grow;
      p.s.h = 46 * grow;
      p.s.x = p.x;
      p.s.y = p.y;
      if (p.t <= 0) p.s.visible = false;
    }
  }

  // Round-clear fanfare, then the next 关.
  if (winT >= 0) {
    winT += dt;
    while (winTones.length && winTones[0].at <= winT) {
      var t = winTones.shift();
      Shim.audio.tone({ freq: t.freq, dur: 0.25, type: 'triangle' });
    }
    if (winT > 1.1) nextRound();
  }
});
