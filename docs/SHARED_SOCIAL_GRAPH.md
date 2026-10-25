# Shared Social Graph Storage

Creator follows, user block relations, and saved-game edges use PostgreSQL for shared, durable reads and writes across backend replicas.

## Configuration

- Set `DQ_SOCIAL_DATABASE_URL` to a PostgreSQL DSN supplied by the deployment secret manager.
- The service creates the `social_follows`, `social_blocks`, and `social_saves` tables and their indexes on startup. The database role should be scoped to this application database and should not be a cluster administrator.
- `/readyz` reports HTTP 503 when a configured shared graph cannot connect or migrate. Do not route traffic to an instance that cannot reach the configured graph.
- When `DQ_SOCIAL_DATABASE_URL` is unset, the service keeps the legacy local snapshot behavior. That mode remains single-instance only.

## Schema and consistency

Follow, block, and saved-game relations use composite primary keys and bounded identifier lengths. Follow/block relations reject self-relations. Inserts are idempotent; deletes are safe to retry. Following/friends feeds read follows from PostgreSQL, feed filtering reads blocks from PostgreSQL, and saved-game endpoints read/write PostgreSQL when configured, so replicas share these behaviors.

This slice makes **follows, blocks, and saved games** shared. Mutes, not-interested preferences, comments, reports, reactions, and moderation audit records still use their existing persistence paths and are not made multi-replica-safe by this change.

## Import existing local follow, block, and save relations

Back up the source snapshot first. Run the migration command against the destination database before enabling the database URL on the service:

```sh
DQ_SOCIAL_DATABASE_URL='postgres://…' \
DQ_SOCIAL_STATE_FILE='telemetry/social-state.json' \
go run ./cmd/migrate-social-graph
```

The command imports follow edges, enabled block relations, and enabled saved-game relations in separate transactions. Imports are idempotent, add missing rows, never delete destination rows, and print only aggregate counts (`source_edges`, `source_blocks`, `source_saves`). Re-running it after interruption is safe. Keep the original snapshot unchanged until counts have been reviewed and the deployment has passed smoke tests.

## Rollout checklist

1. Provision PostgreSQL with TLS and a least-privilege application role.
2. Take and verify a backup of the current social-state snapshot.
3. Run the import command and compare `source_edges`, `inserted_edges`, and `already_present`.
4. Deploy to one staging instance with `DQ_SOCIAL_DATABASE_URL` configured.
5. Verify follow/unfollow, block/unblock, saved/unsaved games, saved-game lists, following list, following feed, mutual-friends feed, and that a blocked creator's games are excluded; restart the instance and repeat.
6. Run two staging replicas against the same database and verify changes made through one replica are visible through the other.
7. Only then enable the configuration in production. Keep the original snapshot for rollback evidence; do not switch back to stale local state after new writes occur in PostgreSQL.

This does not migrate the rest of the social state or make game/package artifacts shared. Object storage and transactional migration for those resources remain separate work.
