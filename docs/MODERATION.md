# Moderation operations

## Operator controls

Set `DQ_MODERATION_TOKEN` to a high-entropy secret in the deployment environment. Do not commit the token or place it in client code. The current operator endpoints require the `X-Moderation-Token` header and fail closed when the token is not configured:

- `GET /v1/moderation/reports` — report queue.
- `POST /v1/moderation/reports/{id}/resolve?reason=...` — resolve a report.
- `POST /v1/games/{id}/moderation` — approve, reject, or quarantine a game.
- `GET /v1/moderation/audit` — recent append-only audit actions, newest first; optional `game_id` and `limit` (1–500).

The audit log is stored as JSONL at `DQ_MODERATION_AUDIT_FILE`, or `telemetry/moderation-audit.jsonl` by default. It records report creation, game moderation status changes, and report resolution, including the claimed actor ID, reason, and UTC timestamp. Audit records are appended before a moderation mutation; if the audit write fails, the mutation is rejected with HTTP 503. Files are created with owner-only permissions where supported.

## Important limitations

The actor ID currently comes from `X-User-ID` and is therefore a development claim, not verified identity. This is not sufficient for a public moderation operation until backlog #10 (authenticated identity) is implemented. Keep operator endpoints behind trusted deployment/network controls as well as the token.

The JSONL audit file is append-only in application code but does not yet have automated rotation, retention, or tamper-evident signing. Restrict filesystem access and back it up with the deployment's operational controls. The current moderation workflow is an API, not a staffed review console; automated semantic review of captions/media and an appeal/contact path remain outstanding before broad public UGC launch.
