# DoodleQuest backend

Go backend for the AI game-generation platform: kids' drawings in,
playable games out. Implements the API and async pipeline from
`architecture-and-execution-plan.md`. Standard library only — no
dependencies, so it builds anywhere (including Replit) with `go build`.

## Layout

- `cmd/server` — entrypoint (`PORT` env, default 8080)
- `internal/model` — domain types (games, jobs, stages, groups, manifests)
- `internal/store` — `Store` interface + in-memory implementation (swap for Postgres later)
- `internal/pipeline` — async stage machine: spec → (user confirm) → art → codegen → validate → publish
- `internal/media` — stub asset generators (sprite PNGs, music WAV loops)
- `internal/shim` — stub codegen: emits a valid GameShim v1 game
- `internal/api` — HTTP handlers + routes

## Pipeline

Every stage records latency, tokens, and cost (`StageRecord`) from day one —
currently all `provider: "stub"` with zero cost. Swap order for production:

| Stage | Stub today | Real implementation |
|---|---|---|
| ingest (sync) | keywords → elements | vision model reads the drawing |
| spec | template design doc | SPEC LLM (structured output) |
| art | painted PNGs + synth WAV | style-preserving image edit; licensed music library |
| codegen | template game.js | codegen LLM vs GameShim v1 |
| validate | static checks | Playwright headless playtest + vision judge |
| publish | in-memory manifest | object storage + CDN |

## API

| Method & path | Description |
|---|---|
| `GET /healthz` | liveness |
| `POST /v1/ingest` | `{images[], keywords[]}` → elements + guessed intent |
| `POST /v1/games` | `{ingest_id\|elements[], intent}` → game + job (202) |
| `GET /v1/games/{id}` | game status |
| `GET /v1/jobs/{id}` | job stages; `design_doc` appears at `awaiting_confirm` |
| `POST /v1/games/{id}/confirm` | `{design_doc?}` → resumes art→codegen→validate→publish |
| `GET /v1/games/{id}/bundle` | published manifest |
| `GET /v1/games/{id}/bundle/game.js` | generated game code |
| `GET /v1/games/{id}/bundle/sprites/{sprite}.png` | sprite assets |
| `GET /v1/games/{id}/bundle/audio/theme.wav` | music loop |
| `POST /v1/groups` | create invite-based group |
| `POST /v1/groups/{id}/join` | `{invite_code}` |
| `GET /v1/groups/{id}/games` | games shared with the group (members only) |
| `POST /v1/games/{id}/visibility` | `{visibility: private\|groups\|public, group_ids[]}` — author only |
| `GET /v1/feed` | public, ready games |

Auth is currently `X-User-ID` (defaults to `demo-parent`); Sign in with
Apple + proper sessions come with the iPhone app.

## Run

```sh
go build ./...
go test ./...
PORT=8080 go run ./cmd/server
```

## Run on Replit

Plain Repl, no Agent mode:

1. Create → Import from GitHub → paste this repo URL (or `GavinDu/html5-game-generator`)
2. In the Shell: `go build ./...`
3. Press Run (or `go run .`) — Replit sets `PORT` for you
4. Open the Webview URL and check `/healthz` → `{"status":"ok"}`

## Sample

`sample/test_pipeline.sh` is an end-to-end smoke test against a running
server: ingest → create game → confirm the design doc → verify bundle assets
(`game.js`, sprite PNG, music WAV) → group sharing and public feed checks.
It ingests with keywords `dragon, castle` (matching the sample kid-style
crayon drawing of a dragon and a castle — drop `drawing.jpg` into `sample/`
via the GitHub web UI if you want the picture alongside the script).

```sh
./sample/test_pipeline.sh http://localhost:8080
# on Replit: ./sample/test_pipeline.sh https://<repl>.<owner>.replit.dev
```

## Roadmap

- Wire real vision/SPEC/codegen behind `pipeline` stage functions via env keys
- Postgres store implementation
- Playwright validation worker
- APNs on job completion, StoreKit entitlement checks
