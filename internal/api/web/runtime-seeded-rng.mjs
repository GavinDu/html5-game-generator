// Trusted deterministic seed generator for the *installed* Game IR player.
// Mulberry32 is kept byte-for-byte equivalent to the former monolithic
// runtime implementation. Never import this from a game package or author
// an executable script reference in Game IR.
export function createSeededRandom(initialSeed = 1) {
  let state = 0;
  const stream = {
    reseed(seed) {
      // The legacy player intentionally maps zero to one. Preserving this
      // avoids changing existing seed/replay semantics and leaderboard runs.
      state = (seed >>> 0) || 1;
    },
    next() {
      state = (state + 0x6D2B79F5) >>> 0;
      let t = state;
      t = Math.imul(t ^ (t >>> 15), t | 1);
      t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
      return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
    },
    getState() { return state >>> 0; },
  };
  stream.reseed(initialSeed);
  return stream;
}
