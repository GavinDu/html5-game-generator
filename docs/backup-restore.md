# Store backup and restore

The file-backed `MemoryStore` supports versioned, SHA-256-checksummed backups. The snapshot includes games, jobs, ingests, groups, and artifact bytes; artifact bytes are base64-encoded by JSON. Backup writes use a temporary file followed by an atomic rename, with restrictive file permissions.

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

## Operational limits

- The service must be stopped or quiesced during restore to prevent concurrent writes from being overwritten.
- This file-backed store backup does not include separate social-state JSONL files, telemetry files, environment secrets, or PostgreSQL social-graph data. Back those up with their own database/filesystem procedures.
- Verify a restore in a staging environment before relying on it for production recovery.
- This is a local snapshot workflow, not a scheduled remote backup service. Deployment automation must schedule, retain, and monitor backups.
