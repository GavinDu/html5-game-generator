# Provisional leaderboard persisted-replay integrity — Epic #202

Parent [Hephaestus Epic #202](https://github.com/GavinDu/Hephaestus/issues/202), [authoritative verified ranking #402](https://github.com/GavinDu/html5-game-generator/issues/402), [server-owned replay attestor #382](https://github.com/GavinDu/html5-game-generator/issues/382), scoped [#419](https://github.com/GavinDu/html5-game-generator/issues/419).

## Invariant

The run-detail route already responds 409/no-store when a **modern** stored replay's server-computed seed/input/duration/hash evidence fingerprint fails its canonical integrity check. Before this change, the public leaderboard merely labeled that run `evidenceStatus: invalid_evidence` and still sorted its browser-claimed score, win and time into public ranks. A corrupted persisted record could appear first even when its diagnostic replay could not be fetched.

The shared `boardRuns` candidate assembly now uses the same `runEvidenceStatus` check and **skips only `invalid_evidence`**, before any metric sorting or rank enumeration. No persisted record is deleted/rewritten, and the existing dynamic board view continues to authorize game access through merged #417. Valid modern evidence stays `unverified`; historical pre-fingerprint records stay `legacy_unverified` on the **provisional** compatibility board, with legacy client-writable Verified/ActualScore still suppressed.

Go regression tests create a real admitted modern run and assert correct provisional status; insert three otherwise board-compatible forged modern records (seed/input-duration drift, missing explicit modern digest and unknown evidence schema) that would top the time board; verify each individually fails sensitive GET as 409/no-store and none can receive a public rank. Preserve a legacy pre-modern fingerprint entry labeled provisional, contiguous rank numbers for remaining entries, and the complete original audit records intact. Never claim an HTTP score or stored SHA proves gameplay execution.

## Senior game-engine-architect adversarial challenge

- Evaluate GET-versus-board integrity parity, the `legacy_unverified` compatibility exception, ranking continuity, time-board losing-run filtering, and whether an unknown modern evidence schema is always denied. Evidence label is never an attestation.
- This does **not** implement semantic input v2 HTTP publishing, a deterministic Go game reducer, trusted score/win verdicts, rate limits, identity attestation, GPU first presented frame or real iPhone/iPad thermal gate. Full verified-only ranking remains #402 and server-owned attestor #382/#360.
- All six **exact-head** Go test/race/static, PostgreSQL, Chromium, WebKit, determinism and Playability Kit jobs must succeed before merging/closing #419; no bypass. Assistant-led senior game-engine-architect review is not an independent human reviewer or physical-device certification.

Rollback consists solely of removing the invalid evidence candidate filter and its regression/doc.
