# Moderation review workflow

This workflow is for authorized operators reviewing user-generated games and reports. It complements the API contract in [MODERATION.md](MODERATION.md); it does not replace identity verification or trusted deployment/network controls.

## Before enabling public user-generated content

- [ ] Set a high-entropy `DQ_MODERATION_TOKEN` through deployment secret management. Never put it in the client, source control, logs, or issue reports.
- [ ] Restrict moderation routes at the deployment/network layer. The token is an additional control, not a substitute for authenticated operator identity.
- [ ] Confirm the configured audit-file path is persistent, access-controlled, backed up, and monitored for write failures.
- [ ] Assign an on-call reviewer and a backup reviewer; document escalation and response targets.
- [ ] Provide a user-facing reporting path and a contact/appeal path before broad launch.
- [ ] Confirm a rollback/disable switch exists for new publishing if review capacity is exceeded.

## Review a report

1. Read the report reason and target game metadata. Treat all captions, filenames, external URLs, and media as untrusted input; do not open unknown links on a privileged workstation.
2. Retrieve the target through the normal moderation-safe interface. Do not rely solely on a user-supplied screenshot or external link.
3. Check the media and caption against the published content policy. Record a concise, factual reason tied to the relevant policy category; avoid copying unnecessary personal information into the audit reason.
4. Choose the narrowest action that addresses the risk:
   - **Approve:** the content is allowed and the report is not substantiated.
   - **Reject:** the content violates policy and should not be published.
   - **Quarantine:** the content needs further review or has an unresolved safety concern.
5. Resolve the report with a reason, then verify the game visibility and audit record through the API.
6. For severe threats, suspected exploitation, or imminent harm, preserve the minimum necessary evidence and escalate through the documented incident channel. Do not investigate beyond the operator's authorization.

## Queue hygiene and escalation

- Review urgent safety reports first, then oldest unresolved reports.
- Keep reports open when evidence is incomplete; do not resolve merely to clear queue volume.
- Use a second reviewer for borderline cases, repeat violations, appeals, or decisions that could materially affect a creator.
- Track queue age, unresolved count, time-to-first-review, action counts, appeal reversals, and audit-write failures. Avoid exposing user identifiers in aggregate dashboards.
- Review a sample of decisions regularly for consistency and over-removal. Record policy changes and reviewer training.

## Audit and incident checks

- Verify each decision has an audit record before considering the workflow complete. Audit write failure should fail closed.
- Restrict read/write access to audit files and include them in backup/restore drills.
- Alert on repeated authorization failures, elevated quarantine/rejection rates, and audit write failures. Never log the moderation token.
- The current JSONL audit file is append-only in application code but is not tamper-evident. For production, forward records to an access-controlled centralized log or immutable storage and test restoration.
- The current actor ID comes from `X-User-ID` and is not verified identity. Until authenticated identity is implemented, do not treat it as proof of who performed an action and do not expose moderation operations publicly without trusted network restrictions.

## Known gaps — launch blockers for broad public UGC

- Automated semantic review of captions and media is not provided by this workflow.
- The API is not a staffed review console; queue triage, reviewer assignment, and appeals need an operational owner or product implementation.
- Identity and role-based authorization for operators remain prerequisites before public exposure.
- Define retention, privacy, escalation, and appeal policy before collecting more review data.

This document is an operational checklist, not evidence that a staffed workflow, media classifier, immutable audit backend, or appeal system already exists.
