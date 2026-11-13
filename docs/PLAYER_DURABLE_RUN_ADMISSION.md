# Replay admission requires durable trace persistence — Epic #202 / #360

## Source-confirmed failure and why this matters

Before this change the Go `handleSubmitRun` handler inserted an allegedly
accepted run into its in-memory leaderboard, unlocked the record mutex, and
unconditionally returned HTTP 201 and a new `runId`. The subsequent
`appendRunFile` silently ignored errors from directory creation, log open,
JSON marshal and file write. The accepted identifier could vanish on the next
process restart; the browser and iOS host could never independently retrieve
its replay trace. Under simultaneous requests, the memory ranking order was
also no longer protected by the same mutex that sequenced JSONL append.

## Implemented narrow fail-closed boundary

- Marshal the canonical `RunRecord` and append its JSONL entry **while
  holding the existing in-process `runsMu`**. The attempt is published in
  memory and given a leaderboard rank **only after** the file write and
  `File.Sync()` return successfully.
- On file/directory/open/write/sync error, respond **503** with a generic
  `unable to persist run` error: do not return `runId`, do not append
  the record to the in-memory board and do not disclose the private log path.
- Newly created replay-log files are mode **0600** because the seed and input
  history can be private; existing on-disk permissions are not silently
  migrated or rewritten.
- Keep old `runs.jsonl` format and read compatibility, 128-bit run IDs,
  the existing content authorization rules, score guard and fixed-tick input
  grammar. This is not a gameplay-semantic change.
- Go HTTP regression accepts a first run, then points the log at two
  independently invalid paths (a parent file where directory needed and
  an existing directory in place of the log file). Each submission must
  return 503 and not create a ghost board entry or a phantom ID. Restore
  the log path, accept a second distinct run and recreate a new Server
  against the persisted disk to verify both accepted IDs still resolve.
  The test also inspects new file privacy.

## Senior engine-architect challenger / remaining limits

This improves **single-process, ordinary I/O failure handling**. It is not
a transactional multi-process append log. In particular:

- An I/O error during or after a short/partial write or failing `Sync()`
  may leave an incomplete or even later-flushed JSONL record despite the
  HTTP failure. Do not claim exactly-once durable commits or crash
  consistency under storage faults from this patch alone.
- Multiple Server processes still race for the JSONL file: the mutex is
  process-local. Use a real database transaction/unique index and a
  versioned idempotency key for durable exactly-once admission if
  multi-instance delivery becomes a supported topology.
- Historically duplicated run IDs and browser-written `Verified=true`
  values require explicit migration/audit; this change neither rewrites
  evidence nor attests scores. [Server-owned replay verifier #360](https://github.com/GavinDu/html5-game-generator/issues/360) remains P0.
- `File.Sync()` increases submit latency and serializes successful
  submissions under `runsMu`. Benchmark sustained run submissions before
  wider deployment; batch/transactional storage should replace JSONL
  rather than dropping the durability gate.
- Physical iPhone/iPad replay parity, P50/P95 latency, thermal and game
  start/teardown lifecycle testing remain Epic #202 acceptance gates.

## Integrated predecessor and test-fixture review — 2026-11-13

This PR has been reconstructed as a single independent commit atop merged
async lifecycle [PR #366](https://github.com/GavinDu/html5-game-generator/pull/366)
(`a0b3eb16bd974430e9d53ca3316131b9c9ab37c0`), retaining exactly
three intended files. Prior source head failed `gofmt` in Go Actions; the
extraneous blank line was removed. The disk-failure test uses a completed
winning run because the fixture game's public board only ranks successful
time-trial completions. None of these test repairs bypass error assertions.
Rerun the entire six-job validation on the new integrated head.

**Release:** Merge only after exact-head Go, PostgreSQL, Chromium, WebKit,
Playability Kit and cross-browser determinism pass and the senior
architect verifies the API responses, malformed storage fixtures and
permission regression. Never relabel this partial fix as the independent
server replay attestor or close Epic #202.
