# Telemetry event contract and data policy

This document describes the current game-generator telemetry contract. Telemetry is used to diagnose player/runtime failures and measure game quality; it is not intended to identify individual people or optimize time spent as a standalone goal.

## Event schema

The structured player event endpoint is `POST /v1/telemetry/events`. Current events use `schemaVersion: 2` and one of these kinds:

| Kind | Meaning | Fields used by aggregation |
|---|---|---|
| `session_start` | A player session began | gameId, deviceClass, quality |
| `perf_window` | A visible-play frame-time sample | frames, buckets, longestFrameMs |
| `milestone` | A gameplay milestone | milestone, ms |
| `run_end` | A run ended | won, durationMs, inputs, deadInputs |

Every current runtime event includes a per-session `eventId` and `sessionId`. The server acknowledges duplicates with HTTP 202 but stores and indexes the first accepted event only. Deduplication is scoped to the session and held in a bounded in-memory index of 20,000 event IDs; it prevents retries within a process lifetime, not duplicates after a server restart or across multiple replicas. Older clients may omit schemaVersion/eventId and are accepted as schema v2 for compatibility; new clients must send both. Unsupported explicit schema versions are rejected with HTTP 400.

The server returns HTTP 400 for malformed events, unknown kinds, invalid event-ID/session combinations, and unsupported schema versions. It returns HTTP 503 when it cannot persist the event to the JSONL log.

## Derived metrics

`GET /v1/telemetry/perf` returns per-game aggregates: session/run counts, win rate, average run duration, dead-input rate, median first-input and first-reward times, frame count, jank rate, and approximate p95 frame time. The p95 is bucket-edge based, not a raw percentile. These aggregates are an operational quality signal, not a user-level profile.

Runtime error reports are accepted separately at `POST /v1/telemetry/runtime-error`. The server normalizes volatile message details into fingerprints and derives incidents/quarantine signals. Performance samples are not counted as runtime errors.

## Data minimization

- Do not send names, email addresses, account identifiers, precise location, contacts, photos, raw input text, or arbitrary user-generated content in telemetry.
- `gameId` identifies the game, not the player. `sessionId` and `eventId` are random per-session identifiers and must not be reused as account IDs.
- `deviceClass` is limited to device family, viewport dimensions, and device-pixel ratio. Avoid adding full user-agent strings or stable device identifiers to structured events.
- Runtime-error reports can contain stack traces and a short runtime log tail; never put secrets, tokens, or user-entered content in those logs.

## Retention and access — current limitations

Raw events are appended to `events.jsonl` and runtime errors to `runtime-errors.jsonl` under `DQ_TELEMETRY_DIR` (default: `telemetry/`). The current code does **not** automatically expire or compact those files. Until a retention job is implemented, operators must restrict filesystem access and rotate/archive/delete the files under the deployment's data-retention policy. Do not expose the telemetry directory as static web content or commit generated telemetry files.

The in-memory indexes are bounded (20,000 structured session/performance/run/milestone records; 10,000 runtime-error records), but this is a memory bound, not a raw-file retention policy. The current perf/incidents endpoints are aggregate operational endpoints; they are not an authorization boundary. Keep them behind trusted deployment/network controls until endpoint-level authorization is added.

## Validation and rollout

- Regression tests cover accepted event aggregation, unknown-kind/schema rejection, duplicate event IDs, and deduplication of session counts.
- The idempotency index is process-local. Durable/cross-replica idempotency and automatic raw-file retention remain open work.
- Before broad public launch, add an automated retention/rotation job, define operator access controls for aggregate endpoints, and verify telemetry delivery end-to-end on real clients.
