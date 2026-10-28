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
//   GET /v1/games/{id}/package -> { packageFormatVersion: "2.0", minRuntimeVersion, gameId, ir, assets[] }
//   Logical stage: 390 x 844, entity positions are centers, sizes px.
//   Rapier works in meters internally (scene.pixelsPerMeter, default
//   100 px = 1 m); IR values stay in logical px / px-per-second.

import * as PIXI from 'pixi.js';
import RAPIER from '@dimforge/rapier2d-compat';

const W = 390, H = 844;
const $ = (id) => document.getElementById(id);
const logEl = $('devlog');
const logBuffer = [];

function log(cat, msg) {
  const t = new Date();
  const ts = t.toTimeString().slice(0, 8) + '.' + String(t.getMilliseconds()).padStart(3, '0');
  const line = `[${ts}] [${cat}] ${msg}`;
  console.log(line);
  logBuffer.push(line);
  if (logBuffer.length > 50) logBuffer.shift();
  // Keep both the buffer and visible DOM bounded for long play sessions.
  logEl.textContent = logBuffer.join('\n') + '\n';
}

const warned = new Set();
function warnOnce(key, cat, msg) {
  if (warned.has(key)) return;
  warned.add(key);
  log(cat, 'WARN ' + msg);
}

// --- Observability (auto detect-and-fix flow) ---------------------------
// The page reports its own failures. Errors are buffered for the ?probe=1
// verdict and beaconed to the backend, which groups them by fingerprint so
// the same failure across games reads as one incident. iOS receives only a
// narrow post-verification run-result notification; it is not a control API.
const RUNTIME_VERSION = '2026.10.23.2';

function compareRuntimeVersions(current, minimum) {
  const parse = (value) => {
    if (typeof value !== 'string' || !/^\d+(?:\.\d+)*$/.test(value)) return null;
    return value.split('.').map(Number);
  };
  const a = parse(current), b = parse(minimum);
  if (!a || !b) return null;
  for (let i = 0; i < Math.max(a.length, b.length); i++) {
    const left = a[i] ?? 0, right = b[i] ?? 0;
    if (left !== right) return left < right ? -1 : 1;
  }
  return 0;
}
const errorBuffer = [];
let totalFrames = 0;
let lastCensus = '';
const probeState = { booted: false };
let beaconCount = 0;

function recordError(kind, err) {
  const rec = {
    kind,
    name: err?.name || 'Error',
    message: String(err?.message || err),
    stack: String(err?.stack || '').split('\n').slice(0, 8).join('\n'),
    at: new Date().toISOString(),
  };
  errorBuffer.push(rec);
  if (errorBuffer.length > 10) errorBuffer.shift();
  if (beaconCount < 3 && navigator.sendBeacon) {
    beaconCount++;
    try {
      const payload = JSON.stringify({
        gameId: new URLSearchParams(location.search).get('game'),
        runtimeVersion: RUNTIME_VERSION,
        userAgent: navigator.userAgent,
        ...rec,
        logTail: logBuffer.slice(-20),
      });
      navigator.sendBeacon('/v1/telemetry/runtime-error', new Blob([payload], { type: 'application/json' }));
    } catch { /* telemetry must never break the game */ }
  }
  return rec;
}
window.__dqErrors = errorBuffer;

function emitProbe() {
  const play = window.__dqPlay || null;
  const visibleSec = play ? play.visibleMs / 1000 : 0;
  // Graded performance measurements (scorecard fusion, Phase D): the
  // same frame samples telemetry aggregates, summarized for the
  // scorer — jank share and p95 frame time over visible play.
  let jankRate = null, p95FrameMs = null;
  if (play && play.frameSamples && play.frameSamples.length >= 30) {
    const sorted = [...play.frameSamples].sort((a, b) => a - b);
    p95FrameMs = Math.round(sorted[Math.min(sorted.length - 1, Math.floor(sorted.length * 0.95))]);
    jankRate = Math.round(1000 * sorted.filter((x) => x > 33).length / sorted.length) / 1000;
  }
  const verdict = {
    gameId: new URLSearchParams(location.search).get('game'),
    runtimeVersion: RUNTIME_VERSION,
    quality: qualityMode,
    // fps over VISIBLE play time only (Phase A): hidden/loading time
    // no longer dilutes the number.
    fps: visibleSec > 1 ? Math.round(totalFrames / visibleSec) : 0,
    seed: play ? play.seed : null,
    shell: play && play.shell ? { archetype: play.shell.archetype || '', started: play.shellStarted } : null,
    events: dqEvents.slice(-40),
    inputs: play ? play.inputLog.length : 0,
    deadInputs: play ? play.deadInputs : 0,
    score: play && typeof play.score === 'number' ? play.score : null,
    jankRate,
    p95FrameMs,
    deadInputRate: play && play.inputLog.length > 0
      ? Math.round(1000 * play.deadInputs / play.inputLog.length) / 1000 : null,
    firstInputMs: play && play.firstInputAt ? play.firstInputAt : null,
    firstRewardMs: play && play.firstRewardAt ? play.firstRewardAt : null,
    scriptEndScore: play && typeof play.scriptEndScore === 'number' ? play.scriptEndScore : null,
    replay: window.__dqReplay || null,
    lastRunId: play ? (play.lastRunId || null) : null,
    booted: probeState.booted,
    frames: totalFrames,
    errors: errorBuffer,
    warnCount: logBuffer.filter((l) => l.includes(' WARN ')).length,
    census: lastCensus,
    hud: $('score')?.textContent || '',
    status: $('status')?.textContent || '',
    logTail: logBuffer.slice(-20),
  };
  window.__dqProbe = verdict;
  let el = $('probe-result');
  if (!el) {
    el = document.createElement('pre');
    el.id = 'probe-result';
    document.body.appendChild(el);
  }
  el.textContent = JSON.stringify(verdict, null, 1);
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

// --- Seeded RNG (Phase A) ----------------------------------------------
// All gameplay randomness flows through one mulberry32 generator so a
// run is reproducible from its seed: the playtest determinism check,
// the difficulty tuner, and leaderboard replay verification all depend
// on seed + input trace -> identical outcome. The seed comes from
// ?seed= when present (tests, replays), else a per-session random seed
// that is reported in telemetry and the probe verdict.
let rngState = 0;
function srand(seed) { rngState = seed >>> 0 || 1; }
function rng() {
  rngState = (rngState + 0x6D2B79F5) >>> 0;
  let t = rngState;
  t = Math.imul(t ^ (t >>> 15), t | 1);
  t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
  return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
}
const rand = (a, b) => a + rng() * (b - a);

// --- Session event stream + telemetry v2 (Phase A) --------------------
// dqEvents is the in-page record the probe verdict and the playtest
// worker read: timestamped gameplay events (start, first_input,
// first_reward, collect, hurt, milestone, win, lose). Telemetry v2
// events go to POST /v1/telemetry/events in the advisor's shapes
// (session_start, perf_window, milestone, run_end); error reports keep
// using the original runtime-error endpoint.
const dqSessionId = Math.floor(Math.random() * 0xffffffff).toString(16);
let dqTelemetrySeq = 0;
let dqNativeMetricSeq = 0;
let dqEvents = [];
function recordEvent(type, detail) {
  dqEvents.push({ t: Math.round(performance.now()), type, detail: detail || '' });
  if (dqEvents.length > 600) dqEvents.splice(0, dqEvents.length - 600);
}
function deviceClass() {
  const ua = navigator.userAgent || '';
  const fam = /iPhone/.test(ua) ? 'iphone' : /iPad/.test(ua) ? 'ipad'
    : /Android/.test(ua) ? 'android' : 'desktop';
  return `${fam}|${window.innerWidth}x${window.innerHeight}|dpr${window.devicePixelRatio || 1}`;
}
function sendTelemetryEvent(kind, fields) {
  try {
    const payload = JSON.stringify({
      schemaVersion: 2, eventId: dqSessionId + ':' + (++dqTelemetrySeq),
      gameId, runtimeVersion: RUNTIME_VERSION, sessionId: dqSessionId,
      kind, quality: qualityMode, deviceClass: deviceClass(),
      at: new Date().toISOString(), ...fields,
    });
    navigator.sendBeacon('/v1/telemetry/events', new Blob([payload], { type: 'application/json' }));
  } catch { /* telemetry must never break the game */ }
}
function sendNativeMetric(metric, fields = {}) {
  try {
    window.webkit?.messageHandlers?.hephaestusRunResult?.postMessage({
      schema: 1,
      type: 'player_metric',
      sessionId: dqSessionId,
      gameId,
      metric,
      sequence: ++dqNativeMetricSeq,
      ...fields,
    });
  } catch { /* native performance reporting is optional outside iOS */ }
}

// --- Player diagnostic social events (Hephaestus #61) -------------------
// The bridgeless iOS shell cannot observe time to interactive, first
// input, or frame times, so the runtime reports them itself as social
// events — the same endpoint iOS uses for preview_dwell, with the same
// schema v1 fields and event-ID idempotency. player_ready fires at boot
// (so sessions that never reach a run end still report time to
// interactive); player_session_summary fires at run end with the full
// frame summary. Sent with apiFetch, not a beacon: when the tab holds a
// session token the event is attributed to that account, otherwise it
// arrives anonymous, which the endpoint accepts. These diagnostic kinds
// are excluded from feed-ranking signals server-side. Delivery is
// fire-and-forget: telemetry must never break the game.
function sendSocialDiagnostic(kind, suffix, fields) {
  try {
    const payload = JSON.stringify({
      schema_version: 1,
      event_id: `${dqSessionId}:run${restartCount}:${suffix}`,
      game_id: gameId, kind, ...fields,
    });
    // Drain the response body: a fire-and-forget fetch whose Response is
    // never read is cancelled by the browser when collected, surfacing as
    // a failed request (and breaking the playability kit's load gate) even
    // though the event already landed server-side. Draining also frees the
    // keep-alive connection for reuse.
    apiFetch('/v1/telemetry/social-event', {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: payload,
    }).then((res) => res.arrayBuffer().catch(() => {}), () => {}).catch(() => {});
  } catch { /* telemetry must never break the game */ }
}

// Bucket-estimated p95 frame time for a run: the smallest bucket upper
// edge whose cumulative share reaches 95%. Edges are the perf_window
// bucket edges rounded to whole milliseconds. The open-ended >50ms
// bucket reports the run's longest frame instead of an unbounded edge.
function frameP95Ms(buckets, total, longestMs) {
  if (!total) return 0;
  const edges = [8, 17, 25, 33, 50];
  let cumulative = 0;
  for (let i = 0; i < 5; i++) {
    cumulative += buckets[i] || 0;
    if (cumulative * 20 >= total * 19) return edges[i];
  }
  return Math.round(longestMs);
}

// --- WebAudio SFX pool (Phase A) ---------------------------------------
// Replaces per-play `new Audio(url)`: buffers are fetched + decoded
// once and replayed from memory, and the context unlocks on the first
// pointer event — which also ends the "audio play blocked" warning
// every game used to log at boot (music started by a rule at game_start
// now waits for the unlock and plays then if it was requested).
const sfx = {
  ctx: null, buffers: new Map(), pendingMusic: [],
  ensureCtx() {
    if (!this.ctx) {
      const AC = window.AudioContext || window.webkitAudioContext;
      if (AC) { try { this.ctx = new AC(); } catch { this.ctx = null; } }
    }
    if (this.ctx && this.ctx.state === 'suspended') this.ctx.resume().catch(() => {});
    return this.ctx;
  },
  unlock() {
    if (!this.ensureCtx()) return;
    for (const job of this.pendingMusic.splice(0)) job();
  },
  play(url, assetId) {
    const ctx = this.ensureCtx();
    if (!ctx) { new Audio(url).play().catch(() => {}); return; }
    const start = (buffer) => {
      const src = ctx.createBufferSource();
      src.buffer = buffer;
      src.connect(ctx.destination);
      src.start();
    };
    const job = () => {
      if (this.buffers.has(assetId)) { start(this.buffers.get(assetId)); return; }
      apiFetch(url).then((r) => r.arrayBuffer()).then((ab) => ctx.decodeAudioData(ab))
        .then((buffer) => { this.buffers.set(assetId, buffer); start(buffer); })
        .catch(() => { new Audio(url).play().catch(() => {}); });
    };
    // Autoplay policy: while the context is suspended (no user gesture
    // yet) a started buffer would never be heard. Queue the play for
    // the unlock instead of dropping it on the floor.
    if (ctx.state === 'suspended') { this.pendingMusic.push(job); return; }
    job();
  },
};
addEventListener('pointerdown', () => sfx.unlock(), { once: false });

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
  static instances = [];
  constructor(container, cfg, textures) {
    Emitter.instances.push(this);
    this.cfg = cfg || {};
    this.container = container;
    const tex = (cfg?.textureAssetId && textures.get(cfg.textureAssetId)) || getDotTexture();
    const qualityScale = qualityMode === 'low' ? 0.4 : 1;
    this.max = clamp(Math.trunc((cfg?.maxParticles ?? 240) * qualityScale), 8, 2000);
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
  aliveCount() {
    let n = 0;
    for (const p of this.pool) if (p.alive) n++;
    return n;
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
const probeMode = params.get('probe') === '1';
const deterministicCapture = probeMode && params.get('capture') === '1';
const playPresentation = Boolean(gameId && params.get('presentation') === 'play');
if (playPresentation) {
  document.body.classList.add('dq-play');
  document.body.dataset.presentation = 'play';
  // The native host owns identity, feed navigation, and diagnostics. Remove
  // browser-only chrome from the DOM rather than merely making it inert.
  document.getElementById('account')?.remove();
  document.querySelector('section[aria-labelledby="feed-title"]')?.remove();
  document.querySelector('details')?.remove();
}
// Quality mode (performance control, driven by the host shell — the iOS
// app appends ?quality=low on thermally constrained / low-power devices,
// or when the family picks it): 'low' caps the render resolution at 1x,
// disables antialiasing, and shrinks particle pools. Anything else is
// 'high' (the default, capped at 2x so 3x phone panels don't triple the
// fill cost for a 390pt-wide stage).
const qualityMode = params.get('quality') === 'low' ? 'low' : 'high';

// The web shell accepts a short-lived bearer token from a trusted identity
// provider; it does not mint credentials. Keep it in tab-scoped session storage
// and never attach it to telemetry beacons or cross-origin requests.
const AUTH_TOKEN_KEY = 'dq.session.bearer';
function readSessionToken() {
  try { return sessionStorage.getItem(AUTH_TOKEN_KEY) || ''; } catch { return ''; }
}
async function apiFetch(input, init = {}) {
  const url = new URL(input, location.href);
  const headers = new Headers(init.headers || {});
  const token = readSessionToken();
  if (token && url.origin === location.origin) headers.set('Authorization', 'Bearer ' + token);
  return window.fetch(input, { ...init, headers });
}
function setAccountStatus(message) {
  const el = $('account-status');
  if (el) el.textContent = message;
}
function initializeAccountControls() {
  const tokenInput = $('bearer-token');
  if (!tokenInput) return;
  $('save-token')?.addEventListener('click', async () => {
    const token = tokenInput.value.trim();
    if (!token) { setAccountStatus('Paste a bearer token from your trusted sign-in provider.'); return; }
    try { sessionStorage.setItem(AUTH_TOKEN_KEY, token); }
    catch { setAccountStatus('This browser does not allow session storage; sign-in cannot be retained in this tab.'); return; }
    tokenInput.value = '';
    setAccountStatus('Checking session…');
    try {
      const res = await apiFetch('/v1/users/me/preferences');
      if (res.status === 401) {
        sessionStorage.removeItem(AUTH_TOKEN_KEY);
        setAccountStatus('The session token was rejected or expired. Sign in again with your provider.');
        return;
      }
      if (!res.ok) {
        setAccountStatus(res.status === 503
          ? 'Backend authentication is not configured. Set DQ_AUTH_HMAC_SECRET and configure a trusted token issuer.'
          : 'Session check failed (HTTP ' + res.status + ').');
        return;
      }
      setAccountStatus('Signed in for this tab. Private social actions will use the verified account.');
      await listGames();
    } catch (err) { setAccountStatus('Unable to check session: ' + err.message); }
  });
  $('clear-token')?.addEventListener('click', () => {
    try { sessionStorage.removeItem(AUTH_TOKEN_KEY); } catch { /* storage unavailable */ }
    tokenInput.value = '';
    setAccountStatus('Signed out of this tab.');
    listGames().catch(() => {});
  });
  $('feed-scope')?.addEventListener('change', () => listGames().catch((err) => setAccountStatus('Feed failed: ' + err.message)));
  $('refresh-feed')?.addEventListener('click', () => listGames().catch((err) => setAccountStatus('Feed failed: ' + err.message)));
}

async function listGames() {
  const status = $('status');
  const list = $('list');
  const count = $('feed-count');
  const scope = $('feed-scope')?.value || 'for_you';
  if (status) status.textContent = 'Choose a game to play.';
  if (list) {
    list.setAttribute('aria-busy', 'true');
    list.replaceChildren();
    for (let i = 0; i < 3; i++) {
      const skeleton = document.createElement('div');
      skeleton.className = 'feed-skeleton';
      skeleton.setAttribute('aria-hidden', 'true');
      list.append(skeleton);
    }
  }
  if (count) count.textContent = 'Loading…';

  let res;
  try {
    res = await apiFetch('/v1/feed?scope=' + encodeURIComponent(scope));
  } catch (err) {
    if (list) {
      list.replaceChildren();
      const state = document.createElement('div');
      state.className = 'feed-state';
      const title = document.createElement('strong');
      title.textContent = 'You appear to be offline';
      const message = document.createElement('span');
      message.textContent = 'Check your connection and try refreshing the feed.';
      state.append(title, message);
      list.append(state);
      list.setAttribute('aria-busy', 'false');
    }
    if (count) count.textContent = '';
    throw err;
  }
  if (!res.ok) {
    if (res.status === 401) setAccountStatus('This feed requires a valid session token. Sign in with your trusted provider.');
    else if (res.status === 503) setAccountStatus('Backend authentication is not configured. Public browsing may be restricted by deployment settings.');
    if (list) {
      list.replaceChildren();
      const state = document.createElement('div');
      state.className = 'feed-state';
      const title = document.createElement('strong');
      title.textContent = res.status === 401 ? 'Sign in to see this feed' : 'Games could not be loaded';
      const message = document.createElement('span');
      message.textContent = res.status === 401
        ? 'Use your trusted sign-in provider, then refresh the feed.'
        : 'The service returned HTTP ' + res.status + '. Try again in a moment.';
      state.append(title, message);
      list.append(state);
      list.setAttribute('aria-busy', 'false');
    }
    if (count) count.textContent = '';
    throw new Error('Feed request failed (HTTP ' + res.status + ')');
  }

  const data = await res.json();
  const games = Array.isArray(data.games) ? data.games : [];
  if (list) list.replaceChildren();
  if (count) count.textContent = games.length + (games.length === 1 ? ' game' : ' games');
  if (!games.length && list) {
    const state = document.createElement('div');
    state.className = 'feed-state';
    const title = document.createElement('strong');
    title.textContent = scope === 'following' ? 'Your feed is just getting started' : 'No games here yet';
    const message = document.createElement('span');
    message.textContent = scope === 'following'
      ? 'Follow a creator from a game card and their public games will show up here.'
      : 'Try another feed, or come back soon for new games to play.';
    state.append(title, message);
    list.append(state);
  }

  for (const g of games) {
    const card = document.createElement('article');
    card.className = 'game-card';
    const topline = document.createElement('div');
    topline.className = 'card-topline';
    const tag = document.createElement('span');
    tag.className = 'playable-tag';
    tag.textContent = 'PLAYABLE';
    topline.append(tag);

    const title = document.createElement('a');
    title.className = 'game-title';
    title.href = '/player?game=' + encodeURIComponent(g.game_id);
    title.textContent = g.title || 'Untitled game';

    const sub = document.createElement('div');
    sub.className = 'sub';
    const creatorID = g.owner_id || g.creator_id || g.creatorId;
    sub.textContent = creatorID ? 'Created by ' + creatorID : 'Community game';

    const actions = document.createElement('div');
    actions.className = 'card-actions';
    const play = document.createElement('a');
    play.className = 'play-link';
    play.href = title.href;
    play.textContent = '▶ Play now';
    actions.append(play);

    if (creatorID) {
      const follow = document.createElement('button');
      follow.type = 'button';
      follow.className = 'follow-button';
      follow.textContent = scope === 'following' ? 'Following feed' : 'Follow creator';
      follow.disabled = scope === 'following';
      follow.addEventListener('click', async () => {
        follow.disabled = true;
        try {
          const response = await apiFetch('/v1/users/' + encodeURIComponent(creatorID) + '/follow', { method: 'POST' });
          if (!response.ok) throw new Error(response.status === 401 ? 'Sign in to follow creators.' : 'Follow failed (HTTP ' + response.status + ')');
          follow.textContent = 'Following';
          setAccountStatus('Creator followed. Refresh the Following feed to see followed creators.');
        } catch (err) {
          follow.disabled = false;
          setAccountStatus(err.message);
        }
      });
      actions.append(follow);
    }
    card.append(topline, title, sub, actions);
    list?.append(card);
  }
  if (list) list.setAttribute('aria-busy', 'false');
  log('PKG', `feed listed ${games.length} games in ${scope}`);
}

// The live session, for in-place restart (Phase A): restarting must
// not reload the page (a reload refetches everything and flashes) —
// it tears the PIXI app down and boots the same game again.
// Web Storage can throw (private browsing, disabled cookies, sandboxed
// frames). Every access goes through these wrappers — a storage failure
// must never break a game (architecture review 2026-10-23 found the
// scene/best-score accesses unguarded).
function storeGet(store, k) { try { return store.getItem(k); } catch { return null; } }
function storeSet(store, k, v) { try { store.setItem(k, v); } catch { /* unavailable */ } }
function storeRemove(store, k) { try { store.removeItem(k); } catch { /* unavailable */ } }

let activeSession = null; // { app, abort: AbortController }
let restartCount = 0;
let skipStartCardOnce = false; // Play again skips the how-to card (design: retry < 1s)
async function restartGame() {
  if (!gameId) return;
  restartCount++;
  skipStartCardOnce = true;
  recordEvent('restart', 'attempt ' + restartCount);
  await run(gameId).catch((err) => {
    $('status').textContent = 'Failed to restart: ' + err.message;
    recordError('restart', err);
  });
}
window.__dqRestart = restartGame;

async function run(id) {
  const started = performance.now();
  // Tear down the previous session (in-place restart path).
  if (activeSession) {
    try { activeSession.abort.abort(); } catch { /* ignore */ }
    try { activeSession.app.destroy(true, { children: true, texture: false }); } catch { /* ignore */ }
    activeSession = null;
    $('stage').innerHTML = '';
    $('end').style.display = 'none';
  }
  const abort = new AbortController();
  dqEvents = [];
  Emitter.instances = [];
  $('shell-start')?.classList.remove('on');
  $('shell-result')?.classList.remove('on');
  // Seed: explicit ?seed= wins (tests, replays); else a fresh random
  // seed, reported in telemetry + the probe verdict.
  const seedParam = params.get('seed');
  let sessionSeed = seedParam !== null && /^\d+$/.test(seedParam)
    ? Number(seedParam) >>> 0
    : Math.floor(Math.random() * 0xffffffff) >>> 0;
  // Replay mode (Phase C): ?replay=<runId> re-runs a submitted run
  // from its seed + input trace so the server can verify the score.
  // The trace's seed overrides everything — determinism is the point.
  const replayRunId = params.get('replay');
  let replayTrace = null;
  if (replayRunId) {
    try {
      const rr = await apiFetch(`/v1/runs/${encodeURIComponent(replayRunId)}`);
      if (rr.ok) {
        replayTrace = await rr.json();
        if (Number.isFinite(replayTrace.seed)) sessionSeed = replayTrace.seed >>> 0;
      }
    } catch { /* verdict will report the missing trace */ }
  }
  srand(sessionSeed);

  // --- Play-session bookkeeping (Phases A+B) ---------------------------
  // One object for everything the shell, the metrics, and telemetry v2
  // need: visible-play clock (fps that excludes hidden time), the
  // tick-stamped input log, dead-input accounting, perf-window frame
  // samples, and shell state. Declared before any handler can fire.
  const play = {
    seed: sessionSeed,
    score: 0,                   // mirrors state.score for the probe verdict
    simStep: 0,                 // fixed-step simulation clock (Phase C determinism)
    visibleMs: 0,               // ticker time while visible + running
    startedAt: 0,               // performance.now() at boot completion
    firstInputAt: 0, firstRewardAt: 0,
    inputLog: [],               // {tick, type, detail}
    pendingInputs: [],          // {at: visibleMs, x, y} awaiting the 200ms activity check
    deadInputs: 0,
    lastActivityMs: 0,          // last visibleMs with a rule fire / collect / player move
    lastPlayerX: -1, lastPlayerY: -1,
    frameSamples: [],           // deltaMS within the current perf window
    longestFrameMs: 0,
    sessionBuckets: [0, 0, 0, 0, 0, 0], // all frames this run, same edges as perf_window
    sessionFrameSumMs: 0,
    sessionFrames: 0,
    sessionLongestMs: 0,
    bootMs: 0,                  // set at boot completion; time to interactive
    liveCounts: () => ({ entities: 0, particles: 0 }),
    shell: null,                // experience block when the game is IR 1.2
    shellStarted: false,
    milestonesHit: new Set(),
  };
  function noteInput(type, detail) {
    play.inputLog.push({ tick: play.simStep, type, detail: detail || '' });
    if (play.inputLog.length > 2000) play.inputLog.splice(0, play.inputLog.length - 2000);
    play.pendingInputs.push({ at: play.visibleMs, x: play.lastPlayerX, y: play.lastPlayerY });
    if (!play.firstInputAt) {
      play.firstInputAt = Math.round(play.visibleMs);
      recordEvent('first_input', type);
      sendTelemetryEvent('milestone', { milestone: 'first_input', ms: play.firstInputAt });
      sendNativeMetric('first_input', { ms: play.firstInputAt });
    }
  }
  function noteActivity() { play.lastActivityMs = play.visibleMs; }
  function flushPerfWindow() {
    if (!play.frameSamples.length) return;
    const buckets = [0, 0, 0, 0, 0, 0]; // <=8, <=16.7, <=25, <=33, <=50, >50 ms
    for (const ms of play.frameSamples) {
      if (ms <= 8) buckets[0]++; else if (ms <= 16.7) buckets[1]++;
      else if (ms <= 25) buckets[2]++; else if (ms <= 33) buckets[3]++;
      else if (ms <= 50) buckets[4]++; else buckets[5]++;
    }
    const sorted = [...play.frameSamples].sort((a, b) => a - b);
    const frameTimeP95Ms = Math.round(sorted[Math.min(sorted.length - 1, Math.floor(sorted.length * 0.95))]);
    const frameTimeAvgMs = Math.round(play.frameSamples.reduce((sum, ms) => sum + ms, 0) / play.frameSamples.length);
    // Session totals for the run-end diagnostic summary: the same buckets
    // and samples the perf_window above reports, accumulated so the summary
    // covers the whole run, not just the last window.
    let windowSumMs = 0;
    for (const ms of play.frameSamples) windowSumMs += ms;
    for (let i = 0; i < 6; i++) play.sessionBuckets[i] += buckets[i];
    play.sessionFrameSumMs += windowSumMs;
    play.sessionFrames += play.frameSamples.length;
    if (play.longestFrameMs > play.sessionLongestMs) play.sessionLongestMs = play.longestFrameMs;
    const counts = play.liveCounts();
    sendTelemetryEvent('perf_window', {
      frames: play.frameSamples.length, buckets,
      longestFrameMs: Math.round(play.longestFrameMs),
      entities: counts.entities, particles: counts.particles,
    });
    sendNativeMetric('perf_window', {
      frameTimeP95Ms, frameTimeAvgMs, frameCount: play.frameSamples.length,
    });
    play.frameSamples = [];
    play.longestFrameMs = 0;
  }
  const res = await apiFetch(`/v1/games/${encodeURIComponent(id)}/package`);
  if (!res.ok) {
    const body = await res.text();
    throw new Error(`package HTTP ${res.status}: ${body.slice(0, 120)}`);
  }
  const pkg = await res.json();
  log('PKG', `package ${id}: HTTP 200 (${Math.round(performance.now() - started)}ms), format v${pkg.packageFormatVersion}, min runtime ${pkg.minRuntimeVersion}, IR v${pkg.ir?.irVersion}, ${pkg.assets?.length ?? 0} assets`);
  if (pkg.packageFormatVersion !== '2.0') throw new Error('unsupported packageFormatVersion ' + pkg.packageFormatVersion);
  const compatibility = compareRuntimeVersions(RUNTIME_VERSION, pkg.minRuntimeVersion);
  if (compatibility === null) throw new Error('invalid minRuntimeVersion ' + pkg.minRuntimeVersion);
  if (compatibility < 0) throw new Error('game requires runtime ' + pkg.minRuntimeVersion + ', current runtime is ' + RUNTIME_VERSION);
  const ir = pkg.ir;
  if (!['1.0', '1.1', '1.2'].includes(ir.irVersion)) throw new Error('unsupported irVersion ' + ir.irVersion);
  play.irHash = pkg.irHash || ''; // leaderboard board key (Phase C)

  const sceneKey = 'dq.scene.' + id;
  const wantedScene = storeGet(sessionStorage, sceneKey) || ir.game.entryScene;
  storeRemove(sessionStorage, sceneKey);
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
    width: W, height: H,
    antialias: qualityMode !== 'low',
    resolution: qualityMode === 'low' ? 1 : Math.min(window.devicePixelRatio || 1, 2),
    autoDensity: true,
    background: scene.backgroundColor ? parseColor(scene.backgroundColor, 0x16204a) : 0x16204a,
  });
  $('stage').appendChild(app.canvas);
  // Letterbox continuity (Phase E): when the viewport aspect differs
  // from the world's, the page around the canvas wears the scene's
  // own background color instead of black — no "black bar" framing
  // the playfield.
  if (scene.backgroundColor) {
    document.body.style.background = scene.backgroundColor;
    const stageEl = $('stage');
    if (stageEl) stageEl.style.background = scene.backgroundColor;
  }
  activeSession = { app, abort };
  if (deterministicCapture) app.ticker.stop();
  log('RT', 'pixi stage ready');

  // Pause when the page is hidden (Phase A): the ticker drives the
  // whole simulation, so stopping it freezes the world; the frame
  // delta is clamped in tickFrame, so resuming doesn't lurch.
  document.addEventListener('visibilitychange', () => {
    if (document.hidden) {
      try { app.ticker.stop(); } catch { /* ignore */ }
      recordEvent('pause', 'hidden');
      flushPerfWindow();
    } else {
      if (play.shellStarted && !(deterministicCapture && capturePaused)) { try { app.ticker.start(); } catch { /* ignore */ } }
      recordEvent('resume', '');
    }
  }, { signal: abort.signal });
  // WebGL context loss: prevent the default (a frozen canvas forever)
  // so the browser restores the context and Pixi re-creates its GL
  // state; report both transitions as telemetry-grade events.
  app.canvas.addEventListener('webglcontextlost', (e) => {
    e.preventDefault();
    log('RT', 'WEBGL CONTEXT LOST');
    recordError('context', new Error('WebGL context lost'));
    recordEvent('context_lost', '');
  });
  app.canvas.addEventListener('webglcontextrestored', () => {
    log('RT', 'WEBGL CONTEXT RESTORED');
    recordEvent('context_restored', '');
  });

  // Pixi's own render listener runs outside our ticker's try/catch; a
  // throw there (e.g. a missing render pipe) repeats every frame and
  // blanks the stage. Trap it: log + report once, then stop the ticker —
  // a frozen frame with a named error beats a silent error storm.
  let renderErrLogged = false;
  const origRender = app.renderer.render.bind(app.renderer);
  app.renderer.render = (...args) => {
    try {
      return origRender(...args);
    } catch (e) {
      if (!renderErrLogged) {
        renderErrLogged = true;
        log('RT', 'RENDER ERROR ' + (e.stack || e.message));
        recordError('render', e);
        try { app.ticker.stop(); } catch { /* ignore */ }
      }
      return undefined;
    }
  };
  app.stage.sortableChildren = true;

  const bgLayer = new PIXI.Container();      // parallax layers
  const worldLayer = new PIXI.Container();   // entities (camera-moved)
  worldLayer.sortableChildren = true;
  const fxLayer = new PIXI.Container();      // particles (screen space)
  app.stage.addChild(bgLayer, worldLayer, fxLayer);

  const assetURL = new Map((pkg.assets || []).map((a) => [a.id, a.url || a.uri]));
  const textures = new Map();
  // Package asset URLs are extension-less (/v1/games/<id>/assets/<assetId>), and
  // PIXI.Assets.load() resolves null for a URL it cannot match to a loader by
  // extension — the null then kills Sprite construction ("reading 'texture'").
  // Load the image element directly and build the texture from it instead; the
  // browser sniffs the real format from the bytes.
  async function loadTexture(url) {
    if (!url) throw new Error('image asset has no URL');
    const img = new Image();
    await new Promise((resolve, reject) => {
      let settled = false;
      const finish = (callback, value) => {
        if (settled) return;
        settled = true;
        clearTimeout(timeout);
        img.onload = null;
        img.onerror = null;
        callback(value);
      };
      const timeout = setTimeout(
        () => finish(reject, new Error(`image load timed out after 15s: ${url}`)),
        15000
      );
      img.onload = () => finish(resolve);
      img.onerror = () => finish(reject, new Error(`image load failed: ${url}`));
      img.src = url;
    });
    return PIXI.Texture.from(img);
  }

  // Independent images load concurrently with a bounded worker pool. This
  // avoids serial cold-start latency without flooding mobile WebKit with
  // one simultaneous request per package asset.
  const imageAssets = (pkg.assets || []).filter((asset) => asset.kind === 'image');
  let nextImageIndex = 0;
  const imageWorkerCount = Math.min(6, imageAssets.length);
  await Promise.all(Array.from({ length: imageWorkerCount }, async () => {
    while (true) {
      const index = nextImageIndex++;
      if (index >= imageAssets.length) return;
      const asset = imageAssets[index];
      try {
        textures.set(asset.id, await loadTexture(assetURL.get(asset.id)));
      } catch (e) {
        log('PKG', `WARN asset '${asset.id}' failed to load: ${e.message}`);
      }
    }
  }));
  log('PKG', `textures loaded: ${textures.size}/${imageAssets.length} images`);
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
      let frameNames = r.animation.frames;
      // Ping-pong is a bounced sequence (1..n..2) — PIXI has no such
      // mode, so expand it here (the old code set loop=true, a no-op).
      if (r.animation.pingPong && frameNames.length > 2) {
        frameNames = frameNames.concat(frameNames.slice(1, -1).reverse());
      }
      view = new PIXI.AnimatedSprite(frameNames.map((f) => textures.get(f)));
      view.width = w; view.height = h;
      view.animationSpeed = (r.animation.fps ?? 8) / 60;
      view.loop = r.animation.loop ?? true;
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
  const byKind = new Map();         // def.kind -> [ent] (indexed lookup; was a linear scan per rule action)
  const byCollider = new Map();     // collider handle -> ent
  const templates = new Map();
  for (const t of scene.templates || []) templates.set(t.id, t);
  let spawnCounter = 0;
  let respawnIndex = 0;

  function register(ent) {
    entities.set(ent.runtimeId, ent);
    if (!byDefId.has(ent.defId)) byDefId.set(ent.defId, []);
    byDefId.get(ent.defId).push(ent);
    const kind = ent.def?.kind;
    if (kind) {
      if (!byKind.has(kind)) byKind.set(kind, []);
      byKind.get(kind).push(ent);
    }
    if (ent.collider) byCollider.set(ent.collider.handle, ent);
  }
  function unregister(ent) {
    entities.delete(ent.runtimeId);
    const arr = byDefId.get(ent.defId) || [];
    const i = arr.indexOf(ent);
    if (i >= 0) arr.splice(i, 1);
    const karr = byKind.get(ent.def?.kind) || [];
    const ki = karr.indexOf(ent);
    if (ki >= 0) karr.splice(ki, 1);
    if (ent.collider) byCollider.delete(ent.collider.handle);
  }
  function findEnt(ref) {
    if (!ref) return undefined;
    return entities.get(ref) || (byDefId.get(ref) || [])[0] || (byKind.get(ref) || [])[0];
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
      destroyed: false, phase: rng() * Math.PI * 2, stillTime: 0,
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

  let prevHealth = state.health;
  function emit(event, source, target, value, inputAction) {
    if (state.depth > 8) { warnOnce('depth', 'RULE', 'event cascade depth exceeded — dropped'); return; }
    // Event stream for the probe / playtest worker / shell.
    if (event === 'collected') { recordEvent('collect', target?.defId || ''); noteActivity(); }
    state.depth++;
    try {
      for (const rule of ir.rules) {
        if (!ruleMatches(rule, event, source, target, value, inputAction)) continue;
        if (!conditionsMet(rule)) continue;
        log('RULE', `rule '${rule.id}' fired by ${event}`);
        noteActivity();
        execute(rule.actions || [], source, target);
        if (state.ended) break;
      }
    } finally {
      state.depth--;
    }
    if (event === 'health_changed') {
      if (state.health < prevHealth) recordEvent('hurt', `health ${prevHealth} -> ${state.health}`);
      prevHealth = state.health;
    }
  }

  function updateHUD() {
    // Shell games (IR 1.2) get the standardized HUD: objective progress
    // on the score side, hearts only when the failure model uses them.
    const exp = play.shell;
    if (exp) {
      const target = exp.objective?.target;
      const scoreTxt = exp.objective?.kind === 'collect' && target
        ? `★ ${state.score}/${target}` : `★ ${state.score}`;
      const mode = exp.failure?.mode || 'health';
      const hearts = (mode === 'health' || mode === 'sudden-death') ? `  ♥ ${state.health}` : '';
      $('score').textContent = scoreTxt + hearts;
      return;
    }
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

  // Pacing ramp (IR 1.2): the speed multiplier at the current point
  // of the session, interpolated between declared steps over
  // normalized session time. Applied when entities (re)spawn, so the
  // game genuinely gets faster as the session progresses — the ramp
  // is data the runtime executes, not decoration.
  function rampSpeedMul() {
    const ramp = play.shell?.pacing?.ramp;
    if (!Array.isArray(ramp) || !ramp.length) return 1;
    const target = play.shell?.pacing?.targetSessionSec || ir.game.sessionTargetSeconds || 60;
    const t = clamp(state.elapsed / target, 0, 1);
    const mul = (s) => (s.speedMul && s.speedMul > 0 ? s.speedMul : 1);
    if (t <= ramp[0].at) return mul(ramp[0]);
    for (let i = 1; i < ramp.length; i++) {
      if (t <= ramp[i].at) {
        const a = ramp[i - 1], b = ramp[i];
        const f = (t - a.at) / Math.max(1e-6, b.at - a.at);
        return mul(a) + (mul(b) - mul(a)) * f;
      }
    }
    return mul(ramp[ramp.length - 1]);
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
      ent.body.setLinvel({ x: 0, y: toM((ent.def.physics?.initialVelocity?.[1] ?? 60) * rampSpeedMul()) }, true);
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
        case 'add_score': {
          state.score += Math.trunc(action.value ?? 1);
          play.score = state.score; // mirrored for the probe verdict (Phase C)
          recordEvent('score_change', 'score ' + state.score);
          noteActivity();
          if (!play.firstRewardAt && state.score > 0) {
            play.firstRewardAt = Math.round(play.visibleMs);
            recordEvent('first_reward', 'score ' + state.score);
            sendTelemetryEvent('milestone', { milestone: 'first_reward', ms: play.firstRewardAt });
          }
          // Experience milestones (IR 1.2): fractions of the objective
          // target, recorded as events the shell + worker can see.
          const exp = play.shell;
          if (exp?.objective?.target > 0 && Array.isArray(exp.milestones)) {
            for (const frac of exp.milestones) {
              const key = String(frac);
              if (!play.milestonesHit.has(key) && state.score >= Math.max(1, Math.round(exp.objective.target * frac))) {
                play.milestonesHit.add(key);
                recordEvent('milestone', key);
              }
            }
          }
          updateHUD();
          emitScoreThresholds();
          break;
        }
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
        case 'change_health': {
          const delta = Math.trunc(action.value ?? 0);
          // Invulnerability window (IR 1.2 failure model): damage
          // inside the window is ignored, so chained contacts in
          // consecutive frames can never drain health instantly.
          // Heals (positive deltas) always apply.
          const invulnMs = play.shell?.failure?.invulnerableMs || 0;
          if (delta < 0 && invulnMs > 0 && state.elapsed - (state.lastDamageAt ?? -1e9) < invulnMs / 1000) {
            log('RULE', `damage ignored (invulnerable, ${Math.round(invulnMs - (state.elapsed - state.lastDamageAt) * 1000)}ms left)`);
            break;
          }
          if (delta < 0) state.lastDamageAt = state.elapsed;
          state.health = Math.max(0, state.health + delta);
          updateHUD();
          emit('health_changed', sourceEnt, targetEnt);
          if (state.health === 0) endGame(false);
          break;
        }
        case 'end_game':
          endGame(Boolean(action.value));
          break;
        case 'play_sound': {
          const url = audio.get(action.assetId);
          if (!url) { warnOnce('sound-' + action.assetId, 'RULE', `play_sound: audio asset '${action.assetId}' missing — skipped`); break; }
          sfx.play(url, action.assetId);
          break;
        }
        case 'change_scene': {
          if (!action.sceneId || !(ir.scenes || []).some((s) => s.id === action.sceneId)) {
            warnOnce('scene-' + action.sceneId, 'RULE', `change_scene: scene '${action.sceneId}' missing — skipped`);
            break;
          }
          storeSet(sessionStorage, sceneKey, action.sceneId);
          log('RULE', `changing scene to '${action.sceneId}'`);
          restartGame();
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

  // Shell result card (IR 1.2): win shows stars from the result
  // block's time thresholds plus the best score; lose shows REAL
  // progress ("9 of 12 hearts") — never a bare "Game over". Play
  // again restarts in place (no reload, no refetch).
  function showShellResult(won) {
    const exp = play.shell;
    const res = won ? (exp.result?.win || {}) : (exp.result?.lose || {});
    $('shell-result-title').textContent = res.title || (won ? 'You did it!' : 'So close!');
    let starsTxt = '';
    if (won) {
      const th = exp.result?.win?.stars?.thresholdsSec;
      if (Array.isArray(th) && th.length >= 3) {
        const secs = play.visibleMs / 1000;
        const stars = secs <= th[2] ? 3 : secs <= th[1] ? 2 : 1;
        starsTxt = '★'.repeat(stars) + '☆'.repeat(3 - stars);
      } else {
        starsTxt = '★★★';
      }
    }
    $('shell-result-stars').textContent = starsTxt;
    const bestKey = 'dq.best.' + gameId;
    const prevBest = Number(storeGet(localStorage, bestKey) || 0);
    const isBest = state.score > prevBest;
    if (isBest) storeSet(localStorage, bestKey, String(state.score));
    const target = exp.objective?.target;
    const progress = exp.objective?.kind === 'collect' && target
      ? `${state.score} of ${target} — ` : '';
    $('shell-result-sub').textContent =
      `${progress}Score: ${state.score}${isBest && state.score > 0 ? ' · New best!' : prevBest ? ` · Best: ${prevBest}` : ''}`;
    $('shell-result').classList.add('on');
    $('shell-again-btn').addEventListener('click', () => {
      $('shell-result').classList.remove('on');
      restartGame();
    }, { once: true });
  }

  function endGame(won) {
    if (state.ended) return;
    state.ended = true;
    state.won = won;
    log('RT', `game ended: ${won ? 'WIN' : 'LOSE'}, score ${state.score}`);
    recordEvent(won ? 'win' : 'lose', 'score ' + state.score);
    flushPerfWindow();
    sendTelemetryEvent('run_end', {
      won, score: state.score,
      durationMs: Math.round(play.visibleMs),
      inputs: play.inputLog.length, deadInputs: play.deadInputs,
      retries: restartCount, seed: play.seed,
    });
    // Run-end diagnostic summary (Hephaestus #61): the timing fields the
    // iOS shell cannot observe, from this run's own measurements. Values
    // are clamped to the social-event caps so one pathological frame or
    // an over-long session cannot drop the whole summary; a zero first
    // input means no input was recorded before the run ended. The event
    // ID is stable per run, so a retried delivery is deduplicated
    // server-side instead of double-counted.
    sendSocialDiagnostic('player_session_summary', 'summary', {
      time_to_interactive_ms: Math.min(play.bootMs, 3600000),
      time_to_first_input_ms: Math.min(Math.round(play.firstInputAt), 3600000),
      frame_time_p95_ms: Math.min(frameP95Ms(play.sessionBuckets, play.sessionFrames, play.sessionLongestMs), 60000),
      frame_time_avg_ms: Math.min(play.sessionFrames ? Math.round(play.sessionFrameSumMs / play.sessionFrames) : 0, 60000),
      frame_count: play.sessionFrames,
    });
    if (play.shell) {
      showShellResult(won);
    } else {
      $('end-title').textContent = won ? 'You did it!' : 'Game over';
      $('end-sub').textContent = 'Score: ' + state.score;
      $('end').style.display = 'block';
    }
    // Phase C: leaderboard run submission (normal play) or replay
    // verdict (verification play). Submission carries the full input
    // trace so the score can be re-run and checked server-side.
    if (replayTrace) {
      const match = state.score === replayTrace.score && won === !!replayTrace.won;
      window.__dqReplay = { runId: replayRunId, match, expected: replayTrace.score, actual: state.score };
      try {
        apiFetch(`/v1/runs/${encodeURIComponent(replayRunId)}/verification`, {
          method: 'POST', headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ match, actualScore: state.score }),
        }).catch(() => {});
      } catch { /* the verdict on the window is the worker's evidence */ }
    } else if (play.shellStarted) {
      submitRun(won);
    }
  }

  // submitRun posts this run to the leaderboard and, when the server
  // answers with a rank, adds it to the result card. Fire-and-forget:
  // a failed submission never breaks the end of a game.
  let playerNameCache = null;
  function playerName() {
    if (playerNameCache) return playerNameCache;
    try {
      let n = localStorage.getItem('dq.playerName');
      if (!n) {
        n = 'Player-' + Math.floor(1000 + rng() * 9000);
        localStorage.setItem('dq.playerName', n);
      }
      playerNameCache = n;
    } catch { playerNameCache = 'Player'; }
    return playerNameCache;
  }
  function submitRun(won) {
    const body = {
      name: playerName(),
      score: state.score,
      won,
      durationMs: Math.round(state.elapsed * 1000),
      seed: play.seed,
      irHash: play.irHash || '',
      inputs: play.inputLog,
    };
    apiFetch(`/v1/games/${encodeURIComponent(id)}/runs`, {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    }).then(async (res) => {
      if (!res.ok) return;
      const data = await res.json().catch(() => null);
      if (data && data.runId) {
        play.lastRunId = data.runId;
        window.__dqLastRunId = data.runId;
        try {
          window.webkit?.messageHandlers?.hephaestusRunResult?.postMessage({
            schema: 1,
            type: 'verified_run_available',
            sessionId: dqSessionId,
            gameId,
            runId: data.runId,
          });
        } catch { /* native bridge is optional outside iOS */ }
      }
      if (data && data.rank && play.shell) {
        const sub = $('shell-result-sub');
        if (sub && !sub.textContent.includes('Leaderboard')) {
          sub.textContent += ` · Leaderboard: #${data.rank} this week`;
        }
      }
    }).catch(() => {});
  }

  $('again').addEventListener('click', () => restartGame(), { signal: abort.signal });

  // --- Input ----------------------------------------------------------
  function tapActionName() {
    const b = (ir.inputBindings || []).find((x) => x.input === 'tap');
    return b?.action;
  }
  const dragXOnly = ir.experience?.controls?.scheme === 'drag_x';
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
    if (replayTrace) return; // the trace drives; live input would corrupt the replay
    pointer.down = true;
    const p = toLogical(ev);
    pointer.x = p.x;
    // IR shell controls can constrain the pointer to horizontal-only movement.
    // Keep the player in the catcher lane instead of following vertical cursor drift.
    pointer.y = dragXOnly && playerEnt ? playerTarget.y : p.y;
    noteInput('pointer', `${Math.round(p.x)},${Math.round(p.y)}`);
    emit('input', playerEnt, undefined, undefined, tapActionName());
  });
  app.canvas.addEventListener('pointermove', (ev) => {
    if (!pointer.down) return;
    const p = toLogical(ev);
    pointer.x = p.x;
    pointer.y = dragXOnly && playerEnt ? playerTarget.y : p.y;
  });
  addEventListener('pointerup', () => { pointer.down = false; }, { signal: abort.signal });

  const keysDown = new Set();
  const kbBindings = (ir.inputBindings || []).filter((b) => b.input === 'keyboard');
  addEventListener('keydown', (ev) => {
    if (replayTrace) return;
    const b = kbBindings.find((x) => x.key === ev.key || x.action === ev.key);
    if (!b) return;
    keysDown.add(b.action);
    noteInput('key', ev.key);
    emit('input', playerEnt, undefined, undefined, b.action);
  }, { signal: abort.signal });
  addEventListener('keyup', (ev) => {
    const b = kbBindings.find((x) => x.key === ev.key || x.action === ev.key);
    if (b) { keysDown.delete(b.action); noteInput('keyup', ev.key); }
  }, { signal: abort.signal });

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
      recordEvent('collision', other.defId);
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

  let frameNo = 0;
  let tickErrLogged = false;
  let censusLogged = false;
  const computeCensus = () => {
    const parts = [`worldLayer@(${worldLayer.position.x.toFixed(0)},${worldLayer.position.y.toFixed(0)})`];
    let n = 0;
    for (const ent of entities.values()) {
      if (n++ >= 4) break;
      parts.push(`${ent.def.id}@(${ent.view.position.x.toFixed(0)},${ent.view.position.y.toFixed(0)}) a=${ent.view.alpha} vis=${ent.view.visible} scale=${ent.view.scale.x.toFixed(2)}`);
    }
    return parts.join(' | ');
  };
  // --- Replay application (Phase C) -----------------------------------
  // Inputs grouped by sim step. During replay the page ignores live
  // input: the trace drives pointer + keys exactly as recorded.
  const replayByStep = new Map();
  if (replayTrace && Array.isArray(replayTrace.inputs)) {
    for (const inp of replayTrace.inputs) {
      const list = replayByStep.get(inp.tick) || [];
      list.push(inp);
      replayByStep.set(inp.tick, list);
    }
  }
  // Scripted inputs (playtest determinism check): the worker may set
  // window.__dqScripted = [{tick, type, detail}] before play begins;
  // they are applied at their steps exactly like a replay trace, so
  // two runs with the same seed + script must end with the same score.
  let scriptedByStep = null;
  function ensureScripted() {
    if (scriptedByStep || !Array.isArray(window.__dqScripted)) return;
    scriptedByStep = new Map();
    for (const inp of window.__dqScripted) {
      const list = scriptedByStep.get(inp.tick) || [];
      list.push(inp);
      scriptedByStep.set(inp.tick, list);
    }
  }
  const replayLastTick = replayTrace && Array.isArray(replayTrace.inputs) && replayTrace.inputs.length
    ? replayTrace.inputs[replayTrace.inputs.length - 1].tick : 0;
  function applyReplayInputs(step) {
    ensureScripted();
    const list = [...(replayByStep.get(step) || []), ...((scriptedByStep && scriptedByStep.get(step)) || [])];
    if (!list.length) return;
    for (const inp of list) {
      if (inp.type === 'pointer') {
        const [x, y] = String(inp.detail || '').split(',').map(Number);
        if (Number.isFinite(x) && Number.isFinite(y)) {
          pointer.down = true; pointer.x = x; pointer.y = y;
          emit('input', playerEnt, undefined, undefined, tapActionName());
        }
      } else if (inp.type === 'pointerup') {
        pointer.down = false;
      } else if (inp.type === 'key' || inp.type === 'keyup') {
        const b = kbBindings.find((x) => x.key === inp.detail || x.action === inp.detail);
        if (b) {
          if (inp.type === 'key') keysDown.add(b.action); else keysDown.delete(b.action);
        }
      }
    }
  }
  // Pointer-move sampling for the trace: while the pointer is down,
  // record its position whenever it moved enough (or every few steps),
  // so a drag is reproducible step for step. Without this, only taps
  // would replay faithfully.
  let lastSampleX = -1, lastSampleY = -1, lastSampleStep = -99, wasDown = false;
  function samplePointerForTrace() {
    if (replayTrace) return;
    if (pointer.down) {
      const moved = Math.abs(pointer.x - lastSampleX) + Math.abs(pointer.y - lastSampleY);
      if (moved >= 3 || play.simStep - lastSampleStep >= 6) {
        play.inputLog.push({ tick: play.simStep, type: 'pointer', detail: `${Math.round(pointer.x)},${Math.round(pointer.y)}` });
        lastSampleX = pointer.x; lastSampleY = pointer.y; lastSampleStep = play.simStep;
      }
      wasDown = true;
    } else if (wasDown) {
      play.inputLog.push({ tick: play.simStep, type: 'pointerup', detail: '' });
      wasDown = false;
    }
  }

  const STEP_MS = 1000 / 60;
  let simAcc = 0;
  let captureTargetStep = null;
  let capturePaused = true;
  let perfWindowStart = 0;
  app.ticker.add((ticker) => {
    frameNo++;
    totalFrames++;
    // Visible-play clock + perf samples (Phase A telemetry v2): the
    // ticker only runs while visible (pause handler) and started
    // (shell gate), so deltaMS here is honest play time — the fps the
    // probe reports no longer counts loading or hidden time.
    play.visibleMs += ticker.deltaMS;
    play.frameSamples.push(ticker.deltaMS);
    if (ticker.deltaMS > play.longestFrameMs) play.longestFrameMs = ticker.deltaMS;
    if (play.visibleMs - perfWindowStart >= 5000) {
      perfWindowStart = play.visibleMs;
      flushPerfWindow();
    }
    // Player-movement activity + dead-input resolution: an input is
    // dead when, 200ms later, no rule fired, nothing was collected,
    // and the player hasn't moved — the Control metric's raw signal.
    if (playerEnt) {
      if (play.lastPlayerX >= 0) {
        const moved = Math.abs(playerEnt.x - play.lastPlayerX) + Math.abs(playerEnt.y - play.lastPlayerY);
        if (moved >= 6) noteActivity();
      }
      play.lastPlayerX = playerEnt.x; play.lastPlayerY = playerEnt.y;
    }
    while (play.pendingInputs.length && play.visibleMs - play.pendingInputs[0].at >= 200) {
      const inp = play.pendingInputs.shift();
      if (play.lastActivityMs < inp.at) play.deadInputs++;
    }
    // Fixed-timestep simulation (Phase C): game logic advances in
    // exact 1/60 steps from an accumulator, never from raw frame
    // time. A run is therefore a pure function of (seed, input trace)
    // on any display — 60Hz panels get exactly one step per frame as
    // before; 120Hz panels step every other frame; slow devices run
    // up to 3 catch-up steps, then shed time instead of spiraling.
    // This is what makes leaderboard replay verification possible.
    if (!deterministicCapture) simAcc += Math.min(ticker.deltaMS, 250);
    samplePointerForTrace();
    let steps = 0;
    try {
      if (deterministicCapture) {
        // Capture mode is driven by integer simulation steps, not the
        // ticker's wall-clock delta. Advance directly to the requested
        // target in fixed 1/60s increments, including low-fps captures.
        while (captureTargetStep != null && play.simStep < captureTargetStep && steps < 10000) {
          applyReplayInputs(play.simStep);
          tickFrame(STEP_MS);
          play.simStep++;
          steps++;
          if (state.ended) break;
        }
      } else {
        while (simAcc >= STEP_MS && steps < 3) {
          applyReplayInputs(play.simStep);
          tickFrame(STEP_MS);
          play.simStep++;
          simAcc -= STEP_MS;
          steps++;
          if (state.ended) break;
        }
      }
    } catch (err) {
      if (!tickErrLogged) {
        tickErrLogged = true;
        log('RT', 'TICK ERROR frame ' + frameNo + ': ' + (err.stack || err.message));
      }
      return;
    }
    if (deterministicCapture && captureTargetStep != null && play.simStep >= captureTargetStep) {
      capturePaused = true;
      captureTargetStep = null;
      try { app.ticker.stop(); } catch { /* capture remains paused if already stopped */ }
    }
    if (steps === 3 && simAcc >= STEP_MS) simAcc = 0; // shed, don't spiral
    // Scripted-run score snapshot (determinism check): when the
    // worker sets __dqScriptEnd, freeze the score at exactly that
    // step — a poll-based read would overshoot by up to a poll
    // interval of steps and fake a divergence.
    if (scriptedByStep && window.__dqScriptEnd && play.scriptEndScore === undefined &&
        play.simStep >= window.__dqScriptEnd) {
      play.scriptEndScore = state.score;
    }
    // Replay watchdog: a trace that never ends the game fails
    // verification (30 sim-seconds past its last recorded input).
    if (replayTrace && !state.ended && !window.__dqReplay && play.simStep > replayLastTick + 1800) {
      window.__dqReplay = { runId: replayRunId, match: false, reason: 'timeout', expected: replayTrace.score, actual: state.score };
      try {
        apiFetch(`/v1/runs/${encodeURIComponent(replayRunId)}/verification`, {
          method: 'POST', headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ match: false, actualScore: state.score }),
        }).catch(() => {});
      } catch { /* verdict on the window is the evidence */ }
      try { app.ticker.stop(); } catch { /* ignore */ }
    }
    if (steps === 0) return; // no sim advance this display frame
    // Census: first at frame 30 (the probe's boot evidence), then
    // refreshed every 60 frames so the probe verdict stays a LIVE
    // position sample — the playtest worker's softlock check reads
    // movement from it. A frozen census under input is the signature
    // of a frozen world; a one-shot census could never show that.
    if (frameNo >= 30 && (frameNo - 30) % 60 === 0) {
      lastCensus = computeCensus();
      if (!censusLogged) {
        censusLogged = true;
        log('RT', 'CENSUS ' + lastCensus);
      }
    }
  });
  function tickFrame(dtMs) {
    if (state.ended) return;
    const dt = Math.min(dtMs / 1000, 0.05);
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
      if (!dragXOnly && keysDown.has('move_up')) playerTarget.y -= speed * dt;
      if (!dragXOnly && keysDown.has('move_down')) playerTarget.y += speed * dt;
      if (pointer.down) {
        playerTarget.x = pointer.x;
        if (!dragXOnly) playerTarget.y = pointer.y;
      }
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
  }

  play.shell = ir.experience || null;
  // Health comes from the declared failure model, not the runtime
  // default, when the block declares it.
  if (play.shell?.failure?.mode === 'health' && play.shell.failure.health) {
    state.health = play.shell.failure.health;
  }
  play.liveCounts = () => {
    let particles = 0;
    for (const e of Emitter.instances) particles += e.aliveCount();
    return { entities: entities.size, particles };
  };
  window.__dqPlay = play;
  play.startedAt = performance.now();
  play.bootMs = Math.round(performance.now() - started);
  updateHUD();
  sendTelemetryEvent('session_start', {
    seed: play.seed, shell: !!play.shell,
    bootMs: play.bootMs,
    renderer: 'webgl', irVersion: ir.irVersion,
  });
  // Time to interactive is known at boot; report it now so sessions that
  // never reach a run end still contribute it (Hephaestus #61).
  sendSocialDiagnostic('player_ready', 'ready', {
    time_to_interactive_ms: Math.min(play.bootMs, 3600000),
  });

  // --- Shell start gate (IR 1.2) ---------------------------------------
  // Shell games wait at a start card: the world is built but the ticker
  // is stopped and game_start hasn't fired, so nothing moves, no music
  // is attempted, and the player begins when THEY tap — the same
  // gesture that unlocks audio. Legacy games start immediately.
  const shellStartMode = play.shell?.start?.mode || 'tap_to_start';
  function beginPlay(fromProbe) {
    if (play.shellStarted) return;
    play.shellStarted = true;
    $('shell-start').classList.remove('on');
    if (!deterministicCapture) {
      try { app.ticker.start(); } catch { /* ignore */ }
    }
    recordEvent('start', fromProbe ? 'probe-auto' : 'tap');
    emit('game_start', undefined, undefined);
    emit('scene_entered', undefined, undefined);
  }
  play.beginPlay = beginPlay;
  const skipCard = skipStartCardOnce; // consume: one restart skips once
  skipStartCardOnce = false;
  if (play.shell && shellStartMode === 'tap_to_start' && !skipCard) {
    // Draw the world once so the start card floats over the actual
    // game, then freeze the ticker until the player taps Start.
    try { app.renderer.render(app.stage); } catch { /* trap handles it */ }
    try { app.ticker.stop(); } catch { /* ignore */ }
    $('shell-start-title').textContent = ir.game.title || id;
    $('shell-start-obj').textContent = play.shell.objective?.label || '';
    $('shell-start-hint').textContent = play.shell.controls?.hint || '';
    $('shell-start').classList.add('on');
    $('shell-start-btn').addEventListener('click', () => beginPlay(false), { signal: abort.signal });
    if (replayTrace) beginPlay(true); // replays start themselves; nobody taps
    $('status').textContent = play.shell.objective?.label || 'Tap Start to play';
    // The probe does NOT auto-start: the playtest worker verifies the
    // card is shown, then begins play itself via __dqBeginPlay (the
    // shell-conformance checks). A human always taps.
    window.__dqBeginPlay = () => beginPlay(true);
  } else {
    play.shellStarted = true;
    $('status').textContent = play.shell?.controls?.hint || 'Drag, tap, or use the arrow keys';
    emit('game_start', undefined, undefined);
    emit('scene_entered', undefined, undefined);
  }
  log('RT', `start '${ir.game.id}' scene '${scene.id}'`);
  // Capture mode stops Pixi before the first animation frame; render the
  // initial state explicitly so frame zero is a real scene, not a blank canvas.
  if (deterministicCapture) {
    try { app.renderer.render(app.stage); } catch (err) {
      log('RT', 'CAPTURE INITIAL RENDER ERROR ' + (err.message || err));
    }
  }
  probeState.booted = true;
  // Probe mode: emit the verdict immediately and keep it live (2s cadence)
  // so the playtest worker can watch frames, HUD and status while it plays.
  if (probeMode) { emitProbe(); setInterval(emitProbe, 2000); }
  // Playability Kit hook (probe mode only): a READ-ONLY structured snapshot
  // so a user-mimic bot can see entity positions/kinds and drive real
  // pointer events. It accepts no input and exposes no mutable state, so it
  // adds no capability a game could abuse.
  if (probeMode) {
    window.__dqTest = {
      snapshot: () => ({
        t: state.elapsed, score: state.score, health: state.health,
        ended: state.ended, won: state.won, frames: totalFrames,
        simStep: play.simStep, capturePaused,
        started: play.shellStarted,
        player: playerEnt ? { id: playerEnt.defId, x: playerEnt.x, y: playerEnt.y, w: playerEnt.w, h: playerEnt.h } : null,
        entities: [...entities.values()].filter((e) => !e.destroyed).map((e) => ({
          id: e.runtimeId, def: e.defId, kind: e.def.kind,
          x: e.x, y: e.y, w: e.w, h: e.h,
          visible: !!e.view.visible && e.view.alpha > 0.05,
        })),
        world: { w: W, h: H },
      }),
      advanceToStep: (step) => {
        if (!deterministicCapture || !Number.isSafeInteger(step) || step < play.simStep) return false;
        if (step === play.simStep) {
          capturePaused = true;
          captureTargetStep = null;
          app.ticker.stop();
          return true;
        }
        captureTargetStep = step;
        capturePaused = false;
        app.ticker.start();
        return true;
      },
    };
  }
}


window.addEventListener('error', (e) => {
  log('RT', 'UNCAUGHT ' + (e.error?.stack || e.message || 'unknown error'));
  recordError('uncaught', e.error || new Error(e.message || 'unknown error'));
});

window.addEventListener('unhandledrejection', (e) => {
  const err = e.reason instanceof Error ? e.reason : new Error(String(e.reason));
  log('RT', 'UNCAUGHT (promise) ' + (err.stack || err.message));
  recordError('unhandledrejection', err);
});

if (!gameId) {
  listGames();
} else {
  run(gameId).catch((err) => {
    $('status').textContent = 'Failed to load: ' + err.message;
    log('PKG', 'ERROR ' + err.message);
    if (err.stack) log('PKG', 'STACK ' + err.stack.split('\n').slice(0, 6).join(' <- '));
    recordError('boot', err);
    if (probeMode) emitProbe();
  });
}

if (!playPresentation) initializeAccountControls();
