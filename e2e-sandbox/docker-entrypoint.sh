#!/usr/bin/env bash
# docker-entrypoint.sh
#
# Boots the headless GUI stack inside the container, then hands off to
# whatever launcher CMD was passed in. Order matters:
#
#   1. Xvfb on $DISPLAY ($XVFB_RES virtual screen).
#   2. fluxbox as a minimal window manager so app windows get focus +
#      decoration. Without a WM, `xdotool key` events sometimes target the
#      wrong window after the app spawns child windows.
#   3. dbus session — WebKit2GTK aborts on startup without it.
#   4. x11vnc bound to :99 with no password; we never expose this beyond
#      the container's docker network unless the runner publishes -p 6080.
#   5. websockify (noVNC bundle) on 0.0.0.0:6080 -> 127.0.0.1:5900 so a
#      browser MCP on the host can see the desktop.
#   6. exec the launcher.
#
# Each step logs to stdout for `docker logs`. The script never daemonises
# the launcher; PID 1 is the launcher so `docker stop` reaches the app.

set -euo pipefail

log() { printf '[entrypoint %s] %s\n' "$(date +%H:%M:%S)" "$*" >&2; }

cleanup() {
  log "shutting down background services"
  jobs -p | xargs -r kill 2>/dev/null || true
}
trap cleanup EXIT

log "starting Xvfb on $DISPLAY ($XVFB_RES)"
Xvfb "$DISPLAY" -screen 0 "$XVFB_RES" -ac +extension RANDR &
XVFB_PID=$!

# Xvfb takes a short moment to listen on the X socket. Polling is faster
# than a fixed sleep and avoids spurious "can't open display" errors when
# fluxbox starts before X is ready.
for _ in $(seq 1 50); do
  if xdpyinfo -display "$DISPLAY" >/dev/null 2>&1; then break; fi
  sleep 0.1
done

log "starting fluxbox"
fluxbox >/tmp/fluxbox.log 2>&1 &

log "starting dbus session"
eval "$(dbus-launch --sh-syntax)"
export DBUS_SESSION_BUS_ADDRESS DBUS_SESSION_BUS_PID

log "starting x11vnc (display=$DISPLAY)"
x11vnc -display "$DISPLAY" -nopw -forever -shared -rfbport 5900 \
    -quiet -bg -o /tmp/x11vnc.log

log "starting noVNC bridge on :6080 -> :5900"
# /usr/share/novnc is provided by the `novnc` apt package; vnc.html is
# the entry page that auto-connects to /websockify.
websockify --web=/usr/share/novnc 6080 127.0.0.1:5900 \
    >/tmp/websockify.log 2>&1 &

log "GUI stack ready. Launching: $*"
exec "$@"
