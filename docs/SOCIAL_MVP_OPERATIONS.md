# Social MVP operational and delivery notes

This document records a low-cost path for the early Hephaestus user base. Do not add managed infrastructure until measurements justify it.

## Current operating model

- Follow relationships and social interactions are written to a local JSON snapshot; social events append to JSONL. This is single-instance file persistence only, not shared storage, and has no transactional or multi-replica guarantees. Full shared durable follow storage remains deferred under issue #11.
- Runtime-error telemetry and existing game artifacts have their own current storage behavior; this social layer does not make them durable.
- Moderation approval requires the server secret `DQ_MODERATION_TOKEN`; send it only from a trusted operator workflow as `X-Moderation-Token`. Do not ship the token in the iOS app.
- The current `X-User-ID` is spoofable. Do not use it as a production authorization boundary.
- Set `DQ_SOCIAL_STATE_FILE` and `DQ_SOCIAL_EVENTS_FILE` to writable persistent-volume paths in deployments where state should survive ephemeral working-directory resets.

## Measurements to collect before scaling

Track weekly:
- Published package count and p50/p95 package size.
- Asset/package request count, bytes served, error rate, and p50/p95 latency.
- Feed requests per active user and p50/p95 latency.
- Social event volume and in-memory state growth.
- Moderation reports and time-to-triage.

## Decision triggers

- **Durable shared storage:** before a restart can no longer reasonably reset social state, before public launch, or before multiple backend replicas.
- **Object storage/CDN:** when measured asset bandwidth, request latency, or backend resource use becomes material. Keep package/version identifiers immutable so caches are safe.
- **Rate limiting and abuse controls:** before unrestricted public exposure; prioritize write endpoints and event ingestion.
- **Queues/caching:** only after profiling demonstrates a bottleneck.

## Recovery checklist for the current stage

1. Keep generated source and game-package artifacts backed up through the existing repository/inventory workflow.
2. Record the current environment variables and deployment command in the deployment environment's private runbook (never commit secrets).
3. Before deploys, verify `GET /healthz`, game package retrieval, and the Go test suite.
4. If social state resets, disclose it as expected behavior; do not imply that the current in-memory social graph is backed up.
5. Before public launch, choose durable storage, run a restore drill, and document migration/rollback steps.

## Package delivery safety

- Validate asset references at publication and loading boundaries.
- Keep published package versions immutable and make removed/quarantined content unavailable through discovery.
- Evaluate CDN/object storage only with measured package-size and traffic data; do not cache mutable URLs as if they were immutable versions.


## Web authentication and social graph client contract

- The web player accepts a short-lived bearer token minted by a trusted identity provider. The player does not issue tokens or collect passwords; do not paste provider credentials into this UI.
- The token is stored in tab-scoped `sessionStorage`, cleared by Sign out, and attached only to same-origin API requests. Telemetry beacons do not carry the bearer token.
- The API verifies HMAC-signed claims using `DQ_AUTH_HMAC_SECRET`; claims must use issuer `hephaestus`, a non-empty subject, and an unexpired `exp`. Provision this secret and a trusted issuer/token-minting integration before enabling authenticated production traffic. Never enable `DQ_ALLOW_INSECURE_USER_ID` in production.
- CORS preflight explicitly allows `Authorization`. The browser feed selector uses the API's `scope` query parameter and creator follow controls call `POST /v1/users/{id}/follow`. The following/friends feeds use the PostgreSQL graph when `DQ_SOCIAL_DATABASE_URL` is configured.
- PostgreSQL graph rollout remains a deployment task: provision the database, set `DQ_SOCIAL_DATABASE_URL`, run `go run ./cmd/migrate-social-graph` with a backed-up local snapshot, and verify follow/unfollow plus cross-instance visibility before routing traffic to multiple replicas.
- The UI token field is a bearer-token integration surface, not a complete Sign in with Apple/OIDC flow. A provider callback, PKCE/state validation, key rotation, and deployment-specific issuer integration must be added before claiming a turnkey sign-in experience.
