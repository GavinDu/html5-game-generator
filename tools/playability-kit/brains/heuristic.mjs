// "Mimic the user" brain, level 1: a rule-based player that reads the
// structured snapshot and plays with intent (collect pickups, avoid
// hazards). Deterministic-ish, free, and fast, so it runs on every generation.
//
// A brain is any object with:  decide(snapshot, controls) -> { x, y }
// where x,y are the pointer target in LOGICAL game coordinates (the same
// space as snapshot.world). The runner turns that into real mouse/touch
// events, so the game sees exactly what a finger would send.
//
// Like a real player it has a little memory:
//  - it estimates each object's velocity, so it leads falling pickups
//    instead of chasing where they were;
//  - it ignores pickups that are lying still on the floor of a catcher game
//    (a human catches them in the air; chasing them wastes the run);
//  - if it is not getting closer to its target, it gives up on it for a
//    while and tries another, like a person who notices "this isn't working".

const dist = (a, b) => Math.hypot(a.x - b.x, a.y - b.y);
const clamp = (v, lo, hi) => Math.min(hi, Math.max(lo, v));

export function heuristicBrain() {
  let prev = null;                // previous snapshot
  const vel = new Map();          // entity id -> {vx, vy} px/s of game time
  const ban = new Map();          // entity id -> game time until which we ignore it
  let lock = null;                // { id, since, bestD, at }

  return {
    name: 'heuristic',
    decide(snap, controls) {
      const p = snap.player;
      if (!p) return null;
      const { w: W, h: H } = snap.world;
      const now = snap.t;

      if (prev && now > prev.t) {
        const dt = now - prev.t;
        const old = new Map(prev.entities.map((e) => [e.id, e]));
        for (const e of snap.entities) {
          const o = old.get(e.id);
          if (o) vel.set(e.id, { vx: (e.x - o.x) / dt, vy: (e.y - o.y) / dt });
        }
      }
      prev = snap;

      const live = snap.entities.filter((e) => e.visible && e.id !== p.id);
      const hazards = live.filter((e) => e.kind === 'enemy' || e.kind === 'obstacle');
      const freeY = controls.movesY;
      const speed = (e) => Math.hypot(vel.get(e.id)?.vx || 0, vel.get(e.id)?.vy || 0);

      // Where will this pickup be shortly? (lead the target)
      const LEAD = 0.35;
      const future = (e) => {
        const v = vel.get(e.id) || { vx: 0, vy: 0 };
        return { x: clamp(e.x + v.vx * LEAD, 0, W), y: clamp(e.y + v.vy * LEAD, 0, H) };
      };

      const risk = (pt) => hazards.reduce((r, h) => {
        const d = dist(pt, h) - (h.w + h.h) / 4 - (p.w + p.h) / 4;
        return r + (d < 120 ? (120 - Math.max(d, 0)) : 0);
      }, 0);

      // Candidate pickups: skip banned ones and, in catcher games (y locked
      // or lots of motion), skip pickups resting on the floor.
      const anyMoving = live.some((e) => e.kind === 'pickup' && speed(e) > 25);
      const pickups = live.filter((e) => {
        if (e.kind !== 'pickup') return false;
        if ((ban.get(e.id) || 0) > now) return false;
        const resting = speed(e) < 8 && e.y > H * 0.8;
        if (resting && anyMoving) return false;
        return true;
      });

      let target = null;
      if (pickups.length) {
        let best = Infinity;
        for (const pk of pickups) {
          const f = future(pk);
          const above = f.y <= p.y + 20;
          const cost = dist(p, f) + (freeY ? 0 : (above ? 0 : 400)) + risk(f) * 2;
          if (cost < best) { best = cost; target = pk; }
        }
      }

      // Give up on a target we are not getting closer to (blocked / unreachable).
      if (target) {
        const d = dist(p, target);
        if (!lock || lock.id !== target.id) lock = { id: target.id, bestD: d, at: now };
        else if (d < lock.bestD - 6) { lock.bestD = d; lock.at = now; }
        else if (now - lock.at > 1.6) { ban.set(target.id, now + 4); lock = null; target = null; }
      }

      let tx, ty;
      if (target) {
        const f = future(target);
        tx = f.x; ty = freeY ? f.y : p.y;
      } else {
        // Nothing sensible to chase: hover mid-field and sweep slowly, which
        // is what a player does while waiting for the next thing to appear.
        tx = W / 2 + Math.sin(now * 0.9) * W * 0.35;
        ty = freeY ? H * 0.65 : p.y;
      }

      // Evade: if a hazard is about to hit us, step away from it.
      const threat = hazards
        .map((h) => ({ h, d: dist(p, h) }))
        .filter((o) => o.d < 130)
        .sort((a, b) => a.d - b.d)[0];
      if (threat) {
        const away = Math.sign(p.x - threat.h.x) || (Math.random() < 0.5 ? -1 : 1);
        tx = clamp(p.x + away * 110, 30, W - 30);
        if (freeY) ty = clamp(p.y + (Math.sign(p.y - threat.h.y) || 1) * 110, 30, H - 30);
      }
      return { x: clamp(tx, 20, W - 20), y: clamp(ty, 20, H - 20) };
    },
  };
}
