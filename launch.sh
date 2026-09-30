#!/bin/bash
# DoodleQuest launcher — builds the backend and serves it on your local
# network so you can open the web player from your phone's browser.
#
# Usage: ./launch.sh            (uses port 8080)
#        PORT=9000 ./launch.sh  (custom port)
set -e
cd "$(dirname "$0")"

# Locate a Go toolchain.
if ! command -v go >/dev/null 2>&1; then
  for p in /usr/local/go/bin /opt/homebrew/bin "$HOME/go/bin"; do
    if [ -x "$p/go" ]; then export PATH="$p:$PATH"; break; fi
  done
fi
command -v go >/dev/null 2>&1 || { echo "Go not found — install it from https://go.dev/dl/"; exit 1; }

PORT="${PORT:-8080}"

# LAN address for the phone URL (macOS: ipconfig, Linux: hostname -I).
IP=""
if command -v ipconfig >/dev/null 2>&1; then
  IP="$(ipconfig getifaddr en0 2>/dev/null || ipconfig getifaddr en1 2>/dev/null || true)"
fi
if [ -z "$IP" ]; then
  IP="$(hostname -I 2>/dev/null | awk '{print $1}')"
fi
[ -z "$IP" ] && IP="localhost"

echo "==> Building DoodleQuest..."
go build ./...

echo ""
echo "==> DoodleQuest is live!"
echo "    Player (open this on your phone):  http://${IP}:${PORT}/player"
echo "    On this machine:                   http://localhost:${PORT}/player"
echo ""
echo "    Your phone must be on the same Wi-Fi network."
echo "    Press Ctrl-C to stop."
echo ""

PORT="$PORT" go run ./cmd/server
