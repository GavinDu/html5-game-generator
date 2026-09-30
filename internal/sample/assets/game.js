// Bobo's Rainbow — sample game built from a kid's drawing (DoodleQuest).
// GameShim v1. Bobo the Bear Prince catches falling hearts to repaint the
// fading rainbow, band by band, so the kitten can cross.
Shim.init({ width: 390, height: 844, background: '#8ecae6' });

var BAND_NAMES = ['Red', 'Orange', 'Yellow', 'Green', 'Blue', 'Purple'];
var HEARTS_TO_WIN = 12;

// The rainbow arch; bands start invisible (faded) and are restored in order.
var bands = [];
for (var b = 0; b < 6; b++) {
  bands.push(Shim.sprite('band' + b, { x: 195, y: 560, w: 380, h: 230, visible: false }));
}

// The kitten waits inside the rainbow's arch.
var kitten = Shim.sprite('kitten', { x: 195, y: 600, w: 76, h: 88 });

// Bobo, the player, slides along the bottom of the sky.
var bear = Shim.sprite('bear', { x: 195, y: 740, w: 104, h: 192 });

var hearts = [];
var score = 0;
var bandsOn = 0;
var elapsed = 0;
var spawnT = 0;

Shim.audio.music('audio/theme.wav', { loop: true, volume: 0.5 });

Shim.text('title', "Bobo's Rainbow", { x: 16, y: 14, size: 24, color: '#123046' });
Shim.text('score', 'Hearts: 0/12', { x: 16, y: 46, size: 26, color: '#123046' });
Shim.text('banner', '', { x: 16, y: 82, size: 20, color: '#b3402e' });
Shim.text('hint', 'Tap to move Bobo - catch the hearts!', { x: 16, y: 114, size: 18, color: '#123046' });

Shim.input('tap', function (x, y) {
  bear.x = Math.max(52, Math.min(338, x));
});

function restoreBand() {
  bands[bandsOn].visible = true;
  Shim.text('banner', BAND_NAMES[bandsOn] + ' restored!', { x: 16, y: 82, size: 20, color: '#b3402e' });
  bandsOn += 1;
}

Shim.loop(function (dt) {
  elapsed += dt;

  // Keep hearts coming until the rainbow is whole — no way to lose.
  if (score < HEARTS_TO_WIN) {
    spawnT += dt;
    if (spawnT > 0.8) {
      spawnT = 0;
      var kind = Math.random() < 0.5 ? 'heart' : 'heart_blue';
      hearts.push(Shim.sprite(kind, { x: 30 + Math.random() * 330, y: -24, w: 46, h: 46 }));
    }
  }

  for (var i = hearts.length - 1; i >= 0; i--) {
    var h = hearts[i];
    h.y += 175 * dt;
    var dx = h.x - bear.x;
    var dy = h.y - (bear.y - 40);
    if (dx * dx + dy * dy < 3600) {
      h.visible = false;
      hearts.splice(i, 1);
      score += 1;
      Shim.text('score', 'Hearts: ' + score + '/12', { x: 16, y: 46, size: 26, color: '#123046' });
      Shim.audio.tone({ freq: 520 + score * 32, dur: 0.12, type: 'sine' });
      if (score % 2 === 0) {
        restoreBand();
      }
    } else if (h.y > 880) {
      h.visible = false;
      hearts.splice(i, 1);
    }
  }

  if (score >= HEARTS_TO_WIN) {
    Shim.text('banner', 'The rainbow is whole! The kitten can cross!',
      { x: 16, y: 82, size: 20, color: '#1e7b3a' });
    Shim.audio.tone({ freq: 880, dur: 0.4, type: 'triangle' });
    Shim.end({ score: score, won: true, stats: { time_s: Math.round(elapsed) } });
  }
});
