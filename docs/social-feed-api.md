# Social feed API MVP

Social state uses a local JSON snapshot plus an append-only JSONL event file. Follow edges, reactions, comments, saves, blocks, reports, topic mutes, not-interested feedback, and comment idempotency records are written to the local snapshot. This is single-instance file persistence only; it is not shared storage, has no schema-migration/transaction guarantees, and does not make multi-replica behavior safe. The `X-User-ID` header remains a development identity, not authentication. This is an intentionally small-user-base implementation, not a production security boundary.

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
- Once published, a post's game IR cannot be changed by repair or difficulty-tuning actions. Create a remix or new draft to publish a changed version; existing post links stay attached to the published game ID.
- `GET /v1/games/{id}/post` returns an eligible published post and basic engagement counts.

## Creator discovery

- `GET /v1/creators?q=puzzle&topic=family&limit=20&cursor=creator-id` lists creators with eligible public games.
- Search matches creator IDs or public game titles case-insensitively. `topic` is an optional exact, case-insensitive topic filter; a leading `#` is accepted.
- Private, quarantined, pending, and rejected posts are excluded. Results include creator ID, eligible public post count, and distinct topics.
- Pagination is stable by creator ID; `limit` defaults to 20 and must be 1–50.
- This endpoint exposes creator IDs rather than unverified display names or avatars; private creator activity is not included.

## Interactions

- `POST /v1/games/{id}/reactions` with reaction `like|love|funny|wow`; `DELETE` removes the current demo user's reaction.
- `GET|POST /v1/games/{id}/comments`; body is `{"body":"..."}`. The comment author can delete via `DELETE /v1/games/{id}/comments/{commentID}`.
- `POST|DELETE /v1/games/{id}/save`; `GET /v1/users/me/saved`.
- `POST /v1/games/{id}/report` with `{"reason":"..."}`. Moderators resolve via `POST /v1/moderation/reports/{reportID}/resolve?reason=...`; the report record retains resolution time, actor label, and reason. Repeated resolution does not overwrite the original audit fields. `X-User-ID` is still a development identity, so the actor label is not a production-grade identity assertion.
- `POST|DELETE /v1/users/{creatorID}/block`.
- `POST /v1/games/{id}/feedback` with `{"kind":"not_interested"}`.
- `GET /v1/users/me/preferences` returns muted topics and not-interested game IDs.
- `PUT /v1/users/me/preferences/topics` with `{"muted_topics":["puzzle"]}` replaces the current muted-topic list; matching topics are excluded from the For You feed (case-insensitive).
- `DELETE /v1/users/me/preferences` clears topic mutes and not-interested feedback so recommendations can be reset.
- `POST /v1/games/{id}/comments` accepts an optional `Idempotency-Key` header (up to 128 characters). Reusing the same key with the same user/game/body returns the original comment; reusing it for a different request returns HTTP 409. Idempotency keys are stored in the same local social snapshot, so this is single-instance persistence, not distributed idempotency.

## Moderation

- `GET /v1/moderation/reports` lists reports; `POST /v1/moderation/reports/{reportID}/resolve` resolves one.
- `POST /v1/games/{id}/moderation` with `{"status":"approved|rejected|quarantined","reason":"..."}`.
- Both moderation endpoints require `DQ_MODERATION_TOKEN` to be configured and the matching `X-Moderation-Token` header. If the secret is not configured, they return 503 rather than allowing an unprotected moderation action.
- Reports are persisted in the local social-state snapshot; a dedicated triage dashboard and tamper-evident moderation audit trail remain future work.
- The server does not yet automatically analyze game content. The publication gate is a workflow/control point, not a claim that automated content moderation exists.

## Events and ranking

- `POST /v1/telemetry/social-event` accepts `impression|play_start|completion|replay|share|save|follow|quick_exit`, with optional `duration_ms` and `expected_duration_ms`. An optional `event_id` (maximum 128 characters) makes retries idempotent per viewer within the retained in-memory event window; duplicate IDs return 202 without appending a second record. Legacy requests without IDs retain at-least-once behavior. Existing runtime `session_start` events map to play/replay signals and successful `run_end` events map to completion.
- For You currently uses a small explainable baseline: recency plus bounded engagement from reactions, completions, replays, saves/shares, and a quick-exit penalty. It is not a trained/personalized recommender. Engagement ranking is held behind a simple low-data guardrail: while fewer than 20 indexed social events plus reaction counts exist, the feed stays newest-first. This avoids overreacting to a handful of early events; the threshold should be evaluated against real usage before changing.
- Not-interested and block feedback remove matching content from that viewer's feed.
- Avoid interpreting raw play duration as quality: normalize by expected game length/type when evaluating future ranking.

## Follow relationships and persistence

Follow edges are the one social relation whose shared persistence is no
longer deferred, when PostgreSQL is configured (`DQ_SOCIAL_DATABASE_URL`):

- Storage: `social_follows` rows, created by `Migrate` (`CREATE TABLE IF
  NOT EXISTS`, no separate schema-version table yet). The primary key
  `(viewer_id, creator_id)` enforces uniqueness, and `Follow` inserts with
  `ON CONFLICT DO NOTHING`, so follow and unfollow are both idempotent
  and concurrent follows of the same edge converge to exactly one row
  (covered by `TestPostgresConcurrentFollowCreatesOneEdge` and
  `TestPostgresStoreSharesFollowEdgesAcrossInstances`).
- Survival and sharing: edges survive restarts and deployments and are
  visible across API instances through the shared database. Without
  PostgreSQL, the fallback is the local social-state snapshot file
  (`TestFollowsSurviveRestart`); in-memory-only mode still loses edges
  on restart and is not shared between replicas.
- Migration and recovery: a file snapshot's follow edges import
  idempotently via `ImportFollows`, and the socialgraph backup (format
  v4) includes `social_follows`; a full restore needs the store backup
  plus the socialgraph backup together (see the backup/restore drill).
- Following and Friends feeds read the same stored relationships
  (`ListFollowing` / `ListFollowers` through the shared graph), so a
  feed cannot show a follow the store does not have, or miss one it
  does, when PostgreSQL is configured.
- Account deletion: there is no account-deletion or account-disable
  operation yet, and users are opaque ID strings with no user table.
  Until one lands, follow edges persist until explicitly unfollowed;
  when deletion is implemented it must remove every edge where the
  account is either viewer or creator. That removal is future work,
  not a currently enforced behavior.

## Deferred security and persistence work

Do not treat `X-User-ID` as authentication. Social relations and interactions are persisted in a local JSON snapshot (default `telemetry/social-state.json`, override with `DQ_SOCIAL_STATE_FILE`); social events append to `telemetry/social-events.jsonl` (override with `DQ_SOCIAL_EVENTS_FILE`). This is local single-instance file persistence, not shared durable storage. For relations other than follow edges (see above), full shared persistence, transactional guarantees, migrations, and concurrent multi-instance behavior remain deferred under issue #11 and must be addressed before multiple replicas or relying on this data for production.
