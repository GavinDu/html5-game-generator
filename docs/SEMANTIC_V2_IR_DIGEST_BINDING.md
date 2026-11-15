# Epic #202 / canonical semantic v2 IR digest binding (atomic #434)

## Current code-level identity boundary

The trusted HTTP validator `POST /v1/games/{id}/semantic-input-v2/validate` fetches a currently deliverable published game and computes `irSHA256Of(game.GameIR)` from canonical Go `json.Marshal` bytes. However the underlying reusable `replaytrace.CanonicalSHA256(ir, gameID, fullIRSHA256, seed, trace)` only checked that `fullIRSHA256` was 64 lowercase hex characters; a future caller could accidentally bind valid semantic action events to an unrelated purported full IR digest. This changed primitive now explicitly recomputes SHA-256 of the provided `ir` document and rejects any caller-supplied mismatch *before* hashing evidence.

The record `gameID` comes from server authorization and delivery metadata, while `ir.Game.ID` is authored inside the Game IR. They must not be conflated or equality asserted as a shortcut for package authorization. `CanonicalSHA256` is data-only and still cannot tell whether the record belongs to a particular authorized user, whether the seed was server-owned, or whether the browser truly executed the declared inputs. Those properties require the scoped `deliverableGame` HTTP gate, immutable published package/seed provenance and independent server-side replay attestor.

## Fixed invariants

- The claimed full `irSha256` must equal `sha256(json.Marshal(ir))` in lowercase hex, never a short 12-hex leaderboard key.
- A different full hash sharing the same 12-character prefix **fails closed**; changing the IR after computing its hash **fails closed**; a correctly recomputed changed IR has a new evidence fingerprint.
- The schema-v2 tick/event/action/phase/ordinal constraints, zero-event nil/empty identity, record-ID domain separation, seed dependence and deterministic replay envelope remain unchanged.
- No changes to live iOS WKWebView, Playability Kit, Pixi physics, run persistence, score verdict or authoritative ranking.

## Senior game-engine architect / security challenger

This strengthens one primitive's **IR-byte identity**, but does not authenticate the caller-supplied record ID, user or seed, verify total runtime execution, or protect preinstalled JS-generated score against browser self-report. Never promote the returned digest to `verified`. Future trusted replay #382/#429 must bind actual published Game IR through the authorized store and independently execute the complete capability-scoped semantics. Only native physical iPhone/iPad measurements qualify for compositor-frame and thermal acceptance (#276/#279).

**Acceptance gate:** Six exact-head Go/race/static, PostgreSQL, Chromium, WebKit, cross-browser determinism and Playability Kit jobs SUCCESS; senior source challenge on PR; only child #434 may close. Parents #381/#382 and native Epic #202 remain open.
