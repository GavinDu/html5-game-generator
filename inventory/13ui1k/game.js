// Bobo's Rainbow v4 — endless meta-levels + leaderboard.
// Catch falling hearts to repaint the rainbow. Clear all 12 hearts and you
// advance to the next LV, faster and faster. Score accumulates across LVs.
// Miss 3 hearts and the run ends — your LV goes on the leaderboard.
// Tap Play Again for another run.
Shim.init({ width: 390, height: 844, background: '#8ecae6' });

var BAND_NAMES = ['Red', 'Orange', 'Yellow', 'Green', 'Blue', 'Purple'];
var HEARTS_TO_WIN = 12;
var MAX_MISSES = 3;
var MAX_COMBO = 5;

var bands = [];
for (var b = 0; b < 6; b++) {
  bands.push(Shim.sprite('band' + b, { x: 195, y: 560, w: 380, h: 230, visible: false }));
}
var kitten = Shim.sprite('kitten', { x: 195, y: 600, w: 76, h: 88 });
var bear = Shim.sprite('bear', { x: 195, y: 740, w: 104, h: 192 });

var hearts = []; // {s, gold}
var caught = 0, misses = 0, score = 0, combo = 1, bandsOn = 0;
var elapsed = 0, spawnT = 0, over = false, level = 1;
var metaLevel = 1, metaMult = 1; // win a round -> next LV, quicker

Shim.audio.music('audio/theme.wav', { loop: true, volume: 0.5 });

Shim.text('title', "Bobo's Rainbow", { x: 16, y: 14, size: 24, color: '#123046' });
Shim.text('meta', 'LV 1', { x: 216, y: 14, size: 24, color: '#1e7b3a' });
Shim.text('score', 'Score: 0', { x: 16, y: 46, size: 26, color: '#123046' });
Shim.text('combo', '', { x: 16, y: 78, size: 20, color: '#b3402e' });
Shim.text('lives', '❤❤❤', { x: 296, y: 14, size: 24, color: '#d33a4e' });
Shim.text('banner', '', { x: 16, y: 108, size: 20, color: '#b3402e' });
Shim.text('hint', 'Tap to move Bobo - catch the hearts!', { x: 16, y: 140, size: 18, color: '#123046' });

Shim.input('tap', function (x, y) {
  bear.x = Math.max(52, Math.min(338, x));
});

function setScore() {
  Shim.text('score', 'Score: ' + score, { x: 16, y: 46, size: 26, color: '#123046' });
}
function setCombo() {
  Shim.text('combo', combo > 1 ? ('COMBO x' + combo + '!') : '',
    { x: 16, y: 78, size: 20, color: '#b3402e' });
}
function setLives() {
  var s = '';
  for (var i = 0; i < MAX_MISSES - misses; i++) s += '❤';
  Shim.text('lives', s, { x: 296, y: 14, size: 24, color: '#d33a4e' });
}
function setMeta() {
  Shim.text('meta', 'LV ' + metaLevel, { x: 216, y: 14, size: 24, color: '#1e7b3a' });
}
function starStr(n) {
  var s = '';
  for (var i = 0; i < 3; i++) s += (i < n ? '★' : '☆');
  return s;
}

function restoreBand() {
  bands[bandsOn].visible = true;
  // Round level up every 2 hearts: quicker within the round too.
  var nl = Math.min(6, Math.floor(caught / 2) + 1);
  var msg = BAND_NAMES[bandsOn] + ' restored!';
  if (nl > level) {
    level = nl;
    msg = 'LEVEL ' + level + '! ' + msg;
    Shim.audio.tone({ freq: 660 + level * 110, dur: 0.25, type: 'triangle' });
  }
  Shim.text('banner', msg, { x: 16, y: 108, size: 20, color: '#b3402e' });
  bandsOn += 1;
}

function nextRound() {
  metaLevel += 1;
  metaMult *= 1.25;
  caught = 0; misses = 0; combo = 1; bandsOn = 0; level = 1; spawnT = 0;
  hearts = [];
  for (var b = 0; b < 6; b++) bands[b].visible = false;
  setMeta(); setCombo(); setLives();
  Shim.text('banner', 'LV ' + metaLevel + '! Faster!',
    { x: 16, y: 108, size: 24, color: '#1e7b3a' });
  Shim.audio.tone({ freq: 784, dur: 0.15, type: 'triangle' });
  Shim.audio.tone({ freq: 1047, dur: 0.25, type: 'triangle' });
}

function endGame() {
  over = true;
  var n = Math.min(3, metaLevel);
  Shim.text('banner', starStr(n) + ' Reached LV ' + metaLevel + '!',
    { x: 16, y: 108, size: 22, color: '#b3402e' });
  Shim.text('hint', 'Tap Play Again for another run!',
    { x: 16, y: 140, size: 18, color: '#123046' });
  Shim.audio.tone({ freq: 220, dur: 0.5, type: 'triangle' });
  Shim.end({ score: score, won: false,
    stats: { time_s: Math.round(elapsed), stars: n, level: metaLevel } });
}

Shim.loop(function (dt) {
  if (over) return;
  elapsed += dt;

  // Pace: round level ramp x meta-level multiplier — quicker every LV.
  var speed = 175 * Math.pow(1.15, level - 1) * metaMult;
  var interval = Math.max(0.35, 0.8 / Math.pow(1.12, level - 1) / metaMult);
  if (caught < HEARTS_TO_WIN) {
    spawnT += dt;
    if (spawnT > interval) {
      spawnT = 0;
      var gold = Math.random() < 0.12;
      var kind = gold ? 'heart_gold' : (Math.random() < 0.5 ? 'heart' : 'heart_blue');
      var s = Shim.sprite(kind, { x: 30 + Math.random() * 330, y: -24, w: 46, h: 46 });
      hearts.push({ s: s, gold: gold });
    }
  }

  for (var i = hearts.length - 1; i >= 0; i--) {
    var h = hearts[i];
    h.s.y += speed * dt;
    var dx = h.s.x - bear.x;
    var dy = h.s.y - (bear.y - 40);
    if (dx * dx + dy * dy < 3600) {
      // Caught!
      h.s.visible = false;
      hearts.splice(i, 1);
      if (h.gold) {
        score += 50;
        Shim.text('banner', 'GOLDEN HEART +50!', { x: 16, y: 108, size: 20, color: '#b8860b' });
        Shim.audio.tone({ freq: 1200, dur: 0.2, type: 'sine' });
      } else {
        score += 10 * combo;
        caught += 1;
        Shim.audio.tone({ freq: 520 + combo * 90, dur: 0.12, type: 'sine' });
        if (caught % 2 === 0) restoreBand();
        combo = Math.min(MAX_COMBO, combo + 1);
      }
      setScore();
      setCombo();
      if (caught >= HEARTS_TO_WIN) { nextRound(); return; }
    } else if (h.s.y > 880) {
      // Missed.
      h.s.visible = false;
      hearts.splice(i, 1);
      if (!h.gold) {
        misses += 1;
        combo = 1;
        setCombo();
        setLives();
        Shim.audio.tone({ freq: 180, dur: 0.2, type: 'sine' });
        if (misses >= MAX_MISSES) { endGame(); return; }
      }
    }
  }
});
