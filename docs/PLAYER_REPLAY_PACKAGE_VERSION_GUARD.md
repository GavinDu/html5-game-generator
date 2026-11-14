# Replay package provenance and keyboard admission during playback

Parents: [Hephaestus Epic #202](https://github.com/GavinDu/Hephaestus/issues/202), [Generator deterministic parity #315](https://github.com/GavinDu/html5-game-generator/issues/315), [Server attestation #360](https://github.com/GavinDu/html5-game-generator/issues/360).

## Confirmed defect

The replay URL already checks run ID, game ID, seed and trace shape, but the playback host checked the stored `irHash` against the *current* package only for deterministic screenshot capture. Ordinary `?replay=...` could therefore run an old score/inputs against a newer published Game IR. It also accepted historical `key`/`keyup` event details absent from the current IR, then silently ignored the unbound key in `applyReplayInputs`. Both yield misleading 'deterministic' diagnostic results.

## Applied playback-only guard

- Require a valid current package short board hash and full SHA-256. Every explicit replay must match the stored short IR hash, regardless of preview-capture flag.
- If the run stores the modern full `irSha256`, require an exact 64-hex package match and ensure its prefix agrees with the short board key. Replays carrying only the historical 12-hex board key remain **diagnostic-only** and explicitly return `attestable: false`; they cannot be promoted to trusted scores.
- Validate all stored keyboard down/up event details against the published IR's exact keyboard `key`/`action` bindings **before** allocating a game scene or starting a ticker. Tap-only games with historic rogue keyboard traces fail closed instead of skipping actions.
- Preserve the existing 2,000-event/time-horizon input preflight, no privileged native/JS bridge, and no dynamic executable content. Backend GET delivery still controls private/removed package and trace access.

## Senior game-engine-architect challenge and rollout exclusions

This is a browser-side *consistency check*, not independent replay verification. A malicious client can alter both requested run data and JavaScript execution; hashes of data from the same server are provenance labels, not attestations of semantic transitions. Legacy 12-hex-only traces cannot rule out theoretical prefix collisions or reconstruct missing historical input edges. A known immutable package digest must be pinned to each server-admitted modern run and replayed by a separate server-owned bounded interpreter before any `Verified=true` result is published.

Required evidence: Go route and HTTP tests; Node negative mutations and published keyboard grammar; browser Chromium/WebKit playability and cross-browser determinism at exact PR SHA. Full typed input protocol, physical iPhone/iPad, permission revocation, versioned physics and rule conformance remain Epic blockers. This patch does not modify iOS or server leaderboard verification.
