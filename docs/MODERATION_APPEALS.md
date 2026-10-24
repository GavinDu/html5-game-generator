# Moderation appeals and contact

A creator can appeal a rejected or quarantined game with `POST /v1/games/{id}/moderation-appeals` and JSON `{"reason":"..."}`. The request must come from the game's current creator ID and the reason must contain 1–1000 characters. One open appeal per creator/game is allowed. The appeal enters the existing moderator report queue and appends an audit event; it does not automatically restore publication. Moderators use the existing moderation/report endpoints to review and resolve it.

`GET /v1/moderation/contact` exposes the configured support channel. Set `DQ_MODERATION_CONTACT_URL` and/or `DQ_MODERATION_CONTACT_EMAIL`; if neither is set, the endpoint reports that no external channel is configured.

The current `X-User-ID` actor identifier is not verified authentication. This workflow is suitable for development/testing, not a public launch with untrusted clients. Authentication and shared durable social storage remain deferred.
