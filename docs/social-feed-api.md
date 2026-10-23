# Social feed API MVP

## Feed scopes

`GET /v1/feed` remains backward-compatible and returns the public, ready, non-quarantined catalog.

- `GET /v1/feed?scope=for_you`: public catalog in the existing newest-first order.
- `GET /v1/feed?scope=following`: public, ready, non-quarantined games from creators the current user follows.
- `GET /v1/feed?scope=friends`: same eligibility rules, restricted to mutual follows.

Unknown scopes return HTTP 400. A feed with no matches returns `{"games":[]}`.

## Relationships

- `POST /v1/users/{creatorID}/follow`: follow a creator with at least one eligible public game.
- `DELETE /v1/users/{creatorID}/follow`: unfollow a creator.
- `GET /v1/users/me/following`: list followed creator IDs for the current demo identity.
- Self-follow and unknown creator IDs are rejected.

## MVP limitations and security gate

This first slice stores follow edges in process memory, so relationships are lost on server restart and are not safe across multiple replicas. The existing `X-User-ID` header is a demo identity, not authentication; callers can spoof another user. Do not expose follow or friends functionality as a production security boundary until authenticated identity and durable shared persistence are implemented. No friend recommendations are generated yet; Friends is defined as mutual follows. The public For You feed is still newest-first, not a personalized ranker.
