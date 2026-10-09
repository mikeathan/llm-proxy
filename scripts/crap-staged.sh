#!/usr/bin/env bash
# Fast, report-only estimate: inspect staged source without tests or downloads.
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"
staged="$(mktemp -d "${TMPDIR:-/tmp}/llm-proxy-crap.XXXXXX")"
trap 'rm -rf "$staged"' EXIT
git diff --cached --name-only --diff-filter=ACMR -z > "$staged/paths"
paths=()
go_count=0
while IFS= read -r -d '' path; do
  case "$path" in
    *_test.go|*.test.ts|*.spec.ts|*.test.tsx|*.spec.tsx|*.d.ts|*/__TESTS__/*|*/__tests__/*|*/testdata/*) continue ;;
    *.go|*.ts|*.tsx) ;;
    *) continue ;;
  esac
  # Symlinks and submodules are not source files, even with a source extension.
  mode="$(git ls-files --stage -- "$path")"
  case "$mode" in 100644\ *|100755\ *) ;; *) continue ;; esac
  case "$path" in
    *.go)
      mkdir -p "$staged/go/$(dirname "$path")"
      git show ":$path" > "$staged/go/$path"
      go_count=$((go_count + 1)) ;;
    *)
      mkdir -p "$staged/ts/$(dirname "$path")"
      git show ":$path" > "$staged/ts/$path"
      paths+=("./$path") ;;
  esac
done < "$staged/paths"
[[ ${#paths[@]} -gt 0 || $go_count -gt 0 ]] || exit 0
echo '[crap] staged Go/TS files; estimate assumes 0% coverage. No tests run; report only.'
if [[ $go_count -gt 0 ]]; then
  scanner="${CRAP_GO_BIN:-crap4go}"
  if command -v "$scanner" >/dev/null 2>&1; then
    printf 'module staged\n\ngo 1.26\n' > "$staged/go/go.mod"
    # A header-only profile makes absent coverage explicit; it contains no hits.
    printf 'mode: set\n' > "$staged/go/coverage.out"
    result=0
    GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off "$scanner" --dir "$staged/go" --coverage "$staged/go/coverage.out" --ceiling 30 || result=$?
    # crap4go: 1 is a score breach (report only), 2 is an analysis error.
    [[ $result -le 1 ]] || exit "$result"
  else
    echo '[crap] crap4go not installed; skipping Go. See CONTRIBUTING.md for setup.'
  fi
fi
if [[ ${#paths[@]} -gt 0 ]]; then
  scanner="${CRAP_BIN:-crap}"
  if command -v "$scanner" >/dev/null 2>&1; then
    cd "$staged/ts"
    "$scanner" "${paths[@]}"
  else
    echo '[crap] crap not installed; skipping TypeScript. See CONTRIBUTING.md for setup.'
  fi
fi
