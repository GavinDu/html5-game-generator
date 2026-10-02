# 飘带补天 Ribbon Mends the Sky — plot & design

Source: a photo of a little girl in a white-and-blue fairy dress, sitting on
a swirl cloud, holding a golden carved staff, blue-white ribbons flowing —
flying through a sunset cloud sea. Chosen by random draw from the top-20
Chinese children's stories: plot B, inspired by 神笔马良 (Magic Brush Ma
Liang — the ribbon as a magic mending brush) + 九色鹿 (Nine-Colored Deer —
the rainbow shards).

## Elements read from the photo

| # | Element | Role in the game |
|---|---------|------------------|
| 1 | **小女孩 the little girl** | The hero — flies the cloud (cut out from the photo) |
| 2 | **金箍棒 the golden staff** | Tap wind demons to bonk them (+20) |
| 3 | **筋斗云 the swirl cloud** | Her mount — tap anywhere and she flies there |
| 4 | **蓝白飘带 the flowing ribbons** | The mending magic — fly through a sky-hole and the ribbons stitch it shut |
| 5 | **云海天空 the cloud sea** | The game world (sunset sky background) |
| 6 | **风妖 wind demons** | Grumpy storm blobs drifting down; bonk for bonus |
| 7 | **彩虹碎片 rainbow shards** | Nine-colored deer shards; collect for +10 |

## Main plot — "飘带补天"

The wind demons have torn six dark holes in the sky, and the cloud sea is
leaking starlight. The little girl rides her somersault cloud up into the
sunset, golden staff in hand, ribbons streaming behind her.

**Fly into each dark hole and her ribbons stitch it shut with golden
stitches.** The wind demons bumble around trying to look scary — tap one and
she bonks it with the staff for bonus points. Rainbow shards from the
nine-colored deer drift by; catch them for extra sparkle.

**Mend all 6 sky-holes and the sky is whole again** — the sunset glows, the
demons pout, everybody celebrates.

Kid-safe by design: there is no way to lose — holes wait patiently to be
mended, and demons are only good for bonus points.

## Play guide (3 steps)

1. 👆 点哪里，云朵就飞到哪里 — tap anywhere, she flies there
2. 🎀 钻过黑洞，飘带把它缝好 — fly through dark holes to stitch them (6 to win)
3. 🔨 点风妖，金箍棒敲它加分 — tap wind demons to bonk them (+20)

## Mechanics (GameShim v1)

- **Controls:** tap only (click / touch)
- **Loop:** holes drift down from the top; hero lerps toward the tap point
  with a gentle cloud bob
- **Stitch:** hero within range of an open hole → hole swaps to the golden
  stitched sprite, spark burst, rising chime, banner "补好啦！ n/6"
- **Bonk:** tap within range of a demon → demon pops, +20, low thump
- **Collect:** hero touches a shard → +10, bright pluck
- **Win:** 6/6 holes stitched → fanfare → `Shim.end({ won: true })`
- **Music:** playful pentatonic loop （宫商角徵羽 — music-box style)

## Alternate plots (not built)

1. **Moon Pearl Chase** （彩带追月, after 嫦娥奔月) — pearls scattered over
   the cloud sea; ribbon-trail to scoop them, staff-bonk storm imps.
2. **Imp Roundup** （小妖大作战, after 天书奇谭) — catch runaway imps with the
   ribbons as a net; free the caged sky-animals.

## v2 revision (2026-10-02) — Poki-inspired engagement pass
Kept the photo-cutout hero, ribbons, demons and tap-to-fly core; borrowed
Poki's risk/reward loop (dodge-and-collect school: streaks, hazards that cost
lives, escalating waves, star ratings):
- **Stitch combo**: consecutive hole stitches build 连击 x2–x5; each stitch
  scores 50 × combo.
- **Demons fight back**: a demon that reaches the hero costs one of 3 lives
  (❤❤❤ display) and resets the combo; 1.5s invulnerability after a hit.
  Tapping a demon to bonk it still scores a flat +100.
- **Lucky shards**: every 3rd rainbow shard is a +100 lucky bonus.
- **Wave ramp**: holes spawn faster as the sky heals; demons get faster and
  bolder (up to 4 at once) in the back half.
- **Star rating**: ★ thresholds at 400 / 900 / 1300, shown on the canvas and
  in end stats.
Six stitched holes still win the sky; 0 lives ends the run with "再试一次！".

## v3 revision (2026-10-02) — explicit level-up pacing
The sky now has 3 explicit waves (one per 2 holes stitched, announced with
"第 X 波！更快了！" + rising tone). Hole spawn, demon spawn and demon fall
speed all scale multiplicatively per level (1.25–1.3x), up to 5 demons at
once, and the hero glides a touch quicker too — noticeably quicker and
quicker as you level up.
