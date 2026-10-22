// pixi-runtime.js — the single Game IR runtime (PixiJS + Rapier).
//
// Since the 2026-10-20 consolidation this is the ONLY runtime for Game
// IR packages. It interprets IR v1.0 and the additive v1.1 expansion:
//   rendering  — sprites, sprite animation, vector shapes, styled text,
//                the five Pixi core filters, parallax layers;
//   physics    — Rapier bodies (dynamic / fixed / kinematic position /
//                kinematic velocity), colliders (cuboid, ball, capsule,
//                convex, segment), density/friction/restitution with
//                combine rules, sensors, CCD, collision groups, joints
//                (fixed, revolute, prismatic, rope, spring) with limits
//                and motors, and the kinematic character controller;
//   effects    — runtime-owned tweens, particle emitters, camera shake
//                and follow (Pixi core ships none of these);
//   rules      — conditions are evaluated; timer, input, contact,
//                collected/collision, score/health/variable events all
//                fire; actions execute for real (spawn from templates,
//                play_sound, change_scene, impulses, velocities, ...).
// Anything a build cannot interpret fails closed with a dev-log
// warning — never a fake execution.
//
// Contract summary (see docs/game-package-ir-v1.md):
//   GET /v1/games/{id}/package -> { packageVersion: "2.0", gameId, ir, assets[] }
//   Logical stage: 390 x 844, entity positions are centers, sizes px.
//   Rapier works in meters internally (scene.pixelsPerMeter, default
//   100 px = 1 m); IR values stay in logical px / px-per-second.

import * as PIXI from 'pixi.js';
import RAPIER from '@dimforge/rapier2d-compat';

const W = 390, H = 844;
const $ = (id) => document.getElementById(id);
const logEl = $('devlog');

function log(cat, msg) {
  const t = new Date();
  const ts = t.toTimeString().slice(0, 8) + '.' + String(t.getMilliseconds()).padStart(3, '0');
  const line = `[${ts}] [${cat}] ${msg}`;
  logEl.textContent += line + '\n';
  console.log(line);
}

const warned = new Set();
function warnOnce(key, cat, msg) {
  if (warned.has(key)) return;
  warned.add(key);
  log(cat, 'WARN ' + msg);
}

function parseColor(c, fallback = 0xffffff) {
  if (typeof c === 'number') return c;
  if (typeof c !== 'string') return fallback;
  const m = c.trim().match(/^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$/);
  if (!m) return fallback;
  let hex = m[1];
  if (hex.length === 3) hex = hex.split('').map((ch) => ch + ch).join('');
  return parseInt(hex.slice(0, 6), 16);
}

const clamp = (v, lo, hi) => Math.min(hi, Math.max(lo, v));
const rand = (a, b) => a + Math.random() * (b - a);

const EASINGS = {
  linear: (t) => t,
  ease_in: (t) => t * t,
  ease_out: (t) => 1 - (1 - t) * (1 - t),
  ease_in_out: (t) => (t < 0.5 ? 2 * t * t : 1 - Math.pow(-2 * t + 2, 2) / 2),
  ease_out_back: (t) => 1 + 2.7 * Math.pow(t - 1, 3) + 1.7 * Math.pow(t - 1, 2),
  sine: (t) => 0.5 - 0.5 * Math.cos(Math.PI * t),
};

// Soft round particle texture, generated once (emitters without an
// authored texture use it).
let dotTexture = null;
function getDotTexture() {
  if (dotTexture) return dotTexture;
  const c = document.createElement('canvas');
  c.width = c.height = 32;
  const g = c.getContext('2d');
  const grad = g.createRadialGradient(16, 16, 0, 16, 16, 16);
  grad.addColorStop(0, 'rgba(255,255,255,1)');
  grad.addColorStop(0.6, 'rgba(255,255,255,0.7)');
  grad.addColorStop(1, 'rgba(255,255,255,0)');
  g.fillStyle = grad;
  g.fillRect(0, 0, 32, 32);
  dotTexture = PIXI.Texture.from(c);
  return dotTexture;
}

// Runtime-owned particle emitter (Pixi core ships the container, not
// emitter behavior). Pool of sprites; continuous rate + bursts.
class Emitter {
  constructor(container, cfg, textures) {
    this.cfg = cfg || {};
    this.container = container;
    const tex = (cfg?.textureAssetId && textures.get(cfg.textureAssetId)) || getDotTexture();
    this.max = clamp(Math.trunc(cfg?.maxParticles ?? 240), 8, 2000);
    this.pool = [];
    for (let i = 0; i < this.max; i++) {
      const s = new PIXI.Sprite(tex);
      s.anchor.set(0.5);
      s.visible = false;
      if (cfg?.blendMode) s.blendMode = cfg.blendMode;
      if (cfg?.tint) s.tint = parseColor(cfg.tint);
      container.addChild(s);
      this.pool.push({ s, alive: false, age: 0, life: 1, vx: 0, vy: 0 });
    }
    this.acc = 0;
    this.x = 0; this.y = 0; // attach point, set by owner each frame
  }
  burst(n, x, y) {
    let spawned = 0;
    for (const p of this.pool) {
      if (spawned >= n) break;
      if (p.alive) continue;
      const cfg = this.cfg;
      const dir = ((cfg.direction ?? -90) + rand(-(cfg.spread ?? 60), cfg.spread ?? 60) / 2) * Math.PI / 180;
      const speed = rand(cfg.speed?.[0] ?? 40, cfg.speed?.[1] ?? 140);
      p.alive = true; p.age = 0;
      p.life = rand(cfg.lifetime?.[0] ?? 0.4, cfg.lifetime?.[1] ?? 0.9);
      p.vx = Math.cos(dir) * speed; p.vy = Math.sin(dir) * speed;
      p.s.position.set(x + rand(-4, 4), y + rand(-4, 4));
      p.s.visible = true;
      spawned++;
    }
  }
  update(dt) {
    const cfg = this.cfg;
    if (cfg.rate) {
      this.acc += cfg.rate * dt;
      const n = Math.floor(this.acc);
      this.acc -= n;
      if (n > 0) this.burst(n, this.x, this.y);
    }
    const grav = cfg.gravity ?? 0;
    for (const p of this.pool) {
      if (!p.alive) continue;
      p.age += dt;
      if (p.age >= p.life) { p.alive = false; p.s.visible = false; continue; }
      p.vy += grav * dt;
      p.s.position.x += p.vx * dt;
      p.s.position.y += p.vy * dt;
      const t = p.age / p.life;
      const sc = (cfg.startScale ?? 0.5) + ((cfg.endScale ?? 0.05) - (cfg.startScale ?? 0.5)) * t;
      p.s.scale.set(Math.max(0.01, sc));
      p.s.alpha = (cfg.startAlpha ?? 1) + ((cfg.endAlpha ?? 0) - (cfg.startAlpha ?? 1)) * t;
    }
  }
}

const params = new URLSearchParams(location.search);
const gameId = params.get('game');

async function listGames() {
  $('status').textContent = 'Pick a game:';
  const res = await fetch('/v1/feed');
  const data = await res.json();
  const list = $('list');
  for (const g of data.games || []) {
    const card = document.createElement('div');
    card.className = 'game-card';
    const a = document.createElement('a');
    a.href = '/player?game=' + encodeURIComponent(g.game_id);
    a.textContent = g.title || g.game_id;
    const sub = document.createElement('div');
    sub.className = 'sub';
    sub.textContent = g.game_id;
    card.append(a, sub);
    list.append(card);
  }
  log('PKG', `feed listed ${(data.games || []).length} games`);
}

async function run(id) {
  const started = performance.now();
  const res = await fetch(`/v1/games/${encodeURIComponent(id)}/package`);
  if (!res.ok) {
    const body = await res.text();
    throw new Error(`package HTTP ${res.status}: ${body.slice(0, 120)}`);
  }
  const pkg = await res.json();
  log('PKG', `package ${id}: HTTP 200 (${Math.round(performance.now() - started)}ms), package v${pkg.packageVersion}, IR v${pkg.ir?.irVersion}, ${pkg.assets?.length ?? 0} assets`);
  if (pkg.packageVersion !== '2.0') throw new Error('unsupported packageVersion ' + pkg.packageVersion);
  const ir = pkg.ir;
  if (!['1.0', '1.1'].includes(ir.irVersion)) throw new Error('unsupported irVersion ' + ir.irVersion);

  const sceneKey = 'dq.scene.' + id;
  const wantedScene = sessionStorage.getItem(sceneKey) || ir.game.entryScene;
  sessionStorage.removeItem(sceneKey);
  const scene = (ir.scenes || []).find((s) => s.id === wantedScene) || ir.scenes?.[0];
  if (!scene) throw new Error('entry scene missing');

  $('title').textContent = ir.game.title || id;

  const PPM = scene.pixelsPerMeter ?? 100;
  const worldW = scene.worldSize?.[0] ?? W;
  const worldH = scene.worldSize?.[1] ?? H;
  const toM = (px) => px / PPM;

  // --- Pixi stage -----------------------------------------------------
  const app = new PIXI.Application();
  await app.init({
    width: W, height: H, antialias: true,
    background: scene.backgroundColor ? parseColor(scene.backgroundColor, 0x16204a) : 0x16204a,
  });
  $('stage').appendChild(app.canvas);
  log('RT', 'pixi stage ready');
  app.stage.sortableChildren = true;

  const bgLayer = new PIXI.Container();      // parallax layers
  const worldLayer = new PIXI.Container();   // entities (camera-moved)
  worldLayer.sortableChildren = true;
  const fxLayer = new PIXI.Container();      // particles (screen space)
  app.stage.addChild(bgLayer, worldLayer, fxLayer);

  const assetURL = new Map((pkg.assets || []).map((a) => [a.id, a.url || a.uri]));
  const textures = new Map();
  for (const a of pkg.assets || []) {
    if (a.kind !== 'image') continue;
    try {
      textures.set(a.id, await PIXI.Assets.load(assetURL.get(a.id)));
    } catch (e) {
      log('PKG', `WARN asset '${a.id}' failed to load: ${e.message}`);
    }
  }
  log('PKG', `textures loaded: ${textures.size}/${(pkg.assets || []).filter((a) => a.kind === 'image').length} images`);
  const audio = new Map();
  for (const a of pkg.assets || []) {
    if (a.kind === 'audio') audio.set(a.id, assetURL.get(a.id));
  }

  if (scene.backgroundAssetId && textures.has(scene.backgroundAssetId)) {
    const bg = new PIXI.Sprite(textures.get(scene.backgroundAssetId));
    bg.width = W; bg.height = H; bg.zIndex = -1000;
    worldLayer.addChild(bg);
  }
  const layers = [];
  for (const l of scene.layers || []) {
    if (!l.assetId || !textures.has(l.assetId)) continue;
    const s = new PIXI.Sprite(textures.get(l.assetId));
    s.width = W; s.height = H;
    s.zIndex = l.zIndex ?? -900;
    if (l.tint) s.tint = parseColor(l.tint);
    if (l.alpha !== undefined) s.alpha = l.alpha;
    bgLayer.addChild(s);
    layers.push({ sprite: s, px: l.parallax?.[0] ?? 1, py: l.parallax?.[1] ?? 1 });
  }

  // --- Rapier world ---------------------------------------------------
  await RAPIER.init();
  log('PHYS', 'rapier initialized');
  const world = new RAPIER.World({ x: toM(scene.gravity?.[0] ?? 0), y: toM(scene.gravity?.[1] ?? 0) });
  world.timestep = 1 / 60;
  const hasContactForceRules = (ir.rules || []).some((r) => r.when?.event === 'contact_force');
  const eventQueue = new RAPIER.EventQueue(true);

  const COMBINE = {
    average: RAPIER.CoefficientCombineRule.Average, min: RAPIER.CoefficientCombineRule.Min,
    multiply: RAPIER.CoefficientCombineRule.Multiply, max: RAPIER.CoefficientCombineRule.Max,
    geometric_mean: RAPIER.CoefficientCombineRule.GeometricMean, clamped_sum: RAPIER.CoefficientCombineRule.ClampedSum,
  };
  const groupsMask = (list, all = 0xffff) => (list ? list.reduce((m, g) => m | (1 << g), 0) : all);

  const KIND_COLORS = { player: 0x22d3ee, pickup: 0xec4899, enemy: 0xef4444, obstacle: 0xf97316, text: 0xffffff };

  function makeView(def) {
    const w = Math.max(8, def.size?.[0] ?? 48);
    const h = Math.max(8, def.size?.[1] ?? def.size?.[0] ?? 48);
    const r = def.render || {};
    let view;
    if (r.animation?.frames?.length && r.animation.frames.every((f) => textures.has(f))) {
      view = new PIXI.AnimatedSprite(r.animation.frames.map((f) => textures.get(f)));
      view.width = w; view.height = h;
      view.animationSpeed = (r.animation.fps ?? 8) / 60;
      view.loop = r.animation.loop ?? true;
      if (r.animation.pingPong && view.loop) view.loop = true;
      if (r.animation.autoplay ?? true) view.play(); else view.stop();
    } else if (r.shape) {
      view = new PIXI.Graphics();
      const fill = r.shape.fill ? { color: parseColor(r.shape.fill), alpha: 1 } : null;
      const stroke = r.shape.stroke ? { color: parseColor(r.shape.stroke), width: r.shape.strokeWidth ?? 2 } : null;
      const pts = (r.shape.points || []).flat();
      switch (r.shape.type) {
        case 'circle': view.circle(0, 0, Math.min(w, h) / 2); break;
        case 'ellipse': view.ellipse(0, 0, w / 2, h / 2); break;
        case 'polygon': case 'line': if (pts.length >= 4) view.poly(pts); break;
        case 'rounded_rect': view.roundRect(-w / 2, -h / 2, w, h, r.shape.cornerRadius ?? 8); break;
        default: view.rect(-w / 2, -h / 2, w, h);
      }
      if (fill) view.fill(fill);
      if (stroke) view.stroke(stroke);
    } else if (def.kind === 'text' || r.text) {
      const t = r.text || {};
      view = new PIXI.Text({ text: t.content ?? '', style: {
        fontFamily: t.fontFamily || 'system-ui, sans-serif',
        fontSize: t.fontSize ?? 24,
        fill: t.fill ? parseColor(t.fill) : 0xffffff,
        fontWeight: t.fontWeight || 'normal',
        align: t.align || 'center',
        ...(t.stroke ? { stroke: { color: parseColor(t.stroke), width: t.strokeWidth ?? 3 } } : {}),
        ...(t.wordWrapWidth ? { wordWrap: true, wordWrapWidth: t.wordWrapWidth } : {}),
      }});
    } else if (def.assetId && textures.has(def.assetId)) {
      view = new PIXI.Sprite(textures.get(def.assetId));
      view.width = w; view.height = h;
    } else {
      view = new PIXI.Graphics();
      view.roundRect(-w / 2, -h / 2, w, h, def.kind === 'pickup' ? 20 : 10)
        .fill(KIND_COLORS[def.kind] ?? 0xf97316);
    }
    if (view.anchor) view.anchor.set(r.anchor?.[0] ?? 0.5, r.anchor?.[1] ?? 0.5);
    if (r.tint) view.tint = parseColor(r.tint);
    view.alpha = r.alpha ?? 1;
    if (r.blendMode) view.blendMode = r.blendMode;
    const sx = (r.scale?.[0] ?? 1) * (r.flipX ? -1 : 1);
    const sy = (r.scale?.[1] ?? 1) * (r.flipY ? -1 : 1);
    view.scale.set(view.scale.x * sx, view.scale.y * sy);
    view.rotation = (def.rotation ?? 0) * Math.PI / 180;
    view.zIndex = def.zIndex ?? 0;
    view.visible = r.visible ?? true;
    view.position.set(def.position[0], def.position[1]);

    const filters = [];
    for (const f of r.filters || []) {
      try {
        switch (f.type) {
          case 'blur': filters.push(new PIXI.BlurFilter({ strength: f.value ?? 4 })); break;
          case 'alpha': filters.push(new PIXI.AlphaFilter({ alpha: f.value ?? 0.5 })); break;
          case 'noise': filters.push(new PIXI.NoiseFilter({ noise: f.value ?? 0.3 })); break;
          case 'color_matrix': {
            const cm = new PIXI.ColorMatrixFilter();
            const v = f.value ?? 1;
            switch (f.operation || 'brightness') {
              case 'brightness': cm.brightness(v, false); break;
              case 'contrast': cm.contrast(v, false); break;
              case 'saturate': cm.saturate(v, false); break;
              case 'hue': cm.hue(v, false); break;
              case 'grayscale': cm.grayscale(v, false); break;
              case 'invert': case 'negative': cm.negative(false); break;
              case 'sepia': cm.sepia(false); break;
              case 'black_and_white': cm.blackAndWhite(false); break;
              case 'tint': cm.tint(0xffd94d, false); break;
              default: cm.brightness(v, false);
            }
            filters.push(cm);
            break;
          }
          case 'displacement': {
            if (f.assetId && textures.has(f.assetId)) {
              const mapSprite = new PIXI.Sprite(textures.get(f.assetId));
              filters.push(new PIXI.DisplacementFilter(mapSprite, f.scale ?? 20));
            } else {
              warnOnce('disp-' + def.id, 'RT', `displacement filter on '${def.id}' needs a loaded map texture — skipped`);
            }
            break;
          }
          default:
            warnOnce('filter-' + f.type, 'RT', `unknown filter '${f.type}' skipped`);
        }
      } catch (e) {
        warnOnce('filter-err-' + def.id, 'RT', `filter '${f.type}' on '${def.id}' failed: ${e.message}`);
      }
    }
    if (filters.length) view.filters = filters;
    return view;
  }

  function makeBody(def, x, y) {
    const w = Math.max(8, def.size?.[0] ?? 48);
    const h = Math.max(8, def.size?.[1] ?? def.size?.[0] ?? 48);
    const p = def.physics;
    let desc;
    if (!p) {
      desc = RAPIER.RigidBodyDesc.kinematicPositionBased();
    } else {
      switch (p.body) {
        case 'dynamic': desc = RAPIER.RigidBodyDesc.dynamic(); break;
        case 'fixed': desc = RAPIER.RigidBodyDesc.fixed(); break;
        case 'kinematic_velocity': desc = RAPIER.RigidBodyDesc.kinematicVelocityBased(); break;
        default: desc = RAPIER.RigidBodyDesc.kinematicPositionBased();
      }
      if (p.initialVelocity) desc.setLinvel(toM(p.initialVelocity[0]), toM(p.initialVelocity[1]));
      if (p.angularVelocity !== undefined) desc.setAngvel(p.angularVelocity);
      if (p.gravityScale !== undefined) desc.setGravityScale(p.gravityScale);
      if (p.linearDamping !== undefined) desc.setLinearDamping(p.linearDamping);
      if (p.angularDamping !== undefined) desc.setAngularDamping(p.angularDamping);
      if (p.ccd) desc.setCcdEnabled(true);
      if (p.lockRotation) desc.lockRotations();
      if (p.lockTranslation) {
        if (p.lockTranslation[0] && p.lockTranslation[1]) desc.lockTranslations();
        else desc.enabledTranslations(!p.lockTranslation[0], !p.lockTranslation[1]);
      }
      if (p.dominance !== undefined) desc.setDominanceGroup(p.dominance);
      if (p.canSleep !== undefined) desc.setCanSleep(p.canSleep);
    }
    desc.setTranslation(toM(x), toM(y));
    if (def.rotation) desc.setRotation(def.rotation * Math.PI / 180);
    const body = world.createRigidBody(desc);

    let cdesc;
    const shape = p?.shape || 'cuboid';
    const pts = (p?.points || []).map(([px, py]) => [toM(px), toM(py)]);
    switch (shape) {
      case 'ball': cdesc = RAPIER.ColliderDesc.ball(toM(Math.min(w, h) / 2)); break;
      case 'capsule': cdesc = RAPIER.ColliderDesc.capsule(toM(Math.max(0, (h - w) / 2)), toM(w / 2)); break;
      case 'segment':
        cdesc = pts.length === 2
          ? RAPIER.ColliderDesc.segment({ x: pts[0][0], y: pts[0][1] }, { x: pts[1][0], y: pts[1][1] })
          : RAPIER.ColliderDesc.cuboid(toM(w / 2), toM(h / 2));
        break;
      case 'convex': {
        const hull = pts.length >= 3 ? RAPIER.ColliderDesc.convexHull(new Float32Array(pts.flat())) : null;
        if (hull) cdesc = hull;
        else { warnOnce('hull-' + def.id, 'PHYS', `convex hull failed for '${def.id}' — cuboid fallback`); cdesc = RAPIER.ColliderDesc.cuboid(toM(w / 2), toM(h / 2)); }
        break;
      }
      default: cdesc = RAPIER.ColliderDesc.cuboid(toM(w / 2), toM(h / 2));
    }
    if (p) {
      if (p.density !== undefined) cdesc.setDensity(p.density);
      if (p.friction !== undefined) cdesc.setFriction(p.friction);
      if (p.restitution !== undefined) cdesc.setRestitution(p.restitution);
      if (p.frictionCombineRule && COMBINE[p.frictionCombineRule] !== undefined) cdesc.setFrictionCombineRule(COMBINE[p.frictionCombineRule]);
      if (p.restitutionCombineRule && COMBINE[p.restitutionCombineRule] !== undefined) cdesc.setRestitutionCombineRule(COMBINE[p.restitutionCombineRule]);
      if (p.sensor) cdesc.setSensor(true);
      let events = RAPIER.ActiveEvents.COLLISION_EVENTS;
      if (hasContactForceRules) { events |= RAPIER.ActiveEvents.CONTACT_FORCE_EVENTS; cdesc.setContactForceEventThreshold(1e-6); }
      if (p.activeEvents !== false) cdesc.setActiveEvents(events);
      if (p.collisionGroups) {
        const packed = ((groupsMask(p.collisionGroups.memberships) << 16) | groupsMask(p.collisionGroups.filter)) >>> 0;
        cdesc.setCollisionGroups(packed);
        cdesc.setSolverGroups(packed);
      }
    } else {
      cdesc.setSensor(true);
      cdesc.setActiveEvents(RAPIER.ActiveEvents.COLLISION_EVENTS);
    }
    const collider = world.createCollider(cdesc, body);
    return { body, collider };
  }

  // --- Entities -------------------------------------------------------
  const entities = new Map();       // runtimeId -> ent
  const byDefId = new Map();        // def.id -> [ent]
  const byCollider = new Map();     // collider handle -> ent
  const templates = new Map();
  for (const t of scene.templates || []) templates.set(t.id, t);
  let spawnCounter = 0;
  let respawnIndex = 0;

  function register(ent) {
    entities.set(ent.runtimeId, ent);
    if (!byDefId.has(ent.defId)) byDefId.set(ent.defId, []);
    byDefId.get(ent.defId).push(ent);
    if (ent.collider) byCollider.set(ent.collider.handle, ent);
  }
  function unregister(ent) {
    entities.delete(ent.runtimeId);
    const arr = byDefId.get(ent.defId) || [];
    const i = arr.indexOf(ent);
    if (i >= 0) arr.splice(i, 1);
    if (ent.collider) byCollider.delete(ent.collider.handle);
  }
  function findEnt(ref) {
    if (!ref) return undefined;
    return entities.get(ref) || (byDefId.get(ref) || [])[0] ||
      [...entities.values()].find((e) => e.def.kind === ref);
  }

  function createEntity(def, x, y, runtimeId) {
    const view = makeView(def);
    view.position.set(x, y);
    worldLayer.addChild(view);
    const { body, collider } = makeBody(def, x, y);
    const ent = {
      runtimeId: runtimeId || def.id, defId: def.id, def, view, body, collider,
      w: Math.max(8, def.size?.[0] ?? 48), h: Math.max(8, def.size?.[1] ?? def.size?.[0] ?? 48),
      x, y, anchorX: x, anchorY: y,
      baseAlpha: view.alpha, baseScaleX: view.scale.x, baseScaleY: view.scale.y,
      baseRot: view.rotation, offX: 0, offY: 0,
      destroyed: false, phase: Math.random() * Math.PI * 2, stillTime: 0,
      bodyKind: def.physics?.body || (def.physics ? 'kinematic_position' : 'legacy'),
      tweens: (def.tweens || []).map((t) => ({ ...t, t: -(t.delayMs || 0), cycle: 0, done: false })),
      emitter: null, controller: null, forceUntil: 0, forceVec: null,
    };
    if (def.render?.particles) {
      ent.emitter = new Emitter(fxLayer, def.render.particles, textures);
      ent.emitter.x = x; ent.emitter.y = y;
    }
    if (view instanceof PIXI.AnimatedSprite && !(def.render.animation.loop ?? true)) {
      view.onComplete = () => emit('animation_complete', ent, ent);
    }
    view.eventMode = 'static';
    view.on('pointerdown', (ev) => {
      ev.stopPropagation();
      emit('input', ent, ent, undefined, tapActionName());
    });
    register(ent);
    return ent;
  }

  for (const def of scene.entities) createEntity(def, def.position[0], def.position[1], def.id);

  const playerEnt = [...entities.values()].find(
    (e) => e.def.kind === 'player' || (e.def.components || []).includes('movement'));

  // Character controller for the player, when authored.
  if (playerEnt?.def.character) {
    const c = playerEnt.def.character;
    const ctrl = world.createCharacterController(toM(c.offset ?? 4));
    if (c.maxSlopeClimbDegrees !== undefined) ctrl.setMaxSlopeClimbAngle(c.maxSlopeClimbDegrees * Math.PI / 180);
    if (c.minSlopeSlideDegrees !== undefined) ctrl.setMinSlopeSlideAngle(c.minSlopeSlideDegrees * Math.PI / 180);
    if (c.autostepMaxHeight !== undefined) ctrl.enableAutostep(toM(c.autostepMaxHeight), toM(c.autostepMinWidth ?? 8), c.applyImpulsesToDynamicBodies ?? true);
    if (c.snapToGroundDistance !== undefined) ctrl.enableSnapToGround(toM(c.snapToGroundDistance));
    if (c.applyImpulsesToDynamicBodies !== undefined) ctrl.setApplyImpulsesToDynamicBodies(c.applyImpulsesToDynamicBodies);
    playerEnt.controller = ctrl;
    log('PHYS', `character controller on '${playerEnt.defId}' (offset ${c.offset ?? 4}px)`);
  }

  // --- Joints ---------------------------------------------------------
  for (const j of scene.joints || []) {
    const a = findEnt(j.bodyA), b = findEnt(j.bodyB);
    if (!a || !b) { log('PHYS', `WARN joint '${j.id}' references missing body — skipped`); continue; }
    const anchorA = { x: toM(j.anchorA?.[0] ?? 0), y: toM(j.anchorA?.[1] ?? 0) };
    const anchorB = { x: toM(j.anchorB?.[0] ?? 0), y: toM(j.anchorB?.[1] ?? 0) };
    let data = null;
    switch (j.type) {
      case 'fixed': data = RAPIER.JointData.fixed(anchorA, 0, anchorB, 0); break;
      case 'revolute': data = RAPIER.JointData.revolute(anchorA, anchorB); break;
      case 'prismatic': {
        const ax = j.axis || [1, 0];
        const len = Math.hypot(ax[0], ax[1]) || 1;
        data = RAPIER.JointData.prismatic(anchorA, anchorB, { x: ax[0] / len, y: ax[1] / len });
        break;
      }
      case 'rope':
        if (typeof RAPIER.JointData.rope === 'function') data = RAPIER.JointData.rope(toM(j.length ?? 100), anchorA, anchorB);
        else warnOnce('rope', 'PHYS', 'JointData.rope unavailable in this Rapier build — rope joint skipped');
        break;
      case 'spring':
        if (typeof RAPIER.JointData.spring === 'function') data = RAPIER.JointData.spring(toM(j.restLength ?? 50), j.stiffness ?? 20, j.damping ?? 2, anchorA, anchorB);
        else warnOnce('spring', 'PHYS', 'JointData.spring unavailable in this Rapier build — spring joint skipped');
        break;
      default:
        warnOnce('joint-' + j.type, 'PHYS', `unknown joint type '${j.type}' skipped`);
    }
    if (!data) continue;
    if (j.limits && (j.type === 'revolute' || j.type === 'prismatic')) {
      data.limitsEnabled = true;
      data.limits = j.type === 'revolute' ? [j.limits[0], j.limits[1]] : [toM(j.limits[0]), toM(j.limits[1])];
    }
    const joint = world.createImpulseJoint(data, a.body, b.body, true);
    if (j.motor && joint) {
      const m = j.motor;
      try {
        if (m.maxForce !== undefined && typeof joint.configureMotorMaxImpulse === 'function') joint.configureMotorMaxImpulse(m.maxForce);
        if (m.mode === 'velocity' && typeof joint.configureMotorVelocity === 'function') {
          joint.configureMotorVelocity(j.type === 'prismatic' ? toM(m.target) : m.target, m.maxForce ?? 100);
        } else if (m.mode === 'position' && typeof joint.configureMotorPosition === 'function') {
          joint.configureMotorPosition(j.type === 'prismatic' ? toM(m.target) : m.target, m.stiffness ?? 50, m.damping ?? 5);
        }
      } catch (e) {
        warnOnce('motor-' + j.id, 'PHYS', `joint '${j.id}' motor setup failed: ${e.message}`);
      }
    }
    log('PHYS', `joint '${j.id}' (${j.type}) ${j.bodyA} <-> ${j.bodyB}`);
  }

  const sceneEmitters = (scene.emitters || []).map((cfg) => {
    const e = new Emitter(fxLayer, cfg, textures);
    e.x = W / 2; e.y = H / 3;
    return e;
  });
  const burstEmitter = new Emitter(fxLayer, { tint: '#ffd94d', speed: [60, 220], lifetime: [0.35, 0.8], spread: 360, startScale: 0.6, endScale: 0.05 }, textures);

  log('RT', `scene '${scene.id}': ${scene.entities.length} entities (${scene.entities.filter((e) => e.behavior).length} behavior, ${scene.entities.filter((e) => e.physics).length} physics), ${(scene.joints || []).length} joints, ${ir.rules.length} rules, ${textures.size} textures`);

  // --- Rule engine ----------------------------------------------------
  const state = {
    score: 0, health: 3, ended: false, won: false,
    variables: {}, elapsed: 0, depth: 0,
    collectedPairs: new Set(), insideRegions: new Set(),
    firedThresholds: new Set(), timers: new Map(), shake: 0, follow: null,
  };

  const matchesRef = (ref, ent) => ref === undefined || ref === '' || ref === ent.defId || ref === ent.def.kind || ref === ent.runtimeId;

  function lookupVar(name) {
    if (name === 'score') return state.score;
    if (name === 'health') return state.health;
    if (name === 'elapsed' || name === 'time') return state.elapsed;
    const key = name.startsWith('variable:') ? name.slice(9) : name;
    return state.variables[key] ?? 0;
  }
  function conditionsMet(rule) {
    for (const p of rule.conditions || []) {
      const lhs = lookupVar(p.variable);
      const rhs = p.value;
      let ok = false;
      const ln = Number(lhs), rn = Number(rhs);
      if (!Number.isNaN(ln) && !Number.isNaN(rn) && typeof rhs !== 'boolean') {
        switch (p.operator) {
          case 'eq': ok = ln === rn; break;
          case 'neq': ok = ln !== rn; break;
          case 'gt': ok = ln > rn; break;
          case 'gte': ok = ln >= rn; break;
          case 'lt': ok = ln < rn; break;
          case 'lte': ok = ln <= rn; break;
        }
      } else {
        if (p.operator === 'eq') ok = lhs === rhs;
        else if (p.operator === 'neq') ok = lhs !== rhs;
        else ok = false;
      }
      if (!ok) return false;
    }
    return true;
  }

  function ruleMatches(rule, event, source, target, value, inputAction) {
    const when = rule.when || {};
    if (when.event !== event) return false;
    if (source && !matchesRef(when.source, source)) return false;
    if (target && !matchesRef(when.target, target)) return false;
    if (inputAction !== undefined && when.inputAction !== undefined && when.inputAction !== inputAction) return false;
    if (value !== undefined && when.value !== undefined && Number(when.value) !== value) return false;
    return true;
  }

  function emit(event, source, target, value, inputAction) {
    if (state.depth > 8) { warnOnce('depth', 'RULE', 'event cascade depth exceeded — dropped'); return; }
    state.depth++;
    try {
      for (const rule of ir.rules) {
        if (!ruleMatches(rule, event, source, target, value, inputAction)) continue;
        if (!conditionsMet(rule)) continue;
        log('RULE', `rule '${rule.id}' fired by ${event}`);
        execute(rule.actions || [], source, target);
        if (state.ended) break;
      }
    } finally {
      state.depth--;
    }
  }

  function updateHUD() {
    $('score').textContent = `★ ${state.score}  ♥ ${state.health}`;
  }

  function emitScoreThresholds() {
    for (const rule of ir.rules) {
      if (rule.when?.event !== 'score_reached') continue;
      if (state.firedThresholds.has(rule.id)) continue;
      if (!(state.score >= Number(rule.when.value))) continue;
      if (!conditionsMet(rule)) continue;
      state.firedThresholds.add(rule.id);
      log('RULE', `rule '${rule.id}' fired by score_reached (${state.score})`);
      execute(rule.actions || [], playerEnt, undefined);
      if (state.ended) return;
    }
  }

  function recycleEntity(ent) {
    respawnIndex += 1;
    const nx = 50 + ((respawnIndex * 83) % Math.max(60, worldW - 100));
    ent.x = nx; ent.y = -ent.h / 2; ent.stillTime = 0; ent.destroyed = false;
    ent.view.visible = ent.def.render?.visible ?? true;
    if (!entities.has(ent.runtimeId)) register(ent);
    if (worldLayer.children.indexOf(ent.view) < 0) worldLayer.addChild(ent.view);
    if (ent.bodyKind === 'dynamic') {
      ent.body.setTranslation({ x: toM(nx), y: toM(-ent.h / 2) }, true);
      ent.body.setLinvel({ x: 0, y: toM(ent.def.physics?.initialVelocity?.[1] ?? 60) }, true);
      ent.body.setAngvel(0, true);
      ent.body.setRotation(0, true);
    }
    if (playerEnt) state.collectedPairs.delete(playerEnt.runtimeId + ':' + ent.runtimeId);
    log('RT', `recycled '${ent.defId}' to the top`);
  }

  function destroyEntity(ent) {
    if (!ent || ent.destroyed) return;
    if (ent.def.properties?.recycle || ent.def.behavior?.type === 'fall') {
      recycleEntity(ent);
      return;
    }
    ent.destroyed = true;
    unregister(ent);
    try { world.removeRigidBody(ent.body); } catch { /* already gone */ }
    ent.view.removeFromParent();
    emit('entity_destroyed', ent, ent);
  }

  function execute(actions, sourceEnt, targetEnt) {
    for (const action of actions) {
      const ent = findEnt(action.target);
      switch (action.type) {
        case 'add_score':
          state.score += Math.trunc(action.value ?? 1);
          updateHUD();
          emitScoreThresholds();
          break;
        case 'set_variable':
          if (action.variable) {
            state.variables[action.variable] = action.value;
            emit('variable_changed', sourceEnt, targetEnt);
          }
          break;
        case 'move_entity':
          if (ent) {
            if (Array.isArray(action.value)) { ent.x += action.value[0]; ent.y += action.value[1]; }
            else ent.x += Number(action.value ?? 0);
            if (ent.bodyKind === 'dynamic') ent.body.setTranslation({ x: toM(ent.x), y: toM(ent.y) }, true);
          }
          break;
        case 'teleport_entity':
          if (ent && Array.isArray(action.value)) {
            ent.x = action.value[0]; ent.y = action.value[1];
            if (ent.bodyKind === 'dynamic' || ent.bodyKind === 'kinematic_velocity') {
              ent.body.setTranslation({ x: toM(ent.x), y: toM(ent.y) }, true);
            }
          }
          break;
        case 'destroy_entity':
          destroyEntity(ent);
          break;
        case 'spawn_entity': {
          const tpl = templates.get(action.target);
          if (!tpl) { warnOnce('spawn-' + action.target, 'RULE', `spawn_entity: template '${action.target}' missing — skipped`); break; }
          const pos = Array.isArray(action.value) ? action.value
            : [targetEnt?.x ?? tpl.position[0], targetEnt?.y ?? tpl.position[1]];
          const clone = createEntity(JSON.parse(JSON.stringify(tpl)), pos[0], pos[1], tpl.id + '#' + (++spawnCounter));
          log('RULE', `spawned '${clone.runtimeId}' at ${Math.round(pos[0])},${Math.round(pos[1])}`);
          break;
        }
        case 'show_text':
          $('status').textContent = String(action.value ?? '');
          break;
        case 'set_health':
          state.health = Math.max(0, Math.trunc(action.value ?? 0));
          updateHUD();
          emit('health_changed', sourceEnt, targetEnt);
          if (state.health === 0) endGame(false);
          break;
        case 'change_health':
          state.health = Math.max(0, state.health + Math.trunc(action.value ?? 0));
          updateHUD();
          emit('health_changed', sourceEnt, targetEnt);
          if (state.health === 0) endGame(false);
          break;
        case 'end_game':
          endGame(Boolean(action.value));
          break;
        case 'play_sound': {
          const url = audio.get(action.assetId);
          if (!url) { warnOnce('sound-' + action.assetId, 'RULE', `play_sound: audio asset '${action.assetId}' missing — skipped`); break; }
          new Audio(url).play().catch((e) => warnOnce('sound-play', 'RULE', 'audio play blocked: ' + e.message));
          break;
        }
        case 'change_scene': {
          if (!action.sceneId || !(ir.scenes || []).some((s) => s.id === action.sceneId)) {
            warnOnce('scene-' + action.sceneId, 'RULE', `change_scene: scene '${action.sceneId}' missing — skipped`);
            break;
          }
          sessionStorage.setItem(sceneKey, action.sceneId);
          log('RULE', `changing scene to '${action.sceneId}'`);
          location.reload();
          break;
        }
        case 'apply_impulse': {
          if (!ent || !Array.isArray(action.value)) break;
          const dvx = toM(action.value[0]), dvy = toM(action.value[1]);
          if (ent.bodyKind === 'dynamic') {
            const m = ent.body.mass();
            ent.body.applyImpulse({ x: dvx * m, y: dvy * m }, true);
          } else if (ent.bodyKind === 'kinematic_velocity') {
            const v = ent.body.linvel();
            ent.body.setLinvel({ x: v.x + dvx, y: v.y + dvy }, true);
          } else {
            warnOnce('impulse-' + ent.defId, 'RULE', `apply_impulse on ${ent.bodyKind} body '${ent.defId}' ignored (needs dynamic or kinematic_velocity)`);
          }
          break;
        }
        case 'set_velocity':
          if (ent && Array.isArray(action.value)) {
            if (ent.bodyKind === 'dynamic' || ent.bodyKind === 'kinematic_velocity') {
              ent.body.setLinvel({ x: toM(action.value[0]), y: toM(action.value[1]) }, true);
            } else { ent.velX = action.value[0]; ent.velY = action.value[1]; }
          }
          break;
        case 'set_angular_velocity':
          if (ent) ent.body.setAngvel(Number(action.value ?? 0), true);
          break;
        case 'add_force':
          if (ent && Array.isArray(action.value)) {
            ent.forceVec = { x: toM(action.value[0]), y: toM(action.value[1]) };
            ent.forceUntil = state.elapsed + (action.durationMs ?? 100) / 1000;
          }
          break;
        case 'set_gravity':
          if (Array.isArray(action.value)) world.gravity = { x: toM(action.value[0]), y: toM(action.value[1]) };
          break;
        case 'set_alpha':
          if (ent) { ent.view.alpha = Number(action.value); ent.baseAlpha = ent.view.alpha; ent.tweenAlpha = undefined; }
          break;
        case 'set_tint':
          if (ent) ent.view.tint = parseColor(action.value);
          break;
        case 'set_visible':
          if (ent) ent.view.visible = Boolean(action.value);
          break;
        case 'set_asset':
          if (!ent || !action.assetId || !textures.has(action.assetId)) {
            warnOnce('asset-' + action.assetId, 'RULE', `set_asset: texture '${action.assetId}' unavailable — skipped`);
            break;
          }
          if (ent.view instanceof PIXI.Sprite) ent.view.texture = textures.get(action.assetId);
          else warnOnce('asset-view-' + ent.defId, 'RULE', `set_asset on non-sprite '${ent.defId}' skipped`);
          break;
        case 'emit_particles': {
          const at = ent || targetEnt || sourceEnt;
          const n = Math.trunc(action.value ?? 16);
          if (at?.emitter) at.emitter.burst(n, at.x, at.y);
          else if (at) burstEmitter.burst(n, at.x, at.y);
          break;
        }
        case 'camera_shake':
          state.shake = Math.max(state.shake, Number(action.value ?? 5));
          break;
        case 'camera_follow':
          if (ent) state.follow = ent;
          break;
        default:
          warnOnce('action-' + action.type, 'RULE', `unknown action '${action.type}' skipped`);
      }
      if (state.ended) break;
    }
  }

  function endGame(won) {
    if (state.ended) return;
    state.ended = true;
    state.won = won;
    log('RT', `game ended: ${won ? 'WIN' : 'LOSE'}, score ${state.score}`);
    $('end-title').textContent = won ? 'You did it!' : 'Game over';
    $('end-sub').textContent = 'Score: ' + state.score;
    $('end').style.display = 'block';
  }

  $('again').addEventListener('click', () => location.reload());

  // --- Input ----------------------------------------------------------
  function tapActionName() {
    const b = (ir.inputBindings || []).find((x) => x.input === 'tap');
    return b?.action;
  }
  const pointer = { down: false, x: W / 2, y: H / 2 };
  const playerTarget = playerEnt ? { x: playerEnt.x, y: playerEnt.y } : { x: W / 2, y: H / 2 };
  const toLogical = (ev) => {
    const rect = app.canvas.getBoundingClientRect();
    return {
      x: (ev.clientX - rect.left) * (W / rect.width),
      y: (ev.clientY - rect.top) * (H / rect.height),
    };
  };
  app.canvas.addEventListener('pointerdown', (ev) => {
    pointer.down = true;
    const p = toLogical(ev); pointer.x = p.x; pointer.y = p.y;
    emit('input', playerEnt, undefined, undefined, tapActionName());
  });
  app.canvas.addEventListener('pointermove', (ev) => {
    if (!pointer.down) return;
    const p = toLogical(ev); pointer.x = p.x; pointer.y = p.y;
  });
  addEventListener('pointerup', () => { pointer.down = false; });

  const keysDown = new Set();
  const kbBindings = (ir.inputBindings || []).filter((b) => b.input === 'keyboard');
  addEventListener('keydown', (ev) => {
    const b = kbBindings.find((x) => x.key === ev.key || x.action === ev.key);
    if (!b) return;
    keysDown.add(b.action);
    emit('input', playerEnt, undefined, undefined, b.action);
  });
  addEventListener('keyup', (ev) => {
    const b = kbBindings.find((x) => x.key === ev.key || x.action === ev.key);
    if (b) keysDown.delete(b.action);
  });

  // --- Contacts -------------------------------------------------------
  function handleContact(a, b, started) {
    if (!playerEnt) return;
    const other = a === playerEnt ? b : b === playerEnt ? a : null;
    if (!other) return;
    const key = playerEnt.runtimeId + ':' + other.runtimeId;
    if (!started) {
      state.collectedPairs.delete(key);
      if (state.insideRegions.has(key)) {
        state.insideRegions.delete(key);
        emit('region_exited', playerEnt, other);
      }
      return;
    }
    if (other.destroyed) return;
    const isPickup = other.def.kind === 'pickup' || (other.def.components || []).includes('collectible');
    const isTrigger = other.def.kind === 'trigger';
    const isHazard = other.def.kind === 'obstacle' || other.def.kind === 'enemy';
    if (isPickup) {
      if (!state.collectedPairs.has(key)) {
        state.collectedPairs.add(key);
        emit('collected', playerEnt, other);
      }
    } else if (isTrigger) {
      if (!state.insideRegions.has(key)) {
        state.insideRegions.add(key);
        emit('region_entered', playerEnt, other);
      }
    } else if (isHazard) {
      log('PHYS', `hazard contact: ${other.defId}`);
      emit('collision', playerEnt, other);
    }
  }

  function drainEvents() {
    eventQueue.drainCollisionEvents((h1, h2, started) => {
      const a = byCollider.get(h1), b = byCollider.get(h2);
      if (!a || !b || a === b) return;
      emit(started ? 'contact_started' : 'contact_stopped', a, b);
      handleContact(a, b, started);
    });
    if (hasContactForceRules) {
      eventQueue.drainContactForceEvents((ev) => {
        const a = byCollider.get(ev.collider1().handle);
        const b = byCollider.get(ev.collider2().handle);
        if (!a || !b) return;
        const mag = typeof ev.maxForceMagnitude === 'function' ? ev.maxForceMagnitude() : 0;
        emit('contact_force', a, b, mag);
      });
    }
  }

  // --- Behaviors ------------------------------------------------------
  function applyBehaviorPosition(ent, b, dt) {
    switch (b.type) {
      case 'fall':
        ent.y += (b.speed ?? 80) * dt;
        if (ent.y > worldH + ent.h / 2) {
          respawnIndex += 1;
          ent.x = 50 + ((respawnIndex * 83) % Math.max(60, worldW - 100));
          ent.y = -ent.h / 2;
        }
        break;
      case 'chase':
        if (playerEnt && ent !== playerEnt) {
          const dx = playerEnt.x - ent.x, dy = playerEnt.y - ent.y;
          const dist = Math.max(1, Math.hypot(dx, dy));
          const step = Math.min((b.speed ?? 50) * dt, dist);
          ent.x += (dx / dist) * step;
          ent.y += (dy / dist) * step;
        }
        break;
      case 'patrol':
        ent.x = ent.anchorX + Math.sin(state.elapsed * (b.speed ?? 1)) * (b.range ?? 60);
        break;
      case 'oscillate':
        ent.y = ent.anchorY + Math.sin(state.elapsed * (b.speed ?? 1.2)) * (b.range ?? 10);
        break;
      case 'drift':
        ent.x = ent.anchorX + Math.sin(state.elapsed * (b.speed ?? 0.6)) * (b.range ?? 24);
        ent.y = ent.anchorY + Math.cos(state.elapsed * (b.speed ?? 0.6) * 0.8) * (b.range ?? 24) * 0.6;
        break;
      case 'orbit':
        ent.x = ent.anchorX + Math.cos(state.elapsed * (b.speed ?? 1)) * (b.range ?? 40);
        ent.y = ent.anchorY + Math.sin(state.elapsed * (b.speed ?? 1)) * (b.range ?? 40);
        break;
      case 'spin':
        ent.view.rotation += (b.speed ?? 90) * dt * Math.PI / 180;
        break;
      case 'pulse':
        break; // scale pulse applied at view sync
      default:
        warnOnce('behavior-' + b.type, 'RT', `unknown behavior '${b.type}' on '${ent.defId}' ignored`);
        ent.def.behavior = undefined;
    }
  }

  function applyBehaviorVelocity(ent, b) {
    const w = b.speed ?? 1;
    switch (b.type) {
      case 'fall':
        ent.body.setLinvel({ x: 0, y: toM(b.speed ?? 80) }, true);
        break;
      case 'chase':
        if (playerEnt && ent !== playerEnt) {
          const dx = playerEnt.x - ent.x, dy = playerEnt.y - ent.y;
          const d = Math.max(1, Math.hypot(dx, dy));
          const s = toM(b.speed ?? 50);
          ent.body.setLinvel({ x: (dx / d) * s, y: (dy / d) * s }, true);
        }
        break;
      case 'patrol':
        ent.body.setLinvel({ x: toM(Math.cos(state.elapsed * w) * (b.range ?? 60) * w), y: 0 }, true);
        break;
      case 'oscillate':
        ent.body.setLinvel({ x: 0, y: toM(Math.cos(state.elapsed * w) * (b.range ?? 10) * w) }, true);
        break;
      default:
        warnOnce('bv-' + b.type, 'RT', `behavior '${b.type}' on kinematic_velocity body '${ent.defId}' ignored`);
    }
  }

  function updateTweens(ent, dt) {
    for (const tw of ent.tweens) {
      if (tw.done) continue;
      tw.t += dt * 1000;
      if (tw.t < 0) continue;
      const dur = Math.max(10, tw.durationMs);
      const pos = tw.t / dur;
      const cycle = Math.floor(pos);
      const allowed = tw.repeat === -1 ? Infinity : (tw.repeat ?? 0) + 1;
      let frac = pos - cycle;
      if (cycle >= allowed) {
        frac = tw.yoyo && allowed % 2 === 0 ? 0 : 1;
        tw.done = true;
      } else if (tw.yoyo && cycle % 2 === 1) {
        frac = 1 - frac;
      }
      const eased = (EASINGS[tw.easing || 'linear'] || EASINGS.linear)(clamp(frac, 0, 1));
      const v = tw.from + (tw.to - tw.from) * eased;
      switch (tw.property) {
        case 'alpha': ent.tweenAlpha = v; break;
        case 'scale': ent.scaleFx = v; ent.scaleFy = v; break;
        case 'scale_x': ent.scaleFx = v; break;
        case 'scale_y': ent.scaleFy = v; break;
        case 'rotation': ent.tweenRot = v; break;
        case 'offset_x': ent.offX = v; break;
        case 'offset_y': ent.offY = v; break;
      }
    }
  }

  // --- Main loop --------------------------------------------------------
  if (!state.follow && playerEnt && (worldW > W || worldH > H)) state.follow = playerEnt;
  let accumulator = 0;

  app.ticker.add((ticker) => {
    if (state.ended) return;
    const dt = Math.min(ticker.deltaMS / 1000, 0.05);
    state.elapsed += dt;

    // Timer rules.
    for (const rule of ir.rules) {
      if (rule.when?.event !== 'timer_elapsed') continue;
      const interval = Number(rule.when.value) || 1;
      const t = (state.timers.get(rule.id) || 0) + dt;
      if (t >= interval) {
        state.timers.set(rule.id, 0);
        emit('timer_elapsed', undefined, undefined, interval);
        if (state.ended) return;
      } else {
        state.timers.set(rule.id, t);
      }
    }

    // Player intent.
    if (playerEnt && !playerEnt.destroyed) {
      const speed = 300;
      if (keysDown.has('move_left')) playerTarget.x -= speed * dt;
      if (keysDown.has('move_right')) playerTarget.x += speed * dt;
      if (keysDown.has('move_up')) playerTarget.y -= speed * dt;
      if (keysDown.has('move_down')) playerTarget.y += speed * dt;
      if (pointer.down) { playerTarget.x = pointer.x; playerTarget.y = pointer.y; }
      playerTarget.x = clamp(playerTarget.x, playerEnt.w / 2, worldW - playerEnt.w / 2);
      playerTarget.y = clamp(playerTarget.y, playerEnt.h / 2, worldH - playerEnt.h / 2);
      if (playerEnt.bodyKind === 'kinematic_velocity' || playerEnt.bodyKind === 'dynamic') {
        const vx = clamp((playerTarget.x - playerEnt.x) * 8, -650, 650);
        const vy = clamp((playerTarget.y - playerEnt.y) * 8, -650, 650);
        playerEnt.body.setLinvel({ x: toM(vx), y: toM(vy) }, true);
      } else {
        let nx = playerTarget.x, ny = playerTarget.y;
        if (playerEnt.controller) {
          playerEnt.controller.computeColliderMovement(playerEnt.collider,
            { x: toM(nx - playerEnt.x), y: toM(ny - playerEnt.y) });
          const mv = playerEnt.controller.computedMovement();
          nx = playerEnt.x + mv.x * PPM;
          ny = playerEnt.y + mv.y * PPM;
        }
        playerEnt.x = nx; playerEnt.y = ny;
      }
    }

    // Per-entity updates.
    for (const ent of entities.values()) {
      if (ent.destroyed) continue;
      const b = ent.def.behavior;
      if (ent.bodyKind === 'dynamic') {
        const sway = ent.def.properties?.sway;
        if (sway) {
          const v = ent.body.linvel();
          ent.body.setLinvel({ x: Math.sin(state.elapsed * 1.9 + ent.phase) * toM(sway), y: v.y }, true);
        }
        if (ent.def.properties?.recycle) {
          const t = ent.body.translation();
          const v = ent.body.linvel();
          if (t.y * PPM > worldH + ent.h) recycleEntity(ent);
          else if (Math.hypot(v.x, v.y) < 0.2 && t.y * PPM > worldH * 0.5) {
            ent.stillTime += dt;
            if (ent.stillTime > 1.4) recycleEntity(ent);
          } else ent.stillTime = 0;
        }
      } else if (ent.bodyKind === 'kinematic_velocity') {
        if (b) applyBehaviorVelocity(ent, b);
        else if (ent.velX !== undefined) ent.body.setLinvel({ x: toM(ent.velX), y: toM(ent.velY || 0) }, true);
      } else if (b) {
        applyBehaviorPosition(ent, b, dt);
      }
      if (ent.forceVec) {
        if (state.elapsed <= ent.forceUntil) ent.body.addForce(ent.forceVec, true);
        else { ent.body.resetForces(true); ent.forceVec = null; }
      }
    }

    // Fixed-step physics.
    accumulator = Math.min(accumulator + dt, 0.12);
    while (accumulator >= 1 / 60) {
      for (const ent of entities.values()) {
        if (ent.destroyed) continue;
        if (ent.bodyKind === 'kinematic_position' || ent.bodyKind === 'legacy') {
          ent.body.setNextKinematicTranslation({ x: toM(ent.x), y: toM(ent.y) });
        }
      }
      world.step(eventQueue);
      accumulator -= 1 / 60;
    }
    drainEvents();
    if (state.ended) return;

    // View sync, tweens, emitters, camera.
    state.shake *= Math.pow(0.002, dt);
    if (state.shake < 0.05) state.shake = 0;
    let camX = 0, camY = 0;
    if (state.follow && !state.follow.destroyed) {
      camX = clamp(state.follow.x - W / 2, 0, Math.max(0, worldW - W));
      camY = clamp(state.follow.y - H / 2, 0, Math.max(0, worldH - H));
    }
    const shX = state.shake ? rand(-state.shake, state.shake) : 0;
    const shY = state.shake ? rand(-state.shake, state.shake) : 0;
    worldLayer.position.set(-camX + shX, -camY + shY);
    for (const l of layers) l.sprite.position.set(-camX * l.px + shX * l.px, -camY * l.py + shY * l.py);

    for (const ent of entities.values()) {
      if (ent.destroyed) continue;
      if (ent.bodyKind === 'dynamic' || ent.bodyKind === 'kinematic_velocity') {
        const t = ent.body.translation();
        ent.x = t.x * PPM; ent.y = t.y * PPM;
        ent.view.rotation = ent.body.rotation();
      }
      updateTweens(ent, dt);
      ent.view.position.set(ent.x + ent.offX, ent.y + ent.offY);
      const pulse = ent.def.behavior?.type === 'pulse'
        ? 1 + 0.1 * Math.sin(state.elapsed * (ent.def.behavior.speed ?? 2)) : 1;
      ent.view.scale.set(
        ent.baseScaleX * (ent.scaleFx ?? 1) * pulse,
        ent.baseScaleY * (ent.scaleFy ?? 1) * pulse);
      if (ent.tweenAlpha !== undefined) ent.view.alpha = ent.tweenAlpha;
      if (ent.tweenRot !== undefined && ent.bodyKind !== 'dynamic' && ent.bodyKind !== 'kinematic_velocity') {
        ent.view.rotation = ent.baseRot + ent.tweenRot * Math.PI / 180;
      }
      if (ent.emitter) {
        ent.emitter.x = ent.x + ent.offX;
        ent.emitter.y = ent.y + ent.offY;
        ent.emitter.update(dt);
      }
    }
    for (const e of sceneEmitters) e.update(dt);
    burstEmitter.update(dt);
  });

  updateHUD();
  $('status').textContent = 'Drag, tap, or use the arrow keys';
  emit('game_start', undefined, undefined);
  emit('scene_entered', undefined, undefined);
  log('RT', `start '${ir.game.id}' scene '${scene.id}'`);
}

if (!gameId) {
  listGames();
} else {
  run(gameId).catch((err) => {
    $('status').textContent = 'Failed to load: ' + err.message;
    log('PKG', 'ERROR ' + err.message);
    if (err.stack) log('PKG', 'STACK ' + err.stack.split('\n').slice(0, 6).join(' <- '));
  });
}
