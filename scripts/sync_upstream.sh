#!/usr/bin/env bash
#
# sync_upstream.sh — pull the latest MartialBE/one-api into backend/core and
# re-apply Prism-specific patches under backend/core-patches/.
#
# Placeholder for milestone M5; actual patch set lands as code diverges.
set -euo pipefail

echo "sync_upstream.sh not yet implemented."
echo "Manual procedure:"
echo "  1. git clone https://github.com/MartialBE/one-api /tmp/one-hub"
echo "  2. rsync -a --exclude=.git --exclude=web /tmp/one-hub/ backend/core/"
echo "  3. re-apply patches in backend/core-patches/*.patch"
