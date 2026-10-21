// pixi-runtime.js — the single Game IR runtime (PixiJS + Rapier).
//
// Since the 2026-10-20 consolidation this is the ONLY runtime for Game IR
// packages: PixiJS renders, Rapier provides the physics/contact layer,
// and this module interprets the IR contract — scenes, entities, motion
// behaviors, rules — exactly as the retired Defold and Swift
// interpreters did. Physics is the expressive layer: entity motion and
// contacts are simulated, rule events (collected / collision /
// score_reached) are derived from Rapier intersection pairs.
//
// Contract summary (see docs/game-package-ir-v1.md):
//   GET /v1/games/{id}/package -> { packageVersion: "2.0", gameId, ir, assets[] }
//   Logical stage: 390 x 844, entity positions are centers.
//   Behaviors: fall (wraps; collected falling pickups respawn at the
//   top), chase (toward the player), patrol / oscillate (sine around
//   the authored anchor). Unsupported rule actions fail closed with a
//   dev-log warning, never a fake execution.

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

const params = new URLSearchParams(location.search);
const gameId = params.get('game');

if (!gameId) {
  listGames();
} else {
  run(gameId).catch((err) => {
    $('status').textContent = 'Failed to load: ' + err.message;
    log('PKG', 'ERROR ' + err.message);
  });
}

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
  if (ir.irVersion !== '1.0') throw new Error('unsupported irVersion ' + ir.irVersion);
  const scene = (ir.scenes || []).find((s) => s.id === ir.game.entryScene) || ir.scenes?.[0];
  if (!scene) throw new Error('entry scene missing');

  $('title').textContent = ir.game.title || id;

  // --- Pixi stage -----------------------------------------------------
  const app = new PIXI.Application();
  await app.init({ width: W, height: H, background: 0x16204a, antialias: true });
  $('stage').appendChild(app.canvas);

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

  if (scene.backgroundAssetId && textures.has(scene.backgroundAssetId)) {
    const bg = new PIXI.Sprite(textures.get(scene.backgroundAssetId));
    bg.width = W; bg.height = H;
    app.stage.addChild(bg);
  }

  // --- Rapier world ---------------------------------------------------
  await RAPIER.init();
  const world = new RAPIER.World({ x: 0, y: 0 });

  // --- Entities -------------------------------------------------------
  const KIND_COLORS = { player: 0x22d3ee, pickup: 0xec4899, enemy: 0xef4444, obstacle: 0xef4444, text: 0xffffff };
  const entities = new Map();
  for (const def of scene.entities) {
    const w = Math.max(8, def.size?.[0] ?? 48);
    const h = Math.max(8, def.size?.[1] ?? def.size?.[0] ?? 48);
    let view;
    if (def.assetId && textures.has(def.assetId)) {
      view = new PIXI.Sprite(textures.get(def.assetId));
      view.anchor.set(0.5);
      view.width = w; view.height = h;
    } else {
      view = new PIXI.Graphics();
      view.roundRect(-w / 2, -h / 2, w, h, def.kind === 'pickup' ? 20 : 10)
        .fill(KIND_COLORS[def.kind] ?? 0xf97316);
    }
    view.position.set(def.position[0], def.position[1]);
    app.stage.addChild(view);

    const body = world.createRigidBody(
      RAPIER.RigidBodyDesc.kinematicPositionBased().setTranslation(def.position[0], def.position[1]));
    const collider = world.createCollider(
      RAPIER.ColliderDesc.cuboid(w / 2, h / 2).setSensor(true), body);

    entities.set(def.id, {
      def, view, body, collider, w, h,
      x: def.position[0], y: def.position[1],
      anchorX: def.position[0], anchorY: def.position[1],
      destroyed: false,
    });
  }
  const playerEnt = [...entities.values()].find(
    (e) => e.def.kind === 'player' || (e.def.components || []).includes('movement'));
  log('RT', `scene '${scene.id}': ${scene.entities.length} entities (${scene.entities.filter((e) => e.behavior).length} animated), ${ir.rules.length} rules, ${textures.size} textures`);

  // --- Rule engine ----------------------------------------------------
  const state = {
    score: 0, health: 3, ended: false, won: false,
    variables: {}, elapsed: 0, respawnIndex: 0,
    collectedPairs: new Set(), hazardContacts: new Set(), firedThresholds: new Set(),
  };

  const matchesRef = (ref, ent) => ref === undefined || ref === ent.def.id || ref === ent.def.kind;

  function ruleMatches(rule, event, source, target, value) {
    const when = rule.when || {};
    if (when.event !== event) return false;
    if (source && !matchesRef(when.source, source)) return false;
    if (target && !matchesRef(when.target, target)) return false;
    if (value !== undefined && when.value !== undefined && Number(when.value) !== value) return false;
    return true;
  }

  function emit(event, source, target, value) {
    for (const rule of ir.rules) {
      if (!ruleMatches(rule, event, source, target, value)) continue;
      if ((rule.conditions || []).length) {
        log('RULE', `WARN rule '${rule.id}' has conditions; predicate evaluation not in runtime v1 — skipped (fail-closed)`);
        continue;
      }
      log('RULE', `rule '${rule.id}' fired by ${event}`);
      execute(rule.actions || []);
      if (state.ended) break;
    }
  }

  function execute(actions) {
    for (const action of actions) {
      switch (action.type) {
        case 'add_score': {
          state.score += Math.trunc(action.value ?? 1);
          $('score').textContent = '★ ' + state.score;
          emitScoreThresholds();
          break;
        }
        case 'set_variable':
          if (action.variable) state.variables[action.variable] = Number(action.value ?? 0);
          break;
        case 'move_entity': {
          const ent = entities.get(action.target);
          if (ent) ent.x += Number(action.value ?? 0);
          break;
        }
        case 'destroy_entity': {
          const ent = entities.get(action.target);
          if (ent && !ent.destroyed) {
            ent.destroyed = true;
            ent.view.visible = false;
            ent.body.setNextKinematicTranslation({ x: -9999, y: -9999 });
          }
          break;
        }
        case 'show_text':
          $('status').textContent = String(action.value ?? '');
          break;
        case 'set_health':
          state.health = Math.max(0, Math.trunc(action.value ?? 0));
          if (state.health === 0) endGame(false);
          break;
        case 'end_game':
          endGame(Boolean(action.value));
          break;
        case 'spawn_entity': case 'change_scene': case 'play_sound':
          log('RULE', `WARN unsupported action '${action.type}' skipped (fail-closed)`);
          break;
        default:
          log('RULE', `WARN unknown action '${action.type}' skipped`);
      }
    }
  }

  function emitScoreThresholds() {
    for (const rule of ir.rules) {
      if (rule.when?.event !== 'score_reached') continue;
      if (state.firedThresholds.has(rule.id)) continue;
      if (Number(rule.when.value) !== state.score) continue;
      state.firedThresholds.add(rule.id);
      log('RULE', `rule '${rule.id}' fired by score_reached (${state.score})`);
      execute(rule.actions || []);
      if (state.ended) return;
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

  // --- Input: drag the player -----------------------------------------
  let dragging = false;
  const toLogical = (ev) => {
    const rect = app.canvas.getBoundingClientRect();
    return {
      x: (ev.clientX - rect.left) * (W / rect.width),
      y: (ev.clientY - rect.top) * (H / rect.height),
    };
  };
  app.canvas.addEventListener('pointerdown', (ev) => {
    dragging = true;
    movePlayer(toLogical(ev));
  });
  app.canvas.addEventListener('pointermove', (ev) => {
    if (dragging) movePlayer(toLogical(ev));
  });
  addEventListener('pointerup', () => { dragging = false; });

  function movePlayer(p) {
    if (state.ended || !playerEnt) return;
    playerEnt.x = Math.min(W - playerEnt.w / 2, Math.max(playerEnt.w / 2, p.x));
    playerEnt.y = Math.min(H - playerEnt.h / 2, Math.max(playerEnt.h / 2, p.y));
  }

  // --- Main loop: behaviors + physics contacts ------------------------
  function overlaps(a, b) {
    let hit = false;
    world.intersectionPair(a.collider, b.collider, (h) => { hit = h; return true; });
    return hit;
  }

  function respawnColumn(ent) {
    state.respawnIndex += 1;
    ent.x = 50 + ((state.respawnIndex * 83) % 290);
    ent.y = -ent.h / 2;
  }

  app.ticker.add((ticker) => {
    if (state.ended) return;
    const dt = Math.min(ticker.deltaMS / 1000, 0.05);
    state.elapsed += dt;

    for (const ent of entities.values()) {
      const b = ent.def.behavior;
      if (ent.destroyed) {
        // Falling pickups are renewable rain: respawn at the top.
        if (b?.type === 'fall') {
          ent.destroyed = false;
          ent.view.visible = true;
          respawnColumn(ent);
          if (playerEnt) state.collectedPairs.delete(playerEnt.def.id + ':' + ent.def.id);
        }
        continue;
      }
      if (!b || !playerEnt) continue;
      switch (b.type) {
        case 'fall':
          ent.y += (b.speed ?? 80) * dt;
          if (ent.y > H + ent.h / 2) respawnColumn(ent);
          break;
        case 'chase': {
          const dx = playerEnt.x - ent.x, dy = playerEnt.y - ent.y;
          const dist = Math.max(1, Math.hypot(dx, dy));
          const step = Math.min((b.speed ?? 50) * dt, dist);
          ent.x += (dx / dist) * step;
          ent.y += (dy / dist) * step;
          break;
        }
        case 'patrol':
          ent.x = ent.anchorX + Math.sin(state.elapsed * (b.speed ?? 1)) * (b.range ?? 60);
          break;
        case 'oscillate':
          ent.y = ent.anchorY + Math.sin(state.elapsed * (b.speed ?? 1.2)) * (b.range ?? 10);
          break;
        default:
          log('RT', `WARN unknown behavior '${b.type}' on '${ent.def.id}' ignored`);
          ent.def.behavior = undefined;
      }
    }

    // Sync physics + views.
    for (const ent of entities.values()) {
      if (ent.destroyed) continue;
      ent.body.setNextKinematicTranslation({ x: ent.x, y: ent.y });
      ent.view.position.set(ent.x, ent.y);
    }
    if (playerEnt && !playerEnt.destroyed) {
      playerEnt.body.setNextKinematicTranslation({ x: playerEnt.x, y: playerEnt.y });
      playerEnt.view.position.set(playerEnt.x, playerEnt.y);
    }
    world.step();

    if (!playerEnt) return;
    // Contacts -> IR events.
    for (const ent of entities.values()) {
      if (ent === playerEnt || ent.destroyed) continue;
      const kind = ent.def.kind;
      const isPickup = kind === 'pickup' || (ent.def.components || []).includes('collectible');
      const isHazard = kind === 'obstacle' || kind === 'enemy';
      if (!isPickup && !isHazard) continue;
      const hit = overlaps(playerEnt, ent);
      const key = playerEnt.def.id + ':' + ent.def.id;
      if (isPickup) {
        if (hit && !state.collectedPairs.has(key)) {
          state.collectedPairs.add(key);
          emit('collected', playerEnt, ent);
        } else if (!hit) {
          state.collectedPairs.delete(key);
        }
      } else if (hit) {
        if (!state.hazardContacts.has(ent.def.id)) {
          state.hazardContacts.add(ent.def.id);
          log('PHYS', `hazard contact: ${ent.def.id}`);
          emit('collision', playerEnt, ent);
        }
      } else {
        state.hazardContacts.delete(ent.def.id);
      }
      if (state.ended) break;
    }
  });

  $('score').textContent = '★ 0';
  $('status').textContent = 'Drag the character to play';
  emit('game_start', undefined, undefined);
  log('RT', `start '${ir.game.id}': drag to move`);
}
