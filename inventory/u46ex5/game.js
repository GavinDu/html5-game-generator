// 飘带补天 Ribbon Mends the Sky — built from a photo (DoodleQuest).
// GameShim v1. The little hero rides her cloud: tap to fly, dive through
// dark sky-holes so her ribbons stitch them shut, tap wind demons to bonk
// them with the golden staff, catch rainbow shards for bonus.
Shim.init({ width: 390, height: 844, background: '#3a4a7a' });

var HOLES_TO_WIN = 6;
var holes = [];   // {open, closed, x, y, ty, stitched}
var demons = [];  // {s, x, y, sway}
var shards = [];  // {s, x, y}
var sparks = [];  // {s, x, y, t}
var score = 0;
var stitched = 0;
var elapsed = 0;
var holesSpawned = 0;
var spawnHoleT = 0;
var spawnDemonT = 3;
var spawnShardT = 1.5;
var won = false;
var winT = -1;
var winTones = [];

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
Shim.text('score', '🌈 0/6  ⭐ 0', { x: 16, y: 48, size: 24, color: '#ffffff' });
Shim.text('banner', '', { x: 16, y: 84, size: 22, color: '#ffe27a' });
Shim.text('hint1', '👆 点哪里，云朵就飞到哪里', { x: 16, y: 796, size: 17, color: '#ffffff' });
Shim.text('hint2', '🎀 钻过黑洞缝补天空  🔨 点风妖敲它', { x: 16, y: 818, size: 17, color: '#ffffff' });

function setScore() {
  Shim.text('score', '🌈 ' + stitched + '/' + HOLES_TO_WIN + '  ⭐ ' + score,
    { x: 16, y: 48, size: 24, color: '#ffffff' });
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
  burst(h.x, h.y);
  setScore();
  Shim.audio.tone({ freq: 520 + stitched * 60, dur: 0.15, type: 'sine' });
  if (stitched >= HOLES_TO_WIN) {
    Shim.text('banner', '天空补好啦！🌈', { x: 16, y: 84, size: 24, color: '#ffe27a' });
    winT = 0;
    winTones = [
      { at: 0, freq: 523 }, { at: 0.22, freq: 659 },
      { at: 0.44, freq: 784 }, { at: 0.66, freq: 1047 }
    ];
  } else {
    Shim.text('banner', '补好啦！ ' + stitched + '/' + HOLES_TO_WIN + ' 🎀',
      { x: 16, y: 84, size: 22, color: '#ffe27a' });
  }
}

function bonkDemon(d) {
  var i = demons.indexOf(d);
  if (i >= 0) demons.splice(i, 1);
  d.s.visible = false;
  score += 20;
  setScore();
  burst(d.x, d.y);
  Shim.audio.tone({ freq: 200, dur: 0.15, type: 'triangle' });
}

Shim.input('tap', function (x, y) {
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
  elapsed += dt;

  // Cloud flight: glide toward the tap point, with a gentle bob.
  var k = Math.min(1, dt * 5);
  hx += (tx - hx) * k;
  hy += (ty - hy) * k;
  hero.x = hx;
  hero.y = hy + Math.sin(elapsed * 4) * 6;

  // Sky-holes drift in.
  if (holesSpawned < HOLES_TO_WIN) {
    spawnHoleT += dt;
    if (spawnHoleT > 2.2) { spawnHoleT = 0; spawnHole(); }
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

  // Wind demons bumble down.
  spawnDemonT += dt;
  if (spawnDemonT > 4 && demons.length < 3) { spawnDemonT = 0; spawnDemon(); }
  for (var i = demons.length - 1; i >= 0; i--) {
    var d = demons[i];
    d.y += 55 * dt;
    d.sway += dt * 3;
    d.x += Math.sin(d.sway) * 30 * dt;
    d.s.x = d.x;
    d.s.y = d.y;
    if (d.y > 900) { d.s.visible = false; demons.splice(i, 1); }
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
      score += 10;
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

  // Win fanfare, then end.
  if (winT >= 0) {
    winT += dt;
    while (winTones.length && winTones[0].at <= winT) {
      var t = winTones.shift();
      Shim.audio.tone({ freq: t.freq, dur: 0.25, type: 'triangle' });
    }
    if (winT > 1.1 && !won) {
      won = true;
      Shim.end({ score: score, won: true, stats: { time_s: Math.round(elapsed) } });
    }
  }
});
