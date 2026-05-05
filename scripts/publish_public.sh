#!/usr/bin/env bash
#
# publish_public.sh — push the staged dist-public/ tree to the public mirror
# repo (autogame-17/prism) as a single squashed commit on main.
#
# Why squashed: the public repo intentionally does NOT carry the private
# repo's commit history. Each publish is one self-contained "release-style"
# commit. This is on purpose — it prevents leaking message bodies, internal
# review notes, or accidentally-committed secrets in old commits.
#
# Usage:
#   ./scripts/publish_public.sh [--message "your release commit message"]
#                               [--tag vX.Y.Z]
#                               [--dry-run]

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

MANIFEST="$ROOT/public.manifest.json"
OUT_DIR="$ROOT/dist-public"

PUBLIC_REPO="$(jq -r '.publicRepo' "$MANIFEST")"
PUBLIC_BRANCH="$(jq -r '.publicBranch' "$MANIFEST")"
PUBLIC_REMOTE="https://github.com/${PUBLIC_REPO}.git"

# ---- parse args ----
MESSAGE=""
TAG=""
DRY_RUN=0
while [[ $# -gt 0 ]]; do
  case "$1" in
    --message|-m) MESSAGE="$2"; shift 2 ;;
    --tag|-t)     TAG="$2"; shift 2 ;;
    --dry-run)    DRY_RUN=1; shift ;;
    -h|--help)
      grep '^#' "$0" | sed 's/^# \{0,1\}//' | head -20
      exit 0
      ;;
    *) echo "publish_public: unknown arg $1" >&2; exit 1 ;;
  esac
done

# ---- preflight ----
if [[ ! -d "$OUT_DIR" ]]; then
  echo "publish_public: $OUT_DIR not found. Run scripts/build_public.sh first." >&2
  exit 1
fi

PRIVATE_SHA="$(git rev-parse HEAD)"
PRIVATE_SHORT="$(git rev-parse --short HEAD)"
TIMESTAMP="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

if [[ -z "$MESSAGE" ]]; then
  MESSAGE="Sync from internal: ${PRIVATE_SHORT} (${TIMESTAMP})"
fi

# ---- secret sanity scan on the staged tree ----
echo ">> scanning dist-public/ for obvious secret patterns"
LEAK_HITS="$(
  grep -rE 'AKIA[0-9A-Z]{16}|sk-[a-zA-Z0-9_-]{32,}|ghp_[a-zA-Z0-9]{30,}|gho_[a-zA-Z0-9]{30,}|-----BEGIN [A-Z ]+PRIVATE KEY-----' \
    "$OUT_DIR" 2>/dev/null \
    | grep -v 'AKIA\.\.\.' \
    | grep -v 'placeholder' \
    || true
)"
if [[ -n "$LEAK_HITS" ]]; then
  echo "publish_public: ABORT — possible secret material in dist-public/:" >&2
  echo "$LEAK_HITS" >&2
  exit 2
fi

# ---- assemble a fresh git repo inside dist-public/ pointing at public remote ----
WORK_DIR="$(mktemp -d)"
trap 'rm -rf "$WORK_DIR"' EXIT
echo ">> staging public commit in $WORK_DIR"

# Mirror dist-public into a clean dir we can git-init.
(cd "$OUT_DIR" && tar cf - .) | (cd "$WORK_DIR" && tar xf -)

cd "$WORK_DIR"
git init -q -b "$PUBLIC_BRANCH"
git config user.name "autogame-17"
git config user.email "autogame-17@users.noreply.github.com"
git add -A
git commit -q -m "$MESSAGE" \
  -m "Source: private SHA ${PRIVATE_SHA}" \
  -m "Built at ${TIMESTAMP}"

git remote add origin "$PUBLIC_REMOTE"

# ---- show preview ----
echo
echo "About to force-push the following commit to ${PUBLIC_REPO}@${PUBLIC_BRANCH}:"
echo "----"
git log -1 --stat
echo "----"

if [[ $DRY_RUN -eq 1 ]]; then
  echo
  echo "[--dry-run] Skipping actual push. Inspect $WORK_DIR if needed."
  trap - EXIT
  echo "Working dir: $WORK_DIR"
  exit 0
fi

# ---- force-push to public main ----
echo ">> pushing to $PUBLIC_REMOTE"
git push --force origin "$PUBLIC_BRANCH"

# ---- optional tag ----
if [[ -n "$TAG" ]]; then
  echo ">> tagging $TAG"
  git tag -a "$TAG" -m "$MESSAGE"
  git push origin "$TAG"
fi

echo
echo "publish_public: done. https://github.com/${PUBLIC_REPO}"
