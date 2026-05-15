#!/usr/bin/env bash
# launchers/prism.sh
#
# Project-specific launcher for prism-desktop. Builds the Linux Wails
# binary inside the container (so the host never needs GTK/WebKit dev
# libs), then runs it.
#
# The launcher does NOT touch the host's ~/Library/Application Support/Prism.
# Wails app config goes to $XDG_CONFIG_HOME/Prism inside the container,
# which we explicitly point at /tmp/prism-data — this gets discarded with
# the container.
#
# Other Linux GUI projects: replace this file with your own build+run
# steps. The Dockerfile and docker-entrypoint.sh are project-agnostic.
set -euo pipefail

log() { printf '[prism %s] %s\n' "$(date +%H:%M:%S)" "$*" >&2; }

# Sandbox: redirect all data writes into /tmp so a `docker rm` wipes them.
export XDG_CONFIG_HOME=/tmp/prism-config
export XDG_DATA_HOME=/tmp/prism-data
export XDG_CACHE_HOME=/tmp/prism-cache
mkdir -p "$XDG_CONFIG_HOME" "$XDG_DATA_HOME" "$XDG_CACHE_HOME"

cd /workspace

# Build the frontend + backend if the binary is missing or stale. We rely
# on `wails build` for incremental rebuilds; it caches under build/.
BINARY=build/bin/Prism
need_build=0
if [[ ! -x "$BINARY" ]]; then
  need_build=1
elif [[ "$BINARY" -ot main.go ]] || [[ "$BINARY" -ot wails.json ]]; then
  need_build=1
fi

if [[ $need_build -eq 1 ]]; then
  log "building Prism for linux (wails v2, webkit2_41)"
  # -tags webkit2_41 selects the WebKit 4.1 binding required on Ubuntu 24.04.
  # -devtools so we can inspect Wails' WebView from within the container if
  # something goes wrong in the React layer.
  wails build -platform linux -tags webkit2_41 -devtools \
      2>&1 | sed 's/^/[wails] /'
else
  log "using cached binary $BINARY"
fi

if [[ ! -x "$BINARY" ]]; then
  log "FATAL: build did not produce $BINARY"
  ls -la build/bin/ 2>&1 || true
  exit 1
fi

# Mark a "ready" file the test harness polls before sending xdotool input.
# The Wails app boots its embedded gin server on 127.0.0.1:39527; once
# that port is listening, the UI is ready to interact with.
(
  for _ in $(seq 1 60); do
    if curl -fsS -o /dev/null http://127.0.0.1:39527/ping 2>&1; then
      log "embedded gin server is up"
      touch /tmp/prism-ready
      break
    fi
    sleep 1
  done
) &

log "launching $BINARY"
exec "$BINARY"
