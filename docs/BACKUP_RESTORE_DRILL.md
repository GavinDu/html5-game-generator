# Backup and Restore Drill

This runbook covers the current **single-instance, file-backed** deployment. It is a validation procedure, not a claim that the service already has automated backups or shared durable storage.

## Scope and prerequisites

- Identify the deployed service's persistent-volume mount and the exact paths used by the process.
- Confirm the current values of `INVENTORY_DIR`, `DQ_SOCIAL_STATE_FILE`, `DQ_SOCIAL_EVENTS_FILE`, `DQ_MODERATION_AUDIT_FILE`, and `DQ_TELEMETRY_DIR` (where configured). Do not print secrets or copy environment files into the backup.
- Confirm the platform's documented volume snapshot/export mechanism and retention policy.
- Use a staging instance for the first restore. Never overwrite production data during a drill.
- Ensure the operator can access `/healthz` and `/readyz` and the moderation endpoints with an authorized token.

## Data inventory

Before a backup, record which durable locations exist and which service owns them:

| Data | Source to inventory | Restore verification |
| --- | --- | --- |
| Game inventory and package assets | Configured inventory and asset storage | Known public game loads; private/removed game remains unavailable as expected |
| Social state | `DQ_SOCIAL_STATE_FILE` or default `telemetry/social-state.json` | Saved game, comment, reaction, follow/block preference, and report state survive restart |
| Social event log | `DQ_SOCIAL_EVENTS_FILE` or default `telemetry/social-events.jsonl` | Recent events parse and counts remain bounded |
| Moderation audit | `DQ_MODERATION_AUDIT_FILE` or configured moderation audit path | Existing audit records remain readable and append-only |
| Runtime telemetry | `DQ_TELEMETRY_DIR` or configured telemetry directory | Recent session events and aggregate counters are available |

Paths above describe current configuration conventions; verify the deployed values rather than assuming defaults.

## Backup drill

1. Choose a maintenance window and record the deployment version, UTC timestamp, configured storage paths, and current `/healthz` and `/readyz` responses.
2. Create a consistent snapshot or export using the hosting provider's supported mechanism. If a consistent live snapshot is not supported, stop writes or use a documented application-level quiescence procedure first.
3. Copy the snapshot to a separate, access-controlled backup location. Encrypt it at rest and restrict access to operators who need it.
4. Record the snapshot identifier, size, checksum (for exported files), and retention expiry. Do not include user identifiers, tokens, or raw event payloads in the drill report.
5. Verify that the backup can be read and that required files are present and non-empty where expected. Do not treat successful file listing as proof of recoverability.

## Restore drill

1. Provision an isolated staging instance on the same application version and compatible configuration.
2. Restore the snapshot to a new volume or isolated directory. Never restore over the live production volume.
3. Validate checksums for exported files before starting the application.
4. Start the application and verify `/healthz` and `/readyz`. A healthy process with an unready persistence layer is a failed restore.
5. Run read-only smoke tests:
   - Load a known ready/public game and its package.
   - Confirm a removed, private, quarantined, or moderation-pending game is not exposed publicly.
   - Confirm saved games, comments, reactions, follows, blocks, and moderation reports are present where included in the snapshot.
   - Confirm moderation audit records remain readable and the audit endpoint remains protected.
   - Submit a synthetic gameplay event in staging and confirm it is accepted and appears in the expected telemetry store.
6. Restart staging and repeat the key checks to ensure restored state is not only held in memory.
7. Record recovery point (data timestamp), elapsed restore time, missing data, errors, and the operator who performed the drill. Destroy the staging copy securely after review.

## Failure handling

- If `/readyz` reports persistence failure, do not route production traffic to the instance.
- Preserve corrupt or unreadable source files for diagnosis; do not overwrite the only copy with a newly initialized empty state.
- If any smoke test fails, mark the drill failed, preserve logs without secrets, and investigate before claiming the backup is usable.
- Keep the last known-good backup unchanged until a newer backup has passed a restore drill.

## Current scaling limitation and migration path

The current file-backed state and append-only local logs are suitable only for a single instance with a reliable persistent volume and a documented write model. A readiness endpoint does **not** make local files shared or coordinate writes between replicas.

Before introducing multiple backend instances:

1. Select a shared durable database for mutable social state, reports, moderation audit metadata, idempotency records, and delivery/telemetry aggregates as appropriate.
2. Select durable object storage for game packages and media, with explicit immutable version keys and lifecycle/deletion rules.
3. Define transactional boundaries, unique constraints/idempotency, migration checkpoints, and compatibility tests.
4. Build an idempotent one-way migration tool that can resume from checkpoints and report counts/checksums without logging sensitive payloads.
5. Dual-read or shadow-validate in staging; compare counts and sampled records before cutover.
6. Define a rollback boundary. After writes begin in the new store, do not silently switch back to stale file state; reconcile or replay writes first.
7. Load-test concurrent writes, restart behavior, and cache invalidation before enabling more than one instance.

## Drill report template

- Date/time (UTC):
- Environment and app commit:
- Backup snapshot/export ID:
- Data paths included:
- Checksum verification:
- `/healthz`:
- `/readyz`:
- Game/package smoke test:
- Social state after restart:
- Moderation audit verification:
- Telemetry event verification:
- Recovery point / data loss window:
- Restore duration:
- Result: PASS / FAIL
- Follow-up issue(s):
