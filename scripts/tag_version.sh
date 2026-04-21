#!/usr/bin/env bash
set -euo pipefail

VERSION="v0.4.2"

if [[ -z "${VERSION}" ]]; then
  echo "VERSION is empty. Set VERSION in scripts/tag_version.sh."
  exit 1
fi

if git rev-parse "${VERSION}" >/dev/null 2>&1; then
  echo "Tag ${VERSION} already exists."
  exit 1
fi

git tag -a "${VERSION}" -m "Release ${VERSION}"
git push origin "${VERSION}"
echo "Created and pushed tag ${VERSION}"
