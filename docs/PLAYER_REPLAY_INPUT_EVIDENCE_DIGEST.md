# Server-derived replay input evidence digest — Epic #202 / security #360

## Source-confirmed gap

The existing browser uploads a bounded ordered seed/input trace with an
untrusted score and win claim. Previous backend hardening established
CSPRNG run IDs, fail-closed durable JSONL append+sync, full **server-derived**
canonical IR SHA-256 and validation of replay tick ordering and the v1
input grammar. However a stored run had no separate versioned identifier
for the exact admitted input sequence.

## New narrow server control

`canonicalRunInputSHA256` in `internal/api/runs.go` uses Go's
deterministic JSON serialization of **explicit struct fields**, not
a browser-supplied string or unordered map:

- `schema: replay-input-evidence-v1`;
- server-known `gameId` and full current `irSha256`;
- accepted `seed` (uint32) and untrusted declared `durationMs`;
- validated **ordered** `inputs` including each integer tick, action
  type and detail; two events on the same fixed tick **retain arrival
  order**, because key press/release or left/right collisions need it.

Hash the exact encoded byte sequence using SHA-256 and persist the
64-hex `replayInputSha256` for *new* accepted records under the
existing durable write gate. Nil and empty input slices normalize to
the same empty array. An invalid/missing full IR fingerprint fails
before allocating an acknowledged run ID. Authorized `GET /v1/runs/{id}`
includes the optional digest for new runs; historical JSONL records
remain readable with **no invented hash**. No new browser-native command,
client-provided verification endpoint, or trusted scoreboard flag exists.

Go regression tests show identical evidence yields the same digest
across record reload, while changing game/version/seed/duration,
input count, input detail or **same-tick order** changes it. The exact
same input trace submitted with a different client score, name or
win assertion has the same evidence digest but **both remain unverified**.

## Senior game-engine architect challenger: important limits

This is an **integrity fingerprint of what the backend accepted**, not
a signature, independent execution proof or attestation. A malicious
browser can still manufacture an entirely false but well-formed
seed/input trace. The backend cannot derive actual score/win/elapsed
simulation ticks from a hash. The declared duration is untrusted, and
the digest cannot prove actual client playback or prevent a replay
submitted under another account. It also binds to the Game IR
available **at submission time**, not an immutable launch-time
IR+assets+engine manifest; prior #368 alone cannot prove launch
identity. The current JSON envelope is a versioned backend encoding,
not a complete client/server canonical cross-language protocol.

Before #360 can close, implement immutable launch-version snapshot,
authorization/ownership/revocation epoch, typed per-tick action
grammar and bounded chunk hash chain, independent server-side
fixed-step semantic execution, and a server-owned transactional
`pending -> verified | rejected` verdict. Add Go/JS/Swift
cross-device negative tests and audit historical browser-self-verified
records. Separate physical iPhone/iPad GPU/memory/thermal acceptance
remains open under Hephaestus Epic #202.

**Release gate:** all six exact-head Go validation jobs (Go, PostgreSQL,
Chromium, WebKit, Playability Kit, cross-browser determinism) must
complete successfully, followed by an adversarial engine architect
review of the final source and expected SHA lease. No local E2E and
no CI bypass.
