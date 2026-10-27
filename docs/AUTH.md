# Authentication and account deletion

Hephaestus #10: the backend never trusts a caller-supplied identity.
Production identity comes from Sign in with Apple, verified server-side.

## Sign-in flow

1. The iOS app runs the system Sign in with Apple sheet
   (`ASAuthorizationAppleIDProvider`, no scopes — identity only, no name
   or email is requested or stored).
2. The app sends the Apple identity token (RS256 JWT) to
   `POST /v1/auth/apple` with `{"identity_token": "<jwt>"}`.
3. The backend verifies the token against Apple's published keys
   (`https://appleid.apple.com/auth/keys`, overridable with
   `DQ_APPLE_JWKS_URL` for tests):
   - signature over RS256 with the key matching `kid`,
   - `iss` is `https://appleid.apple.com`,
   - `aud` matches `DQ_APPLE_AUDIENCE` (default `ai.tinydream.Hephaestus`,
     the iOS bundle ID),
   - `exp` is in the future (5 minutes of clock skew tolerated),
   - `sub` is 1–128 characters.
4. On success the backend mints its own session token and returns
   `{"access_token", "token_type": "Bearer", "expires_in": 2592000,
   "subject": "apple:<apple-sub>"}`. The subject is namespaced by
   provider so a future second provider cannot collide with Apple
   subjects. The app stores the session token in Keychain and sends it
   as `Authorization: Bearer <token>`.

The session token format is unchanged from before: base64url JSON
claims (`sub`, `exp`, `iss: "hephaestus"`) plus HMAC-SHA256 over the
payload with `DQ_AUTH_HMAC_SECRET`. The middleware
(`requireAuthenticatedUser`) enforces it: public discovery/content
reads stay anonymous, every other `/v1/` route requires a valid,
unexpired, non-tombstoned bearer token, and fails closed (503) when
no signing secret is configured.

## Session expiry and revocation

- Sessions last **30 days** (`sessionTTL`) and are **not refreshed**:
  when one expires the user signs in again with Apple. There is no
  refresh-token endpoint and no server-side session table.
- Revocation has three layers, and their limits are stated plainly:
  - **Account deletion** tombstones the subject (see below): all
    outstanding tokens for that subject stop working immediately, and
    the subject cannot sign in again.
  - **Secret rotation**: rotating `DQ_AUTH_HMAC_SECRET` invalidates
    every outstanding session at once. This is the break-glass control.
  - **Apple-side revoke** (Settings > Apple ID > Sign in with Apple)
    stops future issuance for that Apple account. It does **not**
    invalidate outstanding backend sessions before their 30-day
    expiry, because the backend cannot observe Apple's revocation
    without a refresh-token exchange it deliberately does not perform.
    The 30-day cap bounds that window.

## Account deletion

`DELETE /v1/users/me` (authenticated) deletes the account:

- Purges the caller's social data: follows (both directions), blocks
  (both directions), saves, reactions, comments, not-interested marks,
  muted topics, creator profile, comment idempotency records, and
  moderation reports filed by the caller. In Postgres mode the same
  purge runs transactionally (`socialgraph.DeleteUser`) and the subject
  is tombstoned (`deleted_subjects`); in single-instance mode the
  tombstone persists in the social-state snapshot.
- After deletion the subject's bearer tokens are rejected (401 on
  protected routes, no identity on public reads) and re-issuance for
  the same Apple identity is refused (403). Deletion is final.
- **Published games are not deleted.** They are public content
  addressed by an opaque identifier that carries no personal data
  (no name, email, or avatar is ever stored), and deleting them would
  break other users' saves and feeds. The in-app delete flow says so.

The iOS app exposes deletion in Settings next to Sign out: a
confirmation alert, then `DELETE /v1/users/me`, then the local session
is cleared. Users can additionally revoke in Apple ID settings.

## Developer and test identity

- `DQ_ALLOW_INSECURE_USER_ID=true` re-enables caller-controlled
  `X-User-ID` (defaulting to `demo-parent`). It is explicitly
  opt-in, for local development only, and must never be set in
  production. With it unset and no signing secret, protected routes
  answer 503 rather than trusting anyone.
- Tests use `authTokenForTest`, which signs with the same code path
  as production issuance (`mintSessionToken`). It is not an HTTP
  endpoint and cannot mint a production session.
- Operator actions (moderation, telemetry admin) keep their own
  token model (`DQ_MODERATION_TOKEN`, `DQ_TELEMETRY_ADMIN_TOKEN`)
  and are unaffected by user sessions.
