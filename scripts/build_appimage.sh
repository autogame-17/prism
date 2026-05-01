#!/usr/bin/env bash
# Bundle Prism's Linux build into an AppImage when appimagetool is available.
# Usage: ./scripts/build_appimage.sh <version> <arch>
set -euo pipefail

VERSION=${1:-dev}
ARCH=${2:-amd64}

BIN=build/bin/Prism
if [ ! -f "$BIN" ]; then
  echo "[appimage] build/bin/Prism not found; run 'wails build -platform linux/$ARCH' first"
  exit 0
fi

if ! command -v appimagetool >/dev/null 2>&1; then
  echo "[appimage] appimagetool not installed; skipping AppImage bundling."
  echo "           Install from https://github.com/AppImage/AppImageKit/releases"
  exit 0
fi

mkdir -p build/dist
WORK=$(mktemp -d)
APPDIR=$WORK/Prism.AppDir
mkdir -p "$APPDIR/usr/bin" "$APPDIR/usr/share/applications" "$APPDIR/usr/share/icons/hicolor/512x512/apps"

cp "$BIN" "$APPDIR/usr/bin/Prism"
chmod +x "$APPDIR/usr/bin/Prism"

if [ -f build/appicon.png ]; then
  cp build/appicon.png "$APPDIR/usr/share/icons/hicolor/512x512/apps/prism.png"
  cp build/appicon.png "$APPDIR/prism.png"
fi

cat > "$APPDIR/prism.desktop" <<'DESKTOP'
[Desktop Entry]
Name=Prism
Exec=Prism
Icon=prism
Type=Application
Categories=Utility;Development;
Terminal=false
DESKTOP
cp "$APPDIR/prism.desktop" "$APPDIR/usr/share/applications/prism.desktop"

cat > "$APPDIR/AppRun" <<'APPRUN'
#!/bin/sh
HERE="$(dirname "$(readlink -f "$0")")"
exec "$HERE/usr/bin/Prism" "$@"
APPRUN
chmod +x "$APPDIR/AppRun"

ARCH_UP=$(echo "$ARCH" | tr '[:lower:]' '[:upper:]')
ARCH=$ARCH_UP appimagetool "$APPDIR" "build/dist/Prism-$VERSION-linux-$ARCH.AppImage"
rm -rf "$WORK"
