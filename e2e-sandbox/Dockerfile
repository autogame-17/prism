# Generic Linux GUI e2e sandbox image.
#
# What this image gives you:
#   - Ubuntu 24.04 base
#   - Xvfb (virtual framebuffer) on display :99 so any X11 app can render
#     without a real GPU/display.
#   - x11vnc + noVNC: the X server is exposed as a VNC stream and noVNC
#     bridges it to HTTP on port 6080 so a browser MCP can see and screenshot
#     the running app.
#   - xdotool: terminal-injectable keyboard + mouse events. The test driver
#     calls `docker exec ... xdotool ...` so any project can script its own
#     interaction sequence without baking the test into the image.
#   - Build deps for Wails v2 on Linux: GTK3 + WebKit2GTK 4.1 (Ubuntu 24.04
#     ships 4.1 only, so projects must build with `-tags webkit2_41`).
#   - Go 1.25, Node 22 LTS, pnpm — pinned versions match prism-desktop's
#     go.mod / package.json so the in-container build matches the host build.
#
# Reuse for other Linux GUI projects:
#   1. Replace e2e-sandbox/launchers/prism.sh with your project's launcher.
#   2. Optionally trim the Wails-specific build deps in this Dockerfile.
#   3. The xdotool/VNC/Xvfb plumbing is application-agnostic.
FROM ubuntu:24.04

ENV DEBIAN_FRONTEND=noninteractive \
    LANG=C.UTF-8 \
    LC_ALL=C.UTF-8 \
    DISPLAY=:99 \
    XVFB_RES=1280x800x24 \
    HOME=/root \
    PATH="/root/go/bin:/usr/local/go/bin:/root/.local/bin:${PATH}"

# All apt installs in one layer to keep the image smaller.
# - x11vnc / xvfb / fluxbox: virtual desktop. fluxbox is a minimal WM so
#   that our app gets focus and the window manager hints work.
# - novnc / websockify: HTTP <-> VNC bridge.
# - xdotool / wmctrl: keyboard+mouse injection and window management.
# - libgtk-3-dev / libwebkit2gtk-4.1-dev: Wails v2 build deps on Ubuntu 24.04.
# - imagemagick: scripted screenshot fallback if the test wants stills
#   without going through noVNC.
# - dbus-x11: WebKit needs dbus session running.
# - sqlite3: tests inspect the prism_traces table.
RUN apt-get update && apt-get install -y --no-install-recommends \
        ca-certificates curl wget git build-essential pkg-config \
        xvfb x11vnc fluxbox xdotool wmctrl imagemagick \
        novnc websockify \
        dbus-x11 \
        libgtk-3-dev libwebkit2gtk-4.1-dev \
        libnss3 libasound2t64 \
        sqlite3 \
        ca-certificates && \
    rm -rf /var/lib/apt/lists/*

# Go 1.25 — pinned to match prism-desktop go.mod. arm64 image, native build.
ARG GO_VERSION=1.25.0
RUN ARCH=$(dpkg --print-architecture) && \
    curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-${ARCH}.tar.gz" \
        -o /tmp/go.tgz && \
    tar -C /usr/local -xzf /tmp/go.tgz && \
    rm /tmp/go.tgz && \
    /usr/local/go/bin/go version

# Node 22 LTS via NodeSource. Wails frontend uses pnpm; we install corepack-managed pnpm.
RUN curl -fsSL https://deb.nodesource.com/setup_22.x | bash - && \
    apt-get install -y --no-install-recommends nodejs && \
    rm -rf /var/lib/apt/lists/* && \
    corepack enable

# Wails v2 CLI — pinned to the version the project uses.
ARG WAILS_VERSION=v2.12.0
RUN /usr/local/go/bin/go install github.com/wailsapp/wails/v2/cmd/wails@${WAILS_VERSION}

# Healthcheck: noVNC reachable on 6080 means the GUI stack is up. The
# launcher is responsible for exiting if the app itself fails to start.
HEALTHCHECK --interval=5s --timeout=3s --start-period=10s --retries=10 \
    CMD curl -fsS http://127.0.0.1:6080/ >/dev/null || exit 1

EXPOSE 6080
WORKDIR /workspace
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
CMD ["/workspace/e2e-sandbox/launchers/prism.sh"]
