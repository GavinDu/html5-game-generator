# Server-owned full IR fingerprint for new replay traces — Epic #202 / #360

## Observed gap

Run records and leaderboard currently key on a 12-hex truncated
`irHash`. Although this is adequate for a compact board partition key,
it is **not** enough information for a future independent attestor to
identify exact canonical Game IR with 256-bit collision resistance.
The backend already implements `irSHA256Of(*gameir.Document)` over
`json.Marshal` of the current IR, so the full digest can be recorded at
the run admission boundary without relying on a client-provided field.

## Narrow server implementation

- Reuse the existing authoritative loaded `GameIR` (not client JSON) to
  compute a full 64-character SHA-256 hex digest **before** assigning
  the short `irHash`. Reject an unavailable digest instead of admitting
  an unidentified game version.
- Persist `irSha256` into each **new** `RunRecord` alongside its
  backward-compatible 12-character `irHash`, under the existing
  durable append + sync gate introduced in PR #367.
- An authorized `GET /v1/runs/{id}` returns the digest with other replay
  metadata. No privileged endpoint, token, game execution, scoring, or
  native bridge behavior is modified.
- Historical records without `irSha256` remain readable and their full
  digest stays **unknown**; never fabricate one from the short hash.
- Go HTTP tests submit an accepted current game run, verify the full
  server-owned digest and 12-character prefix, reload a fresh Server
  from disk to assert persistence, confirm no false verified verdict, and
  establish explicit compatibility with legacy JSON.

## Senior game-engine architect challenger / limitations

This is a **provenance prerequisite**, not an immutable package or
game-result attestor. It binds the stored trace to a canonical IR digest
computed *at submission time*, not a historical snapshot captured when the
game was delivered. A game can be republished between play and run upload:
the existing short hash check rejects stale builds under ordinary
circumstances, but neither a 12-char match nor the new digest proves what
the client actually executed. A complete attestor must pin immutable IR
and **all content-addressed asset/runtime versions** at launch, include
the full digest in a versioned run upload contract, reject any mismatch,
authorize moderated/private versions, deterministically replay every
input against a server-owned semantic interpreter, compare score/win,
and commit a server-owned verdict. Legacy unproven runs must remain
unverified, with an explicit migration/audit decision.

The `json.Marshal` digest is the existing backend's canonical *IR
serialization* digest; it does not include referenced image/audio bytes
or independently identify Pixi/Rapier runtime bits. Do not describe it
as a full immutable package digest.

**Release gates:** Go/unit, PostgreSQL, Chromium, WebKit, Playability Kit,
cross-browser determinism on exact PR head; senior source/code challenge;
no new `Verified` flag or executable client code. Keep generator #360
and Hephaestus Epic #202 OPEN until independent replay attestation and
physical iPhone/iPad evidence exist.
