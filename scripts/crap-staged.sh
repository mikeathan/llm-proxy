#!/usr/bin/env bash
# Fast, report-only estimate: inspect staged source without tests or downloads.
set -euo pipefail

self_test() (
  script="$(cd "$(dirname "$0")" && pwd)/$(basename "$0")"
  fixture="$(mktemp -d)"
  trap 'rm -rf "$fixture"' EXIT
  mkdir -p "$fixture/bin"
  export CRAP_TEST_FIXTURE="$fixture"
  export TMPDIR="$fixture"
  cat > "$fixture/bin/git" <<'SH'
#!/usr/bin/env bash
case "$1" in
  rev-parse) printf '%s\n' "$CRAP_TEST_FIXTURE" ;;
  diff) cat "$CRAP_TEST_FIXTURE/staged" ;;
  ls-files) printf '100644 abc 0\t%s\n' "$4" ;;
  show)
    printf '// staged source: %s\n' "$2"
    case "$2" in
      *.go) printf 'package sample\nfunc Pointer() *int { return new(42) }\n' ;;
      *.ts) printf 'export function Answer(): number { return 42 }\n' ;;
    esac ;;
  *) exit 1 ;;
esac
SH
  cat > "$fixture/bin/crap" <<'SH'
#!/usr/bin/env bash
printf '%s\n' "$@" > "$CRAP_TEST_FIXTURE/args"
for path in "$@"; do
  [[ -f "$path" ]] || continue
  cat "$path" >> "$CRAP_TEST_FIXTURE/content"
done
exit "${CRAP_TEST_EXIT:-0}"
SH
  chmod +x "$fixture/bin/git" "$fixture/bin/crap"
  cat > "$fixture/bin/crap4go" <<'SH'
#!/usr/bin/env bash
cat "$2/backend/path with spaces.go" >> "$CRAP_TEST_FIXTURE/content"
exit "${CRAP_TEST_GO_EXIT:-0}"
SH
  chmod +x "$fixture/bin/crap4go"
  export PATH="$fixture/bin:$PATH"
  export CRAP_BIN="$fixture/bin/crap"
  export CRAP_GO_BIN="$fixture/bin/crap4go"
  printf '%s\0' 'backend/path with spaces.go' 'frontend/src/a.ts' 'frontend/src/A.vue' 'backend/a_test.go' 'frontend/src/__TESTS__/helper.ts' 'frontend/src/types/a.d.ts' > "$fixture/staged"
  bash "$script" > "$fixture/output"
  [[ "$(wc -l < "$fixture/args" | tr -d ' ')" == 1 ]]
  grep -Fq 'staged source: :backend/path with spaces.go' "$fixture/content"
  grep -Fq 'staged source: :frontend/src/a.ts' "$fixture/content"
  grep -Fq '0% coverage' "$fixture/output"
  [[ -z "$(find "$fixture" -name 'llm-proxy-crap.*' -print)" ]]
  export CRAP_TEST_EXIT=1
  if bash "$script" > /dev/null 2>&1; then
    echo 'FAIL: scanner error was swallowed'; exit 1
  fi
  unset CRAP_TEST_EXIT
  export CRAP_TEST_GO_EXIT=1
  bash "$script" > /dev/null
  export CRAP_TEST_GO_EXIT=2
  if bash "$script" > /dev/null 2>&1; then
    echo 'FAIL: Go analysis error was swallowed'; exit 1
  fi
  unset CRAP_TEST_GO_EXIT
  : > "$fixture/staged"
  export CRAP_BIN="$fixture/missing"
  export CRAP_GO_BIN="$fixture/missing"
  bash "$script" > "$fixture/output"
  [[ ! -s "$fixture/output" ]]
  printf '%s\0' 'backend/a.go' > "$fixture/staged"
  bash "$script" > "$fixture/output"
  grep -Fq 'not installed' "$fixture/output"
  if [[ $# == 2 ]]; then
    printf '%s\0' 'backend/path with spaces.go' 'frontend/src/a.ts' > "$fixture/staged"
    export CRAP_GO_BIN="$1" CRAP_BIN="$2"
    bash "$script" > "$fixture/output"
    grep -Fq 'Pointer' "$fixture/output"
    grep -Fq 'Answer' "$fixture/output"
    [[ -z "$(find "$fixture" -name 'llm-proxy-crap.*' -print)" ]]
  fi
  echo 'PASS: staged Go/TS selection, index content, exclusions, errors, empty and missing-tool cases'
)

if [[ ${1:-} == --self-test ]]; then
  shift
  self_test "$@"
  exit 0
fi
if [[ $# -ne 0 ]]; then
  echo 'Usage: bash scripts/crap-staged.sh [--self-test [GO_SCANNER TS_SCANNER]]' >&2
  exit 2
fi

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
