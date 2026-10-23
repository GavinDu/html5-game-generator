#!/usr/bin/env bash
# sample/test_pipeline.sh — end-to-end smoke test for the DoodleQuest backend.
#
#   ./sample/test_pipeline.sh [BASE_URL]
#
# BASE_URL defaults to http://localhost:8080. On Replit, pass the webview URL:
#   ./sample/test_pipeline.sh https://<repl>.<owner>.replit.dev
#
# Flow: ingest the sample drawing -> create game -> wait for the design doc ->
# confirm it -> wait for done -> verify bundle assets -> sharing checks.
set -euo pipefail

BASE="${1:-http://localhost:8080}"
USER="${X_USER_ID:-demo-parent}"

say()  { printf '\n==> %s\n' "$*"; }
fail() { printf '\n!! FAIL: %s\n' "$*" >&2; exit 1; }

# jget: extract a dotted path from JSON on stdin, e.g. `jget job_id`.
jget() {
  python3 -c '
import json,sys
d=json.load(sys.stdin)
for p in sys.argv[1].split("."):
    d=d[int(p)] if isinstance(d,list) else d[p]
print(d if not isinstance(d,(dict,list)) else json.dumps(d))
' "$1"
}

api() { # $1=method $2=path [$3=json body]
  if [ $# -ge 3 ]; then
    curl -sf -X "$1" "$BASE$2" \
      -H 'Content-Type: application/json' -H "X-User-ID: $USER" -d "$3"
  else
    curl -sf -X "$1" "$BASE$2" -H "X-User-ID: $USER"
  fi
}

wait_for_job() { # $1=job_id $2=want_status $3=timeout_secs
  local deadline=$((SECONDS + $3)) st=""
  while [ $SECONDS -lt $deadline ]; do
    st="$(api GET "/v1/jobs/$1" | jget status)"
    case "$st" in
      "$2") printf '%s\n' "$st"; return 0 ;;
      failed) fail "job $1 failed: $(api GET "/v1/jobs/$1" | jget error)" ;;
    esac
    sleep 1
  done
  fail "timed out waiting for job status '$2' (last: $st)"
}

say "1/9 health check"
[ "$(api GET /healthz | jget status)" = "ok" ] || fail "/healthz not ok"
echo "service is up at $BASE"

say "2/9 ingest sample drawing (dragon + castle)"
INGEST_JSON="$(api POST /v1/ingest \
  '{"images":[{"url":"sample/drawing.jpg"}],"keywords":["dragon","castle"]}')"
echo "$INGEST_JSON" | python3 -m json.tool
INGEST_ID="$(echo "$INGEST_JSON" | jget ingest_id)"
[ -n "$INGEST_ID" ] || fail "no ingest_id returned"

say "3/9 create game from ingest"
GAME_JSON="$(api POST /v1/games "{\"ingest_id\":\"$INGEST_ID\"}")"
GAME_ID="$(echo "$GAME_JSON" | jget game.game_id)"
JOB_ID="$(echo "$GAME_JSON" | jget job.job_id)"
echo "game_id=$GAME_ID job_id=$JOB_ID"

say "4/9 wait for design doc (awaiting_confirm)"
wait_for_job "$JOB_ID" awaiting_confirm 30
api GET "/v1/jobs/$JOB_ID" | jget design_doc.title | sed 's/^/design doc title: /'

say "5/9 confirm design doc"
CONFIRM_JSON="$(api POST "/v1/games/$GAME_ID/confirm" '{}')"
[ "$(echo "$CONFIRM_JSON" | jget job_id)" = "$JOB_ID" ] \
  || fail "confirm did not resume job $JOB_ID"

say "6/9 wait for pipeline to finish"
wait_for_job "$JOB_ID" done 60
[ "$(api GET "/v1/games/$GAME_ID" | jget status)" = "ready" ] || fail "game not ready"

say "7/9 verify bundle assets"
MANIFEST="$(api GET "/v1/games/$GAME_ID/bundle")"
echo "$MANIFEST" | python3 -m json.tool | head -20
JS="$(api GET "/v1/games/$GAME_ID/bundle/game.js")"
[ ${#JS} -gt 100 ] || fail "game.js too small"
echo "$JS" | grep -q "Shim\.init" && echo "game.js targets the GameShim API"
SPRITE_SRC="$(echo "$MANIFEST" | jget assets.sprites.0.src)"
if [ -n "$SPRITE_SRC" ]; then
  curl -sf -H "X-User-ID: $USER" "$BASE/v1/games/$GAME_ID/bundle/$SPRITE_SRC" -o /tmp/dq_sprite.png
  head -c 8 /tmp/dq_sprite.png | grep -q "PNG" && echo "sprite $SPRITE_SRC is a valid PNG"
fi
curl -sf -H "X-User-ID: $USER" "$BASE/v1/games/$GAME_ID/bundle/audio/theme.wav" -o /tmp/dq_theme.wav
head -c 4 /tmp/dq_theme.wav | grep -q "RIFF" && echo "theme.wav is a valid WAV"

say "7b/9 verify sample game bundle (Bobo's Rainbow)"
SMANIFEST="$(api GET "/v1/games/sample-rainbow-bear/bundle")"
echo "$SMANIFEST" | grep -q "Bobo's Rainbow" || fail "sample game missing from bundle"
for SRC in $(echo "$SMANIFEST" | python3 -c "import json,sys; m=json.load(sys.stdin); print(' '.join([s['src'] for s in m['assets']['sprites']] + [m['assets']['music']['src']]))"); do
  curl -sf -H "X-User-ID: $USER" "$BASE/v1/games/sample-rainbow-bear/bundle/$SRC" -o /tmp/dq_sample_asset || fail "sample asset $SRC not fetchable"
done
echo "sample game: all $(echo "$SMANIFEST" | python3 -c "import json,sys; m=json.load(sys.stdin); print(len(m['assets']['sprites']))") sprites + music fetchable as declared"

say "8/9 sharing: invite-only group"
GROUP_JSON="$(api POST /v1/groups '{"name":"test-family"}')"
GROUP_ID="$(echo "$GROUP_JSON" | jget group_id)"
INVITE="$(echo "$GROUP_JSON" | jget invite_code)"
echo "group_id=$GROUP_ID invite_code=$INVITE"
api POST "/v1/groups/$GROUP_ID/join" "{\"invite_code\":\"$INVITE\"}" >/dev/null
api POST "/v1/games/$GAME_ID/visibility" \
  "{\"visibility\":\"groups\",\"group_ids\":[\"$GROUP_ID\"]}" >/dev/null
api GET "/v1/groups/$GROUP_ID/games" | grep -q "$GAME_ID" \
  && echo "game visible to group members" || fail "game not listed in group"

say "9/9 sharing: public feed"
api POST "/v1/games/$GAME_ID/visibility" '{"visibility":"public","group_ids":[]}' >/dev/null
api GET /v1/feed | grep -q "$GAME_ID" \
  && echo "game visible on public feed" || fail "game not on public feed"

say "ALL GREEN — pipeline works end to end"
