// Penguin's Heartlight Rescue v1.
// A gentle, drawing-inspired catcher: tap to guide the penguin and catch
// 12 hearts. Every pair lights one aurora band. Misses never cost a life.
Shim.init({ width: 390, height: 844, background: '#c8ecf8' });

var TARGET = 12;
var BAND_NAMES = ['Ruby', 'Amber', 'Sunshine', 'Meadow', 'Ocean', 'Violet'];
var hearts = [];
var caught = 0;
var score = 0;
var elapsed = 0;
var spawnTimer = 0;
var ended = false;
var skin = 0;

var bands = [];
for (var b = 0; b < 6; b++) {
  bands.push(Shim.sprite('band' + b, {
    x: 195, y: 500, w: 380, h: 230, visible: false
  }));
}

var penguins = [
  Shim.sprite('penguin_rainbow', { x: 195, y: 724, w: 168, h: 201 }),
  Shim.sprite('penguin_holiday', { x: 195, y: 724, w: 168, h: 201, visible: false }),
  Shim.sprite('penguin_classic', { x: 195, y: 724, w: 168, h: 201, visible: false })
];

Shim.audio.music('audio/theme.wav', { loop: true, volume: 0.42 });
Shim.text('title', "Penguin's Heartlight Rescue", {
  x: 14, y: 14, size: 22, color: '#173b59'
});
Shim.text('progress', 'Hearts 0 / 12', {
  x: 14, y: 48, size: 25, color: '#173b59'
});
Shim.text('scarf', 'RAINBOW SCARF', {
  x: 14, y: 82, size: 17, color: '#7046a6'
});
Shim.text('banner', 'Catch hearts to wake the aurora!', {
  x: 14, y: 116, size: 18, color: '#245b78'
});
Shim.text('hint', 'Tap anywhere to guide your penguin', {
  x: 14, y: 804, size: 17, color: '#245b78'
});

function setSkin(next) {
  if (next === skin) return;
  penguins[skin].visible = false;
  skin = next;
  penguins[skin].visible = true;
  var names = ['RAINBOW SCARF', 'HOLIDAY SCARF', 'CLASSIC BLUE SCARF'];
  Shim.text('scarf', names[skin], {
    x: 14, y: 82, size: 17, color: '#7046a6'
  });
  Shim.text('banner', skin === 1 ? 'A festive new look!' : 'A classic winter look!', {
    x: 14, y: 116, size: 18, color: '#245b78'
  });
}

function updateProgress() {
  Shim.text('progress', 'Hearts ' + caught + ' / ' + TARGET + '   Stars ' + score, {
    x: 14, y: 48, size: 23, color: '#173b59'
  });
}

function finish() {
  if (ended) return;
  ended = true;
  Shim.text('banner', 'Aurora restored! You warmed the whole sky! ✨', {
    x: 14, y: 116, size: 19, color: '#7046a6'
  });
  Shim.text('hint', 'Wonderful helping, little explorer!', {
    x: 14, y: 804, size: 17, color: '#245b78'
  });
  Shim.audio.tone({ freq: 523, dur: 0.18, type: 'triangle' });
  Shim.audio.tone({ freq: 659, dur: 0.18, type: 'triangle' });
  Shim.audio.tone({ freq: 784, dur: 0.28, type: 'triangle' });
  Shim.end({
    score: score,
    won: true,
    stats: { hearts: caught, aurora_bands: 6, time_s: Math.round(elapsed), skin: skin }
  });
}

Shim.input('tap', function (x) {
  if (ended) return;
  penguins[skin].x = Math.max(58, Math.min(332, x));
});

Shim.loop(function (dt) {
  if (ended) return;
  elapsed += dt;
  spawnTimer += dt;

  var interval = Math.max(0.42, 0.82 - caught * 0.025);
  if (caught < TARGET && spawnTimer >= interval) {
    spawnTimer = 0;
    var golden = Math.random() < 0.14;
    var sprite = Shim.sprite(golden ? 'heart_gold' : 'heart', {
      x: 34 + Math.random() * 322, y: -28, w: golden ? 42 : 38, h: golden ? 42 : 38
    });
    hearts.push({ sprite: sprite, golden: golden });
  }

  var fallSpeed = 150 + Math.min(55, caught * 4);
  for (var i = hearts.length - 1; i >= 0; i--) {
    var heart = hearts[i];
    heart.sprite.y += fallSpeed * dt;
    var dx = heart.sprite.x - penguins[skin].x;
    var dy = heart.sprite.y - 690;
    if (dx * dx + dy * dy < 3300) {
      heart.sprite.visible = false;
      hearts.splice(i, 1);
      caught += 1;
      score += heart.golden ? 30 : 10;
      if (caught % 2 === 0) {
        var band = caught / 2 - 1;
        bands[band].visible = true;
        Shim.text('banner', BAND_NAMES[band] + ' aurora restored!', {
          x: 14, y: 116, size: 20, color: '#7046a6'
        });
        Shim.audio.tone({ freq: 520 + band * 75, dur: 0.16, type: 'sine' });
      } else {
        Shim.audio.tone({ freq: 660, dur: 0.1, type: 'sine' });
      }
      if (caught === 4) setSkin(1);
      if (caught === 8) setSkin(2);
      updateProgress();
      if (caught >= TARGET) { finish(); return; }
    } else if (heart.sprite.y > 880) {
      // Gentle mode: missed hearts fade away without taking a life.
      heart.sprite.visible = false;
      hearts.splice(i, 1);
    }
  }
});