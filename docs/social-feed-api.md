# Social feed API MVP

All social state is currently process-local and resets on restart. The `X-User-ID` header remains a development identity, not authentication. This is an intentionally small-user-base implementation, not a production security boundary.

## Feeds and creator profiles

- `GET /v1/feed?scope=for_you`: eligible public games ranked by a transparent baseline combining freshness and observed engagement.
- `GET /v1/feed?scope=following`: eligible public games by followed creators.
- `GET /v1/feed?scope=friends`: eligible public games by mutual follows.
- `GET /v1/users/{creatorID}`: public creator posts, counts, and current viewer follow/mutual state.
- `GET /v1/users/me/following`: `{"creator_ids":[]}`.
- Unknown feed scopes return HTTP 400. Empty feeds return `{"games":[]}`. Feed and comment lists accept `limit` (1–50, default 20) and an ID-based `cursor`; responses include `next_cursor` when another page exists. Cursors are best-effort anchors rather than snapshot isolation while engagement scores change.

## Publish and versioning

- `POST /v1/games/{id}/publish` with `{"caption":"...","topics":["puzzle"],"remix_parent_id":"optional"}`.
- A game must be ready, not quarantined, and owned by the caller's demo identity.
- The first published version captures caption/topics/remix attribution and cannot be overwritten through this endpoint. Create a new game/version to revise the post.
- New posts enter `pending` moderation and are excluded from feeds until approved.
- `GET /v1/games/{id}/post` returns an eligible published post and basic engagement counts.

## Interactions

- `POST /v1/games/{id}/reactions` with reaction `like|love|funny|wow`; `DELETE` removes the current demo user's reaction.
- `GET|POST /v1/games/{id}/comments`; body is `{"body":"..."}`. The comment author can delete via `DELETE /v1/games/{id}/comments/{commentID}`.
- `POST|DELETE /v1/games/{id}/save`; `GET /v1/users/me/saved`.
- `POST /v1/games/{id}/report` with `{"reason":"..."}`.
- `POST|DELETE /v1/users/{creatorID}/block`.
- `POST /v1/games/{id}/feedback` with `{"kind":"not_interested"}`.

## Moderation

- `GET /v1/moderation/reports` lists reports; `POST /v1/moderation/reports/{reportID}/resolve` resolves one.
- `POST /v1/games/{id}/moderation` with `{"status":"approved|rejected|quarantined","reason":"..."}`.
- Both moderation endpoints require `DQ_MODERATION_TOKEN` to be configured and the matching `X-Moderation-Token` header. If the secret is not configured, they return 503 rather than allowing an unprotected moderation action.
- Reports are persisted in the local social-state snapshot; a dedicated triage dashboard and tamper-evident moderation audit trail remain future work.
- The server does not yet automatically analyze game content. The publication gate is a workflow/control point, not a claim that automated content moderation exists.

## Events and ranking

- `POST /v1/telemetry/social-event` accepts `impression|play_start|completion|replay|share|save|follow|quick_exit`, with optional `duration_ms` and `expected_duration_ms`. Existing runtime `session_start` events map to play/replay signals and successful `run_end` events map to completion.
- For You currently uses a small explainable baseline: recency plus bounded engagement from reactions, completions, replays, saves/shares, and a quick-exit penalty. It is not a trained/personalized recommender.
- Not-interested and block feedback remove matching content from that viewer's feed.
- Avoid interpreting raw play duration as quality: normalize by expected game length/type when evaluating future ranking.

## Deferred security and persistence work

Do not treat `X-User-ID` as authentication. Follow edges remain in memory and disappear on restart by deliberate deferral. Reactions, comments, saves, blocks, feedback, and reports are written to a local JSON snapshot (default `telemetry/social-state.json`, override with `DQ_SOCIAL_STATE_FILE`). Events append to `telemetry/social-events.jsonl` (override with `DQ_SOCIAL_EVENTS_FILE`). This is single-instance persistence, not shared durable storage. Those limitations are intentionally deferred while the app has few users, but must be revisited before relying on these features for public production use or multiple server instances.
