#!/usr/bin/env bash
#
# fetch_cloudflared.sh — populate resources/cloudflared/<os>-<arch>/ with the
# latest cloudflared release for every platform Prism ships on.
#
# Usage: ./scripts/fetch_cloudflared.sh [version]
#        version defaults to "latest".
#
# Bash 3.2-compatible: macOS still ships /bin/bash 3.2 and `declare -A`
# (associative arrays) blow up there with the cryptic "unbound variable"
# error under `set -u`. We use parallel-key/value entries instead.
set -euo pipefail

VERSION="${1:-latest}"
BASE="https://github.com/cloudflare/cloudflared/releases/${VERSION}/download"
if [[ "$VERSION" == "latest" ]]; then
  BASE="https://github.com/cloudflare/cloudflared/releases/latest/download"
fi

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="$ROOT/resources/cloudflared"

# Each entry is "<os-arch>:<asset filename>". Order is preserved.
TARGETS=(
  "darwin-arm64:cloudflared-darwin-arm64.tgz"
  "darwin-amd64:cloudflared-darwin-amd64.tgz"
  "linux-amd64:cloudflared-linux-amd64"
  "linux-arm64:cloudflared-linux-arm64"
  "windows-amd64:cloudflared-windows-amd64.exe"
)

mkdir -p "$OUT"
for entry in "${TARGETS[@]}"; do
  key="${entry%%:*}"
  asset="${entry#*:}"
  dir="$OUT/$key"
  mkdir -p "$dir"
  echo ">> $key <= $asset"
  case "$asset" in
    *.tgz)
      curl -fsSL "$BASE/$asset" -o "$dir/tmp.tgz"
      tar -xzf "$dir/tmp.tgz" -C "$dir"
      rm -f "$dir/tmp.tgz"
      ;;
    *.exe)
      curl -fsSL "$BASE/$asset" -o "$dir/cloudflared.exe"
      chmod +x "$dir/cloudflared.exe"
      ;;
    *)
      curl -fsSL "$BASE/$asset" -o "$dir/cloudflared"
      chmod +x "$dir/cloudflared"
      ;;
  esac
done

echo "$VERSION" > "$OUT/VERSION"
echo "done. artifacts in $OUT"
