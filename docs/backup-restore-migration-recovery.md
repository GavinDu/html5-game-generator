# Backup, restore, and migration recovery drills

Status: operational runbook for the current file-backed MVP store. This document describes procedures and invariants; it does not claim that a production staging drill has been executed.

## Scope and current durability boundary

The current `internal/store.MemoryStore` persists a JSON snapshot containing ingests, games, jobs, game-to-job indexes, groups, and artifacts. Its backup envelope is versioned and checksummed. `BackupTo` writes a separate backup; `RestoreFrom` validates the envelope, checksum, required collections, and snapshot invariants before replacing active state. When persistence is enabled, restore atomically writes the configured live snapshot before swapping in-memory state.

Treat these as separate durability domains until an inventory proves otherwise:

- Store snapshot: ingests, games, jobs, groups, and stored artifacts.
- Social graph/follow state and run/leaderboard records: verify their configured persistence files and back them up independently.
- Uploaded source media, generated packages, object-store content, provenance, moderation/audit records, secrets, and deployment configuration: include the durable locations actually used by deployment. Do not assume they are inside the store snapshot.
- Rebuildable caches and temporary preview output: document separately; do not mistake them for authoritative data.

Before operating a deployment, record configured paths, storage volumes, encryption/access controls, backup schedule, and owner for every durable domain. Never put credentials or signed URLs in this inventory.

## Recovery objectives

Set service-specific objectives before launch. Until measured values are approved, use these only as proposed drill targets, not guarantees:

| Measure | Proposed initial target | How to measure |
| --- | --- | --- |
| Recovery point objective (RPO) | At most 24 hours | Age of newest verified off-host backup |
| Recovery time objective (RTO) | At most 4 hours | Time from restore start to invariant checks and read-only smoke tests passing |
| Integrity | Zero unexplained missing or dangling durable references | Compare counts, indexes, hashes, and audit integrity |
| Confidentiality | No public or world-readable backup artifacts | Verify storage ACLs and encryption/access policy |

A deployment owner must approve targets against actual traffic and business needs. A successful local unit test is not a staging restore drill.

## Backup procedure

1. Identify the exact deployment and maintenance window. Confirm the backup destination is not the live snapshot path or a publicly served directory.
2. Confirm persistence health is healthy. If persistence is unhealthy, stop and investigate rather than creating a misleading backup.
3. Use the application's supported backup operation (`MemoryStore.BackupTo`) to create a versioned backup. Do not copy a live JSON snapshot while it may be changing.
4. Back up every other durable domain using its own consistent backup/export mechanism. Record a common backup-set ID and timestamps so related files can be matched.
5. Store artifacts off-host with restricted access, encryption at rest and in transit, retention policy, and audit logging. Do not store credentials in the backup manifest.
6. Verify each artifact is non-empty, readable, has the expected format/version, and matches its checksum. Record byte size and checksum in a manifest.
7. Retain at least one independently stored backup and periodically test retrieval using the recovery operator's permissions.

A backup is not verified merely because the write operation returned success.

## Restore procedure

1. Declare the incident, freeze writes, and record deployment version and schema/migration level.
2. Preserve failed live data and logs as forensic evidence. Do not overwrite the only copy.
3. Retrieve a complete backup set and verify its manifest, checksums, format versions, access controls, and timestamps.
4. Restore into an isolated empty environment first. For the store snapshot, use `RestoreFrom`; it validates before changing live state and atomically writes the configured persistence target when persistence is enabled.
5. Run the integrity checks below. Investigate every discrepancy; do not silently repair unknown references.
6. Restore remaining durable domains and validate cross-domain references using the same backup-set ID.
7. Run read-only health, feed, game detail, package download, social graph, and moderation/audit smoke tests. Confirm private resources remain private and cache headers do not expose restricted content.
8. Obtain explicit recovery-owner sign-off before switching traffic. Keep original failed data and restore logs until the incident is closed.

Do not restore a store snapshot from one backup set alongside social/run/media data from a different point in time unless the resulting inconsistency has been assessed and accepted.

## Required integrity checks

For the store snapshot, validate at minimum:

- Every job references an existing game; every `jobsByGame` entry points to an existing job whose `GameID` matches the map key.
- Every artifact belongs to an existing game, and artifact names/content types are valid.
- Game, job, ingest, and group map keys match their embedded identifiers.
- Group invite indexes resolve to the intended group and do not create ambiguous invite ownership.
- Package manifests and referenced assets exist; content hashes match restored bytes.
- Social follow edges, run records, and moderation/audit records have valid account/game references and preserve uniqueness/ordering constraints.
- Restored backup format versions are supported; unknown versions fail closed rather than being guessed or partially loaded.

The store's `validateStoreSnapshot` is the authoritative current validation entry point for its in-memory snapshot. Cross-domain checks must be performed by the recovery integration/drill; do not infer that the store validator covers separate persistence domains.

## Migration preflight and recovery

Before applying a schema migration:

1. Identify source and target application versions and migration IDs.
2. Verify a recent, retrievable backup set and run a restore rehearsal on an isolated copy.
3. Check available disk space, expected migration duration, lock behavior, and whether the migration is safe to retry.
4. Run a read-only preflight checking current schema version, required columns/indexes, row counts, foreign-key integrity, and known incompatible values.
5. Record a checkpoint before each irreversible step. Make migration steps idempotent where possible and persist progress so a restart can determine which steps completed.
6. Run migration-specific postflight checks and application compatibility tests before allowing writes/traffic.

If migration fails, stop further writes, preserve logs and the pre-migration backup, and follow the migration's documented rollback or forward-recovery plan. Do not blindly rerun a partially completed non-idempotent migration. If rollback cannot safely reverse the change, restore the verified backup into a clean environment and reconcile post-backup writes according to the incident plan.

## Drill matrix

Run these in a non-production environment first, using synthetic fixtures:

| Drill | Fault injected | Expected result |
| --- | --- | --- |
| Clean restore | Restore a valid backup into an empty directory | All durable store records and artifacts match source counts and hashes |
| Corrupt envelope | Flip one byte in the backup | Checksum validation fails; active state remains unchanged |
| Unsupported format | Change format version to an unknown value | Restore rejects it; active state remains unchanged |
| Truncated backup | Truncate the backup file | Decode/checksum fails; active state remains unchanged |
| Missing required collection | Remove a required snapshot collection | Restore rejects it before replacing active state |
| Invalid references | Add a dangling job/game or index reference and recompute checksum | Snapshot validation rejects it |
| Interrupted migration | Terminate migration after a checkpoint | Restart resumes safely or documented rollback/forward recovery succeeds |
| Disk/write failure | Make restore target unavailable or unwritable | Restore reports failure and does not swap in-memory state |
| Full backup-set recovery | Restore store plus social/run/media/audit fixtures | Cross-domain references and hashes pass; read-only smoke tests pass |

For each drill record: date, environment, application/schema versions, backup-set ID, injected fault, expected and observed result, RPO/RTO measured, logs, discrepancies, operator, reviewer, and remediation issue.

## Release gate and ownership

A release that changes durable schemas or storage layout must include:

- Updated inventory and backup/restore compatibility notes.
- Migration preflight and recovery instructions.
- Automated tests for interrupted migration and restore from a known fixture.
- A successful isolated restore drill with integrity results.
- A named owner for backup monitoring, retention, restore access, and incident response.

Do not claim multi-instance readiness from this file-backed MVP store. Shared durable storage, cross-process consistency, locking, and replica coordination require a separately reviewed architecture before horizontal scaling.
