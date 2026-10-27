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

Raw events are appended to `events.jsonl` and runtime errors to `runtime-errors.jsonl` under `DQ_TELEMETRY_DIR` (default: `telemetry/`). On startup, the server can prune expired records from these files and the moderation audit JSONL file when `DQ_TELEMETRY_RETENTION_DAYS` is configured to an integer from 1 to 3650. For example, set it to `30` for a 30-day retention window. The setting is deliberately opt-in: when unset, no existing telemetry is deleted. Pruning uses the record's `receivedAt` or `created_at` timestamp and atomically rewrites a file only when expired records are found; malformed or undated lines are preserved for manual review. Keep filesystem access restricted and never expose the telemetry directory as static web content or commit generated telemetry files.

The in-memory indexes are bounded (20,000 structured session/performance/run/milestone records; 10,000 runtime-error records), but this is a memory bound, not a raw-file retention policy. The aggregate `GET /v1/telemetry/perf` and `GET /v1/telemetry/incidents` endpoints require `DQ_TELEMETRY_ADMIN_TOKEN` and the `X-Telemetry-Admin-Token` header. If the token is not configured, the endpoints fail closed with HTTP 503; a missing or incorrect token receives HTTP 403. Use a high-entropy secret, never ship it in a client app, and retain network-level restrictions as defense in depth.

## Validation and rollout

- Regression tests cover accepted event aggregation, unknown-kind/schema rejection, duplicate event IDs, and deduplication of session counts.
- The idempotency index is process-local. Durable/cross-replica idempotency and automatic raw-file retention remain open work.
- Before broad public launch, add an automated retention/rotation job, define operator access controls for aggregate endpoints, and verify telemetry delivery end-to-end on real clients.

## Package and asset delivery metrics

The token-protected `GET /v1/telemetry/delivery` endpoint reports aggregate request counts for the package and asset delivery routes. Send the configured `DQ_TELEMETRY_ADMIN_TOKEN` in the `X-Telemetry-Admin-Token` header.

Per route, the response includes total requests, successful responses, `304 Not Modified` responses, 4xx and 5xx counts, response-body bytes written, cumulative latency in milliseconds, and maximum observed latency. The counters are process-local and reset when the service restarts. They deliberately omit game IDs, user IDs, request paths, and headers.

Use these counters to establish a baseline before selecting object storage/CDN infrastructure. They are not durable analytics, do not measure cache hits at an external CDN, and do not replace a production monitoring system. Configure the admin token before exposing operator telemetry; an unset token makes the endpoint unavailable.


## Social feed preview and play-quality events

`POST /v1/telemetry/social-event` accepts the existing event kinds and an optional `schema_version`. Omitted versions remain compatible and are treated as schema v1; unsupported explicit versions are rejected. Event IDs remain scoped to the authenticated viewer and duplicate delivery acknowledges the first stored event without overwriting its measurements.

Schema v1 optionally accepts these non-negative, bounded measurements:

- `preview_dwell_ms`: time a preview card remained visible.
- `time_to_interactive_ms`: time from launch request to an interactive game.
- `time_to_first_input_ms`: time until the first user input.
- `frame_time_p95_ms` and `frame_time_avg_ms`: client-computed frame-time summary, in milliseconds.
- `frame_count`: frames represented by the summary.

Dwell and startup/input times are capped at one hour; frame-time values are capped at 60 seconds. These bounds catch corrupt or mis-scaled measurements, not user behavior. Keep values aggregate and avoid adding device IDs, account identifiers beyond the authenticated server context, precise location, raw input, or full user-agent strings. The server stores the event once; downstream ranking should use only events that pass the established eligibility and visibility rules.

The web runtime reports the timing fields itself (Hephaestus #61), because
the bridgeless iOS shell cannot observe them: `player_ready` fires at boot
with `time_to_interactive_ms` (boot time to a built, interactive game), and
`player_session_summary` fires at run end with `time_to_interactive_ms`,
`time_to_first_input_ms` (visible-play time to the first input; zero means
none was recorded), and the frame summary (`frame_time_p95_ms` estimated
from the run's frame-time histogram buckets, `frame_time_avg_ms`,
`frame_count`). The runtime clamps values to the caps above so one
pathological frame cannot drop a summary. Event IDs are
`<session>:run<restart>:ready` / `<session>:run<restart>:summary`, so a
retried delivery is deduplicated instead of double-counted. Both kinds are
diagnostics: they are excluded from feed-ranking signals, and they arrive
anonymous when the tab holds no session token. The runtime does not send a
summary for runs that never end; time to interactive for those sessions is
still covered by `player_ready` at boot.
