#!/usr/bin/env bash
# scripts/run-e2e.sh
#
# Builds the sandbox image, starts a fresh container with the project
# repo bind-mounted at /workspace, exposes noVNC on host:6080, waits
# until Prism is ready, then prints the URL the browser MCP can open.
#
# Usage:
#   e2e-sandbox/scripts/run-e2e.sh start        # build + boot, returns URL
#   e2e-sandbox/scripts/run-e2e.sh exec ARGS    # run a command in the
#                                                 container (e.g.
#                                                 "exec xdotool key Return")
#   e2e-sandbox/scripts/run-e2e.sh logs         # docker logs -f
#   e2e-sandbox/scripts/run-e2e.sh stop         # remove container
#
# Sandbox guarantees:
#   * Repo is bind-mounted READ-WRITE only because `wails build` writes
#     into ./build/bin. Nothing under $HOME on the host is mounted.
#   * The container deletes its own $XDG_CONFIG_HOME on `docker rm`
#     because launchers/prism.sh redirects to /tmp.
#   * Port mapping is to 127.0.0.1 only; noVNC is never exposed beyond
#     the host loopback.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
IMAGE=prism-e2e-sandbox:latest
CONTAINER=prism-e2e
HOST_PORT=6080
GIN_PORT=39527

log() { printf '[run-e2e %s] %s\n' "$(date +%H:%M:%S)" "$*" >&2; }

cmd=${1:-start}

build_image() {
  log "building image $IMAGE (this is slow on the first run)"
  docker build \
    -t "$IMAGE" \
    -f "$REPO_ROOT/e2e-sandbox/Dockerfile" \
    "$REPO_ROOT/e2e-sandbox" \
    | sed 's/^/[docker build] /'
}

start_container() {
  if docker ps -a --format '{{.Names}}' | grep -qx "$CONTAINER"; then
    log "removing stale container $CONTAINER"
    docker rm -f "$CONTAINER" >/dev/null
  fi
  log "starting container $CONTAINER (noVNC on 127.0.0.1:$HOST_PORT, gin on 127.0.0.1:$GIN_PORT)"
  # Notes on volumes:
  # - The repo is bind-mounted at /workspace so the container builds the
  #   exact code on disk.
  # - frontend/node_modules and frontend/wailsjs are shadowed by NAMED
  #   docker volumes. Reason: the host (macOS arm64) installed
  #   @rollup/rollup-darwin-arm64 etc. into node_modules; in the linux/arm64
  #   container Rollup needs @rollup/rollup-linux-arm64-gnu instead. Sharing
  #   one node_modules across both OSes blows up either build. Named volumes
  #   give the container its own writable layer that survives between runs
  #   (so `pnpm install` only happens once) but never leaks back to the host.
  # - build/bin (where wails writes the linux binary) is also a named volume
  #   to keep the build cache around. Do NOT mount all of build/, because
  #   main.go embeds build/trayicon-template.png and a volume on build/ would
  #   hide that source asset from go:embed.
  docker run -d \
    --name "$CONTAINER" \
    -p "127.0.0.1:${HOST_PORT}:6080" \
    -p "127.0.0.1:${GIN_PORT}:39527" \
    -v "$REPO_ROOT:/workspace" \
    -v prism-e2e-node-modules:/workspace/frontend/node_modules \
    -v prism-e2e-wailsjs:/workspace/frontend/wailsjs \
    -v prism-e2e-build-bin:/workspace/build/bin \
    -v prism-e2e-go-cache:/root/go \
    "$IMAGE" >/dev/null
}

wait_ready() {
  log "waiting for noVNC to come up (max 120s)"
  for _ in $(seq 1 120); do
    if curl --noproxy '*' -fsS -o /dev/null "http://127.0.0.1:${HOST_PORT}/" 2>/dev/null; then
      log "noVNC ready"
      break
    fi
    sleep 1
  done

  log "waiting for Prism gin server (max 300s, includes one-time wails build)"
  for _ in $(seq 1 300); do
    if docker exec "$CONTAINER" test -f /tmp/prism-ready 2>/dev/null; then
      log "Prism is ready"
      return 0
    fi
    sleep 1
  done
  log "FATAL: Prism failed to come up; recent logs:"
  docker logs --tail 80 "$CONTAINER" >&2 || true
  return 1
}

case "$cmd" in
  start)
    build_image
    start_container
    wait_ready
    cat <<EOF
Sandbox is up:
  noVNC URL :  http://127.0.0.1:${HOST_PORT}/vnc.html?autoconnect=1&resize=remote
  gin proxy :  http://127.0.0.1:${GIN_PORT}
Drive it with:
  e2e-sandbox/scripts/run-e2e.sh exec xdotool key ctrl+l
  e2e-sandbox/scripts/run-e2e.sh exec xdotool type --delay 30 'hello'
  e2e-sandbox/scripts/run-e2e.sh exec sqlite3 /tmp/prism-data/Prism/prism.db 'select count(*) from prism_traces'
Stop with: e2e-sandbox/scripts/run-e2e.sh stop
EOF
    ;;
  exec)
    shift
    docker exec -e DISPLAY=:99 "$CONTAINER" "$@"
    ;;
  logs)
    docker logs -f "$CONTAINER"
    ;;
  stop)
    docker rm -f "$CONTAINER" 2>/dev/null || true
    log "container removed"
    ;;
  *)
    echo "usage: $0 {start|exec ARGS|logs|stop}" >&2
    exit 2
    ;;
esac
