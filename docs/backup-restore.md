# Store backup and restore

The file-backed `MemoryStore` supports versioned, SHA-256-checksummed backups. The snapshot includes games, jobs, ingests, groups, and artifact bytes; artifact bytes are base64-encoded by JSON. Backup writes use a temporary file followed by an atomic rename, with restrictive file permissions.

## Recovery objectives

For an initial small-user deployment, a **provisional target** is an RPO of at most 24 hours and an RTO of at most 4 hours. These are planning defaults, not a production SLA; confirm them against the expected cost of lost creations and downtime before public launch. Until a scheduler, retention policy, and alerting are configured in the deployment, do not claim the service meets either target.
## Create a backup

```sh
go run ./cmd/store-backup -mode backup -state /data/doodlequest/store.json -backup /data/backups/store-$(date -u +%Y%m%dT%H%M%SZ).json
```

Use a backup destination on durable storage separate from the live snapshot. Keep access restricted: artifacts may include private creator media. Copy completed backups to separately managed storage according to the deployment retention policy.

## Restore

Stop service writes before restoring, then run:

```sh
go run ./cmd/store-backup -mode restore -state /data/doodlequest/store.json -backup /data/backups/store-20261025T000000Z.json
```

Restore verifies the backup format version, checksum, and required collections before changing state. It writes the restored snapshot atomically to the configured state path before replacing in-memory data. A corrupt backup is rejected without mutating the active store. A corrupt current snapshot can be repaired by restoring a valid backup; the command intentionally continues after a failed initial load in restore mode.

## PostgreSQL social graph backup

The shared social graph has its own checksummed backup because it is stored in PostgreSQL, not in the file-backed game store:

```sh
export DQ_SOCIAL_DATABASE_URL='postgres://…'
go run ./cmd/socialgraph-backup -mode backup -backup /data/backups/social-graph-$(date -u +%Y%m%dT%H%M%SZ).json
```

To restore, stop application writes and run the same command with `-mode restore` and the chosen backup path. Restore validates the checksum, format version, and relation fields before writing; it is additive and never deletes destination rows. Restore into an empty database for an exact snapshot. The backup includes follow and block relations; saved-game relations will be included once the PostgreSQL saves migration is deployed.
## Operational limits

- The service must be stopped or quiesced during restore to prevent concurrent writes from being overwritten.
- The file-backed store backup does not include separate social-state JSONL files, telemetry files, environment secrets, or PostgreSQL social-graph data. Use the separate social graph backup command and deployment-specific procedures for those sources.
- Verify a restore in a staging environment before relying on it for production recovery.
- This is a local snapshot workflow, not a scheduled remote backup service. Deployment automation must schedule, retain, and monitor backups.
