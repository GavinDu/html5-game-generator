# Recovery and scaling runbook

This runbook defines the safe sequence for validating recovery and preparing the service for more than one backend instance. Do not scale out until all state used by the deployment is durable and shared.

## State inventory

| State | Current handling / known boundary | Required before multi-instance deployment |
|---|---|---|
| Game inventory and package assets | Shipped sample games are under `inventory/<id>/`; delivery reads persisted Game IR and assets. | Put generated artifacts and mutable package metadata in shared durable storage; keep published versions immutable and assets available through a shared origin. |
| Social follow graph | Optional PostgreSQL store configured with `DQ_SOCIAL_DATABASE_URL`; local snapshot migration uses `DQ_SOCIAL_STATE_FILE`. | Verify every instance uses the same database and migrations complete before readiness succeeds. |
| Telemetry events | Appends JSONL records to `events.jsonl`; event-ID index is restored at startup. | Move event storage to shared durable storage or a durable event service before multi-instance deployment; a local file is not a shared event log. |
| Other social state | Blocks, mutes, comments, saved games, reports, moderation audit, and game artifacts are not all covered by the follow-graph migration. | Inventory and migrate each state domain independently. |
| Runtime configuration and secrets | Environment-based configuration. | Use deployment secret management and apply identical reviewed configuration to each instance. |

## Backup and restore drill

Run this in a disposable staging environment first. Never test destructive restore steps against production data.

1. Record the deployment version, configuration keys (not secret values), and locations of all persistent state.
2. Pause writes or use each store's consistent-snapshot mechanism. Copy the `DQ_SOCIAL_STATE_FILE` snapshot if local follow-graph state is still in use. Back up generated game artifacts, package metadata, moderation state, and telemetry separately wherever they are persisted.
3. Record checksums, byte sizes, timestamps, and a manifest of included files/objects. Store the backup outside the service instance and restrict access.
4. Restore into an isolated environment using the same application version and compatible schema. Do not overwrite source data.
5. Run API and storage tests, then verify health/readiness, package retrieval, asset retrieval, public feed visibility, follow/unfollow persistence, telemetry deduplication after restart, and moderation report/audit visibility.
6. Compare restored counts and checksums where applicable. Confirm unpublished games are not served from stale caches and invalid persisted Game IR is rejected at delivery.
7. Record restore duration, recovery point, missing state, and corrective actions. The drill passes only when every check has an explicit result.

## Social follow-graph migration

For deployments using PostgreSQL follow-graph storage:

1. Back up the source snapshot and destination database before migration.
2. Provision PostgreSQL and set `DQ_SOCIAL_DATABASE_URL` in staging.
3. Run `go run ./cmd/migrate-social-graph` with the source snapshot path and destination DSN, following the command's current help/usage.
4. Re-run the migration to verify idempotence; confirm it does not delete destination data.
5. Verify follow/unfollow, following and mutual-friends feed scopes, restart persistence, and cross-instance visibility.
6. Roll out only after readiness checks and GitHub Actions pass. Retain the source snapshot until post-rollout verification and the rollback window are complete.

## Multi-instance release gate

Do not increase the backend replica count until all are true:

- [ ] Generated Game IR, assets, and publication metadata are shared and durable.
- [ ] Follow graph uses shared PostgreSQL; migrations are complete and verified.
- [ ] Blocks, mutes, comments, saved games, reports, moderation audit, and remaining mutable state have an explicit shared-storage plan.
- [ ] Telemetry no longer depends on an instance-local JSONL file for cross-instance completeness or deduplication.
- [ ] Readiness fails closed when required storage is unavailable or migrations are incomplete.
- [ ] Cache invalidation and unpublish behavior are verified across instances.
- [ ] A staging backup/restore drill passes and recovery time/data-loss objectives are documented.
- [ ] GitHub Actions tests, race detector, static analysis, browser playtests, and relevant integration tests pass.

## Rollback principles

- Prefer rolling back application code while preserving forward-compatible data.
- Do not automatically reverse schema migrations or delete destination records during rollback.
- Keep the pre-migration backup until the new deployment is verified and the agreed rollback period ends.
- If shared storage is unavailable, remove unhealthy instances from service and fail readiness rather than silently falling back to instance-local state.
- Capture the incident timeline and recovery evidence without logging user identifiers or secrets.
