# Epic #202 — Authorized published-game tap-score Go diagnostic (#439)

## Execution boundary

`POST /v1/games/{id}/semantic-tap-score-v1/diagnose?irSha256=<full-64-hex>&seed=<canonical-u32>` is a **diagnostic**, not a verified-gameplay endpoint. It resolves the server's currently deliverable published Game IR through the existing authorization/moderation/visibility gate. It rejects nonmatching immutable IR hash, noncanonical seed, malformed or oversize canonical semantic trace v2 and the entire Game IR document if *any* scene/rule/action/input/capability falls outside the narrow `semanticreplay.SupportedIR` allowlist. The read-only Go interpreter replays 60 Hz, zero-entity pointer taps into bounded integer score/terminal state, with deterministic seed and ordinals. It never substitutes a simulated partial score for a game containing physics, drag, keyboard, conditions, health, or other unsupported behavior.

The pure `ReplayDiagnosticForRecord(ir, recordID, trace, seed)` API receives a **trusted game record ID**, distinct from creator-authored Game IR `game.id`. The existing detached `ReplayDiagnostic` helper preserves pure test compatibility. The HTTP route obtains the record from `deliverableGame`; the request cannot choose any record ID except the authorization-scoped URL path. Both `traceSha256` and `stateSha256` bind that record identity and the actual published full Game IR SHA. A user-provided seed is only a diagnostic parameter; **it is not server-issued or secret**.

## Threat containment

- `Cache-Control: private, no-store`; `Vary: Authorization, X-User-ID` even for errors; same authorized visibility rules as package delivery
- Strict exact query keyset, canonical unsigned 32-bit seed, media type and at-read 1 MiB request bound; strict canonical wire schema/duplicate/unknown key rejection
- At most **four in-flight diagnostic computations per process**; a fifth returns HTTP 429 with `Retry-After: 1` and private/no-store/authorization-vary headers. Authentication checks happen **before** the slot gate to avoid leaking private game existence under load. This is a fixed safety ceiling, **not** per-user abuse rate limiting or an SLA.
- Only tiny whole-document tap-score capability accepted, bounded 2,000 edges/108,000 ticks/50,000 rule-effect work; no fallback for unknown authored gameplay
- Result exposes diagnostic `score`, `terminal`, fingerprints and work counts but **always** `attested:false, evidenceStatus:unverified, rankingScope:none`, never a verified run or leaderboard position
- No writes to persistent runs, verifier status, scoreboard, user profile, published game or creator permissions

## Senior game engine architect adversarial challenge

The server uses its own code to compute a score from **client-chosen trace/seed**, but does not know that a client ever played that trace. Deployment must separately set request/header timeouts and authenticated per-account or per-IP abuse quotas; a four-slot local semaphore only caps **simultaneous** work and does not prevent repeated sequential requests. This is useful solely for future cross-language conformance and diagnosing capability-bounded rules. It is **not independent gameplay attestation**, nor authorization to mark a score true; a future attestor must own signed session seeds, authenticated trace capture, immutable gameplay version, independent full-capability interpretation, durable verification ledger and verified-only ranks. See #381/#382/#428/#429/#402. The current published games may use more than this tiny allowlist; a 422 is an **expected safe refusal**, not permission to coerce results or broaden the versioned capability.

No native WKWebView, poster, GPU compositor, 50-cycle iPhone/iPad thermal or Apple 4.7 evidence is supplied by this route; all native Epic #202 acceptance stays independent.

**Release:** Senior source-review challenge and all six exact-head Go/race/static, PostgreSQL, Chromium, WebKit, deterministic cross-browser and Playability Kit GitHub Actions success before merging/closing only #439. Atomic rollback removes the route + its new diagnostic API and pure record-ID wrapper; existing `/semantic-input-v2/validate` preflight is unchanged.
