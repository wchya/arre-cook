#!/usr/bin/env bash
set -euo pipefail

version="$(tr -d '[:space:]' < VERSION)"
if [[ ! "$version" =~ ^([0-9]+)\.([0-9]+)\.([0-9]+)$ ]]; then
  echo "VERSION must contain X.Y.Z" >&2
  exit 1
fi
major="${BASH_REMATCH[1]}"
minor="${BASH_REMATCH[2]}"
patch="${BASH_REMATCH[3]}"
range="$(git log --format='%s%n%b' -20 2>/dev/null || true)"
if [[ "$range" == *BREAKING* || "$range" == *"!"* ]]; then
  major=$((major + 1)); minor=0; patch=0
elif grep -Eiq '(^|[[:space:]])feat(\(|:|!)' <<<"$range"; then
  minor=$((minor + 1)); patch=0
else
  patch=$((patch + 1))
fi
printf '%s.%s.%s\n' "$major" "$minor" "$patch"
