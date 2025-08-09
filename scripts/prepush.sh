#!/usr/bin/env bash
set -euo pipefail

# Config (override with env if you like)
GOOS_DEFAULT="${GOOS_DEFAULT:-linux}"
GOARCH_DEFAULT="${GOARCH_DEFAULT:-amd64}"

# Ensure we can diff against main
git fetch origin +refs/heads/main:refs/remotes/origin/main --depth=0 >/dev/null 2>&1 || true
BASE="$(git merge-base origin/main HEAD || git rev-list --max-parents=0 HEAD)"

# What changed?
mapfile -t CHANGED < <(git diff --name-only "$BASE..HEAD" || true)

# Decide which apps to build
APPS=()
shared_hit=false
for f in "${CHANGED[@]}"; do
  [[ "$f" =~ ^(go\.mod|go\.sum|internal/|pkg/) ]] && shared_hit=true && break
done

if $shared_hit; then
  # shared code changed -> build all apps
  mapfile -t APPS < <(ls -d cmd/*/ 2>/dev/null | xargs -n1 basename)
else
  # only build apps with changes under cmd/<app>/
  while IFS= read -r app; do
    APPS+=("$app")
  done < <(printf '%s\n' "${CHANGED[@]}" | awk -F/ '/^cmd\/[^/]+\//{print $2}' | sort -u)
fi

# Nothing to build? exit 0 to not block the push.
if [ ${#APPS[@]} -eq 0 ]; then
  echo "[prepush] No app-level changes; nothing to build."
  exit 0
fi

echo "[prepush] Apps to build: ${APPS[*]}"

# Build for requested targets (defaults to linux/amd64)
GOOS_LIST=(${GOOS_LIST:-$GOOS_DEFAULT})
GOARCH_LIST=(${GOARCH_LIST:-$GOARCH_DEFAULT})

for GOOS in "${GOOS_LIST[@]}"; do
  for GOARCH in "${GOARCH_LIST[@]}"; do
    export GOOS GOARCH
    for APP_NAME in "${APPS[@]}"; do
      echo "[prepush] Building $APP_NAME for $GOOS-$GOARCH..."
      APP_NAME="$APP_NAME" make build
    done
  done
done

echo "[prepush] Build OK."

