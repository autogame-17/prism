#!/usr/bin/env bash
#
# build_public.sh — assemble the public mirror tree from the current private
# checkout, into dist-public/.
#
# How it works:
#   1. List every git-tracked file in the current repo.
#   2. Drop anything matched by `exclude` in public.manifest.json.
#   3. Apply renames (e.g. README.public.md -> README.md).
#   4. Apply rewrites (small in-file string substitutions).
#   5. Splat .public-only/ on top (CONTRIBUTING.md, SECURITY.md, .github/...).
#
# Usage:
#   ./scripts/build_public.sh
#
# Output: dist-public/ contains exactly what the public repo's main branch
# should look like. Diff freely before publishing.

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

MANIFEST="$ROOT/public.manifest.json"
OUT_DIR="$ROOT/dist-public"

if [[ ! -f "$MANIFEST" ]]; then
  echo "build_public: missing $MANIFEST" >&2
  exit 1
fi

if ! command -v jq >/dev/null 2>&1; then
  echo "build_public: 'jq' is required (brew install jq)" >&2
  exit 1
fi

echo ">> wiping $OUT_DIR"
rm -rf "$OUT_DIR"
mkdir -p "$OUT_DIR"

# ---- step 1: collect tracked files ----
TRACKED_LIST="$(mktemp)"
trap 'rm -f "$TRACKED_LIST"' EXIT
git ls-files > "$TRACKED_LIST"

# ---- step 2: build exclusion list (pathspec) ----
EXCLUDE_PATTERNS=()
while IFS= read -r line; do
  EXCLUDE_PATTERNS+=("$line")
done < <(jq -r '.exclude[]' "$MANIFEST")

is_excluded() {
  local path="$1"
  local pat
  for pat in "${EXCLUDE_PATTERNS[@]}"; do
    # support trailing /** as recursive prefix; trailing * as glob.
    case "$path" in
      $pat) return 0 ;;
    esac
  done
  return 1
}

# ---- step 3: collect rename + rewrite tables ----
# bash 3.2 has no associative arrays; use parallel arrays keyed by index.
RENAME_KEYS=()
RENAME_VALS=()
while IFS=$'\t' read -r src dst; do
  RENAME_KEYS+=("$src")
  RENAME_VALS+=("$dst")
done < <(jq -r '.rename | to_entries[] | "\(.key)\t\(.value)"' "$MANIFEST")

lookup_rename() {
  local needle="$1"
  local i
  for (( i=0; i<${#RENAME_KEYS[@]}; i++ )); do
    if [[ "${RENAME_KEYS[$i]}" == "$needle" ]]; then
      echo "${RENAME_VALS[$i]}"
      return 0
    fi
  done
  echo "$needle"
}

# ---- step 4: copy each file ----
COUNT_INCLUDED=0
COUNT_EXCLUDED=0
COUNT_RENAMED=0

while IFS= read -r src; do
  if is_excluded "$src"; then
    COUNT_EXCLUDED=$((COUNT_EXCLUDED + 1))
    continue
  fi

  dst="$(lookup_rename "$src")"
  if [[ "$dst" != "$src" ]]; then
    COUNT_RENAMED=$((COUNT_RENAMED + 1))
  fi

  mkdir -p "$OUT_DIR/$(dirname "$dst")"
  cp -p "$src" "$OUT_DIR/$dst"
  COUNT_INCLUDED=$((COUNT_INCLUDED + 1))
done < "$TRACKED_LIST"

# ---- step 5: apply rewrites ----
# rewrites is { "<file>": [ {"from": "...", "to": "..."} ] }
# Use python for safe multi-line literal replacement.
python3 - "$MANIFEST" "$OUT_DIR" <<'PY'
import json, sys, pathlib
manifest_path, out_dir = sys.argv[1], pathlib.Path(sys.argv[2])
manifest = json.loads(pathlib.Path(manifest_path).read_text())
rewrites = manifest.get("rewrite", {})
for fname, rules in rewrites.items():
    target = out_dir / fname
    if not target.exists():
        print(f"  rewrite: skipped (not in mirror): {fname}", file=sys.stderr)
        continue
    text = target.read_text()
    for rule in rules:
        before = text
        text = text.replace(rule["from"], rule["to"])
        if text == before:
            print(f"  rewrite: WARNING no match in {fname} for {rule['from']!r}", file=sys.stderr)
    target.write_text(text)
    print(f"  rewrite: {fname}")
PY

# ---- step 6: overlay .public-only/ ----
PUBLIC_ONLY_DIR="$ROOT/.public-only"
if [[ -d "$PUBLIC_ONLY_DIR" ]]; then
  COUNT_OVERLAY=$(find "$PUBLIC_ONLY_DIR" -type f | wc -l | tr -d ' ')
  echo ">> overlaying $COUNT_OVERLAY file(s) from .public-only/"
  # -a copies everything, including dotfiles like .github/
  (cd "$PUBLIC_ONLY_DIR" && tar cf - .) | (cd "$OUT_DIR" && tar xf -)
fi

# ---- summary ----
TOTAL=$(find "$OUT_DIR" -type f | wc -l | tr -d ' ')
echo
echo "build_public: done."
echo "  included:  $COUNT_INCLUDED"
echo "  excluded:  $COUNT_EXCLUDED"
echo "  renamed:   $COUNT_RENAMED"
echo "  overlay:   ${COUNT_OVERLAY:-0}"
echo "  --"
echo "  total in dist-public: $TOTAL"
echo
echo "Inspect with: ls -la dist-public && (cd dist-public && git init && git add -A && git status)"
