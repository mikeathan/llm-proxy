#!/usr/bin/env bash
set -euo pipefail

# scripts/tag_version.sh — manually create and push a release tag.
#
# Releases are normally automated: merging a PR to main bumps the root VERSION
# file and the release workflow tags it (CONTRIBUTING.md → Releases). Use this
# script only for an out-of-band tag. The tag is derived from the root VERSION
# file so it cannot drift from the documented single source of truth.
#
#   ./scripts/tag_version.sh              # tag v$(cat VERSION)
#   ./scripts/tag_version.sh 0.8.0        # tag v0.8.0 explicitly
#   ./scripts/tag_version.sh --dry-run    # print what would happen
#   ./scripts/tag_version.sh --yes        # skip the confirmation prompt

PRJ_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

# shellcheck source=lib/ui.sh
source "$PRJ_ROOT/scripts/lib/ui.sh"

DRY_RUN=0
ASSUME_YES=0
VERSION_ARG=""
for arg in "$@"; do
  case "$arg" in
    -h | --help)
      sed -n '4,14p' "$0"
      exit 0
      ;;
    --dry-run) DRY_RUN=1 ;;
    --yes)     ASSUME_YES=1 ;;
    -*)        ui_die "unknown option: $arg (-h for usage)" ;;
    *)         VERSION_ARG="$arg" ;;
  esac
done

cd "$PRJ_ROOT"

if [[ -n "$VERSION_ARG" ]]; then
  VERSION="$VERSION_ARG"
elif [[ -f VERSION ]]; then
  VERSION="$(tr -d '[:space:]' < VERSION)"
else
  ui_die "no VERSION file at $PRJ_ROOT/VERSION and no version argument given"
fi
[[ -n "$VERSION" ]] || ui_die "VERSION is empty"

# Accept both "0.8.0" and "v0.8.0"; git tags are always v-prefixed.
TAG="v${VERSION#v}"

if git rev-parse -q --verify "refs/tags/${TAG}" >/dev/null 2>&1; then
  ui_die "tag ${TAG} already exists"
fi

ui_header "tag release" "$TAG"
ui_detail "HEAD: $(git rev-parse --short HEAD)"

if [[ $DRY_RUN == 1 ]]; then
  ui_info "dry-run: git tag -a $TAG -m \"Release $TAG\""
  ui_info "dry-run: git push origin $TAG"
  exit 0
fi

if [[ $ASSUME_YES != 1 ]]; then
  printf '  Create and push %s? [y/N] ' "$TAG"
  read -r ans || ans=""
  [[ "$ans" =~ ^[Yy] ]] || { ui_warn "aborted"; exit 1; }
fi

git tag -a "$TAG" -m "Release $TAG"
git push origin "$TAG"
ui_ok "created and pushed ${TAG}"
