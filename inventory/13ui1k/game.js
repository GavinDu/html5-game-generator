// Bobo's Rainbow v2 — Poki-style revision of the kid's-drawing game.
// Tap to slide Bobo along the sky and catch falling hearts to repaint the
// fading rainbow, band by band, so the kitten can cross.
//
// Poki elements: combo multiplier for consecutive catches, 3-miss game over
// ("one more try" loop), golden-heart power-up, speed ramp, 1-3 star rating.
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

var hearts = []; // {s, gold, x, y}
var caught = 0, misses = 0, score = 0, combo = 1, bandsOn = 0;
var elapsed = 0, spawnT = 0, over = false;

Shim.audio.music('audio/theme.wav', { loop: true, volume: 0.5 });

Shim.text('title', "Bobo's Rainbow", { x: 16, y: 14, size: 24, color: '#123046' });
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
function starCount() {
  if (score >= 450) return 3;
  if (score >= 300) return 2;
  return 1;
}
function starStr(n) {
  var s = '';
  for (var i = 0; i < 3; i++) s += (i < n ? '★' : '☆');
  return s;
}

function restoreBand() {
  bands[bandsOn].visible = true;
  Shim.text('banner', BAND_NAMES[bandsOn] + ' restored!',
    { x: 16, y: 108, size: 20, color: '#b3402e' });
  bandsOn += 1;
}

function endGame(won) {
  over = true;
  var n = starCount();
  Shim.text('banner', won ? (starStr(n) + ' Rainbow complete!') : (starStr(n) + ' Try again!'),
    { x: 16, y: 108, size: 22, color: won ? '#1e7b3a' : '#b3402e' });
  Shim.text('hint', won ? 'The kitten can cross!' : 'Catch the hearts, avoid misses!',
    { x: 16, y: 140, size: 18, color: '#123046' });
  Shim.audio.tone({ freq: won ? 880 : 220, dur: 0.5, type: 'triangle' });
  Shim.end({ score: score, won: won, stats: { time_s: Math.round(elapsed), stars: n } });
}

Shim.loop(function (dt) {
  if (over) return;
  elapsed += dt;

  // Poki ramp: hearts fall faster and spawn quicker as you catch more.
  var speed = 175 + caught * 10;
  var interval = Math.max(0.45, 0.8 - caught * 0.03);
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
      if (caught >= HEARTS_TO_WIN) { endGame(true); return; }
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
        if (misses >= MAX_MISSES) { endGame(false); return; }
      }
    }
  }
});
