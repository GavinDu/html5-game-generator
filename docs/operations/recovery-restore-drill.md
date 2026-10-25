# Recovery and restore drill

This runbook defines the minimum recovery checks for a staging deployment. It is intentionally fail-closed: do not declare a restore successful until the checks below pass.

## Recovery objectives

Set and approve service-specific recovery point objective (RPO) and recovery time objective (RTO) before production launch. Until those values are approved and measured, record them as **unverified**; do not imply that a backup schedule meets an unstated objective.

- **RPO:** maximum acceptable loss of acknowledged durable writes.
- **RTO:** maximum acceptable time to restore a usable service.
- Record the selected backup timestamp, restore start/end times, operator, application commit, schema/migration version, and validation output for each drill.

## Durable data inventory

The restore owner must inventory and verify each enabled persistence backend, rather than assuming all runtime state is in one database:

- Game records and lifecycle/visibility/moderation state.
- Game package manifests, immutable package versions, and referenced assets.
- Asset bytes and their declared content hashes.
- Social relationships and user preferences.
- Comments, saves/reactions, reports, moderation decisions, and audit records.
- Provenance/remix lineage and any idempotency records needed to make retries safe.

Classify each item as **durable**, **rebuildable**, or **not yet verified**. Caches and derived feed/ranking data must be documented separately from the authoritative records they can be rebuilt from. Do not classify a record as durable unless the configured deployment actually persists it to the backup source.

## Staging restore drill

1. Use an access-controlled staging environment and a known, non-production fixture. Never restore production user data into an unapproved environment.
2. Capture a versioned backup manifest containing the backup format version, creation time, source schema version, artifact names, byte sizes, and SHA-256 checksums.
3. Verify the manifest and every artifact checksum **before** applying any data. Abort without mutating the target if verification fails.
4. Restore into an empty target using the documented schema migration path. Capture migration output and stop on the first error; do not skip failed migrations or mark them successful.
5. Verify record counts against the manifest and fixture expectations. Check foreign references, game/package/asset references, package hashes, visibility and moderation state, social relationships, report resolution metadata, and audit continuity.
6. Exercise representative read paths after restore: fetch a public game and its assets; verify a private or quarantined game remains inaccessible to an unauthorized viewer; list and resolve a fixture moderation report; verify the first resolution metadata remains unchanged on a repeated resolution.
7. Rebuild only documented derived caches. Compare authoritative record counts and hashes again after cache rebuild.
8. Record RPO/RTO measurements, discrepancies, logs, and remediation owners. The drill passes only if integrity checks pass and the restored service behaves according to the authorization rules.

## Failure handling

- Checksum mismatch, unsupported backup version, missing artifact, unresolved reference, unexpected count, invalid package hash, or audit-integrity failure means **restore failed**.
- Keep the target isolated on failure. Preserve logs and the manifest for investigation; do not overwrite the last known-good backup.
- Prefer forward recovery for a partially applied migration unless a tested rollback procedure exists. Document the exact checkpoint and operator action before retrying.
- Restrict backup read/write permissions, encrypt backups at rest and in transit using the deployment's approved mechanisms, and periodically test that access controls work.

## Drill record template

- Date / operator:
- Application commit / schema version:
- Backup ID / format version / checksum verification:
- Source and target environments:
- Expected and restored record counts:
- Reference, package-hash, moderation, and audit checks:
- Restore duration / observed RPO / observed RTO:
- Result: pass / fail:
- Follow-up issues and owners:

This runbook does not claim that every listed data class is currently included in a backup. Verify the inventory against the deployed configuration before approving the drill.
