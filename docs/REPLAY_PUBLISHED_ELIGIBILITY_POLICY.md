# Private pure publication eligibility policy — Epic #202 / #463

Parent [#460](https://github.com/GavinDu/html5-game-generator/issues/460) / issuer [#457](https://github.com/GavinDu/html5-game-generator/issues/457).

## Scope

The unexported deterministic \`evaluatePublishedAttemptEligibility\` in \`internal/attestation\` only evaluates already-server-resolved facts. It does **not** authenticate a caller, read the published catalog, authorize HTTP, issue an attempt, mint random seeds, modify a score or grant the \`Verified\` status. These structs can be fabricated by a Go caller; **they are not authentication credentials or attestation evidence**. The package remains unwired to public endpoints.

When the server supplies a current authenticated subject and authoritative immutable published package, the policy requires complete 64-lowercase-hex IR and capability digests, exact immutable revision and Game IR selector, approved moderation and capabilities, active/not-removed publication, matching auth-epoch digest, and explicit public or exact-owner-private scope. Unknown group/friend/collaborator visibility and anonymous trusted issuance fail closed. Every denial uses a bounded private reason code, not a user-facing resource-enumeration response.

## Tests and future trust integration

Pure Go tests cover public/owner-private positives, denied guest/foreign owner/unknown scopes, malformed identifiers/epochs/digests, changed full hash with identical prefix, stale revision, blocked/deleted/mutable/draft package, unsupported capability, token epoch rotation and 128/129 ASCII selector boundary. CI must show all six exact-head Go/race/PostgreSQL/Chromium/WebKit/determinism/Playability Kit jobs successful before this *pure* child is marked complete.

Important remaining security work:

1. Parent #460: genuine middleware-derived server subject and authoritative persisted package lookup, publication/moderation and per-game entitlement; real authenticated HTTP negatives and no browser-supplied context.
2. Child #461: durable server CSPRNG seed/run/nonce, microsecond issue/expiry and auth-epoch grant with DB uniqueness; no reuse of legacy \`RunRecord.Verified\`.
3. Child #462: transactional replay worker lock, credential rotation, moderation revocation and expiry CAS; do not make this snapshot-only policy a time-of-check/time-of-use authority.
4. Full game replay #382 and authoritative verified rank #402 are separate. A status string or client result is never a verified score.

Do not claim iPhone/iPad GPU first frame, 50-cycle thermal/memory, App Store 4.7, real deployed Game IR, or external security review from these unit tests. Rollback removes these private files and leaves all historical provisional results untouched.
