# Run ID provenance across process restarts — Epic #202 / generator #362

## Correctness issue

The legacy persisted leaderboard used `run-YYYYMMDD-<counter>`. Its
counter lived only in the Go `Server` struct and reset after a restart;
`loadRunsLocked` loaded `runs.jsonl` entries but did not restore it.
Two same-day accepted runs around a restart could therefore share an ID,
causing `GET /v1/runs/{id}` to return the **older** trace.

## Adopted policy

- Each new accepted run ID contains a `run-YYYYMMDD-` prefix for
  continuity with logs, plus **128 bits from crypto/rand** (32 lowercase
  hex digits). This identifier is not a deterministic gameplay seed and
  must never derive from a game, score, identity or client claim.
- A cryptographic random failure returns HTTP 503 instead of falling back
  to a guessable/reused counter. Before persistence, the process also
  checks the loaded in-memory run records for a colliding ID and fails
  closed on any collision. Read API paths and historical stored IDs are
  unchanged.
- A Go HTTP regression actually creates a new server against the same
  on-disk run history, submits another run, verifies distinct IDs and
  retrieval scores, and then admits 64 more unique same-day runs. This is
  behavioral evidence, not a claim of mathematical collision impossibility.
- Native production's `runId` message limit of 128 characters covers
  the 45-character new value. Its verified-run API checks remain independent
  of the ID shape.

## Unresolved architecture challenges

This prevents **new** process-restart sequence collisions, but does not
repair historic already-duplicated IDs or make the append-only JSONL storage
safe for multiple simultaneous writer processes. Multi-instance deployment
requires transactional durable storage, unique index, permission-aware replay
access, a versioned immutable package digest, auth-epoch binding and a
server-owned replay verifier (generator #360). The ID is a locator, **not
a bearer secret or proof of a verified outcome**.

Keep #362 open until exact-head CI and senior-architect review complete.
Do not close parent Epic #202 or security issue #360 on this basis.
