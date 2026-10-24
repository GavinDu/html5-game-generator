# Single-instance backup, restore, and recovery runbook

This runbook is for the current low-traffic, single-backend deployment. It is **not** a multi-replica guarantee: JSON snapshots and JSONL logs are local files, some indexes are process-local, and there is no cross-file transaction. Do not run multiple backend replicas against the same files.

## State files to preserve

Resolve each path from the deployment environment before operating. Defaults are relative to the service working directory:

| Data | Environment variable | Default |
|---|---|---|
| Games, jobs, ingests, groups, artifacts | `DQ_STORE_FILE` | `telemetry/store-state.json` |
| Social relationships/preferences/reports | `DQ_SOCIAL_STATE_FILE` | `telemetry/social-state.json` |
| Social events | `DQ_SOCIAL_EVENTS_FILE` | `telemetry/social-events.jsonl` |
| Telemetry and leaderboard runs | `DQ_TELEMETRY_DIR` | `telemetry/` |
| Moderation audit log | `DQ_MODERATION_AUDIT_FILE` | `<DQ_TELEMETRY_DIR>/moderation-audit.jsonl` |

The telemetry directory may contain `events.jsonl`, `runtime-errors.jsonl`, `moderation-audit.jsonl`, and `runs.jsonl`. Back up the entire directory, not only the files currently present. Also preserve the `inventory/` directory and deployment configuration needed to reproduce the service. Never put secrets or an environment dump in the backup archive.

## Backup procedure

1. Pause writes or stop the backend service. This avoids a backup containing snapshots and append-only logs from different moments.
2. Resolve any custom paths above and make sure all relevant files are on persistent storage.
3. Create a compressed archive with restrictive permissions. Example for the default layout:

   ```sh
   umask 077
   mkdir -p backups
   tar -czf "backups/dq-$(date -u +%Y%m%dT%H%M%SZ).tar.gz" telemetry inventory
   chmod 600 backups/dq-*.tar.gz
   sha256sum backups/dq-*.tar.gz > backups/SHA256SUMS
   ```

   If any state path is outside these directories, include it explicitly. Store a copy of the archive away from the running deployment; an archive on the same ephemeral disk is not disaster recovery.

4. Restart the service and check `GET /healthz`. Record the backup timestamp and the deployed commit alongside the archive.

## Restore drill / recovery

1. Stop the service and preserve a copy of the current files before replacing anything.
2. From the project root, verify the backup checksum (`sha256sum -c backups/SHA256SUMS`) and inspect the archive listing before extraction.
3. Restore files to their configured paths, retaining restrictive permissions (state snapshots should be readable/writable only by the service account). Do not restore a snapshot while the service is writing.
4. Start one backend instance and inspect logs for persistence/load errors. The service currently logs and continues in memory if the store snapshot cannot be loaded, so a healthy HTTP response alone does not prove state was restored.
5. Validate `GET /healthz`, fetch a known published game's package and asset, verify expected inventory games, inspect the feed/social state, and verify the most recent expected run and moderation audit record. Do not use real user content in test fixtures.
6. If validation fails, stop the service and restore the preserved pre-restore copy. Do not overwrite a good backup with a failed recovery attempt.

## Automated coverage and limits

- `go test ./internal/store` includes `TestPersistentStoreSurvivesRestart`, which exercises reload of games, jobs, groups, and artifacts from a fresh store using the same snapshot.
- API tests cover social-state persistence/restart, telemetry retention, and moderation/audit behavior. Run `go test ./...` before deployment.
- These tests are not a production-volume backup restore drill and do not prove that the deployment's persistent disk survives a platform-level loss. Schedule a manual restore exercise before broad public launch.
- The store and social JSON files are write-through single-instance persistence, not shared transactional storage. Rate limits, verified identity, cross-replica deduplication, and multi-instance consistency remain separate work; do not infer these protections from this runbook.
- Keep `DQ_TELEMETRY_RETENTION_DAYS` opt-in and documented. Retention pruning is not a backup policy; keep backup retention and access controls separate.

## Multi-instance gate

Do not scale the service to multiple replicas until shared storage/database semantics, schema migration and rollback, uniqueness/idempotency, operator authentication, rate limiting, and a real restore/load test are implemented. The current JSON snapshot design intentionally favors simplicity for the small user base.
