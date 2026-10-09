#!/usr/bin/env bash
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
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
bash "$root/scripts/crap-staged.sh" > "$fixture/output"
[[ "$(wc -l < "$fixture/args" | tr -d ' ')" == 1 ]]
grep -Fq 'staged source: :backend/path with spaces.go' "$fixture/content"
grep -Fq 'staged source: :frontend/src/a.ts' "$fixture/content"
grep -Fq '0% coverage' "$fixture/output"
[[ -z "$(find "$fixture" -name 'llm-proxy-crap.*' -print)" ]]
export CRAP_TEST_EXIT=1
if bash "$root/scripts/crap-staged.sh" > /dev/null 2>&1; then
  echo 'FAIL: scanner error was swallowed'; exit 1
fi
unset CRAP_TEST_EXIT
export CRAP_TEST_GO_EXIT=1
bash "$root/scripts/crap-staged.sh" > /dev/null
export CRAP_TEST_GO_EXIT=2
if bash "$root/scripts/crap-staged.sh" > /dev/null 2>&1; then
  echo 'FAIL: Go analysis error was swallowed'; exit 1
fi
unset CRAP_TEST_GO_EXIT
: > "$fixture/staged"
export CRAP_BIN="$fixture/missing"
export CRAP_GO_BIN="$fixture/missing"
bash "$root/scripts/crap-staged.sh" > "$fixture/output"
[[ ! -s "$fixture/output" ]]
printf '%s\0' 'backend/a.go' > "$fixture/staged"
bash "$root/scripts/crap-staged.sh" > "$fixture/output"
grep -Fq 'not installed' "$fixture/output"
if [[ $# == 2 ]]; then
  printf '%s\0' 'backend/path with spaces.go' 'frontend/src/a.ts' > "$fixture/staged"
  export CRAP_GO_BIN="$1" CRAP_BIN="$2"
  bash "$root/scripts/crap-staged.sh" > "$fixture/output"
  grep -Fq 'Pointer' "$fixture/output"
  grep -Fq 'Answer' "$fixture/output"
  [[ -z "$(find "$fixture" -name 'llm-proxy-crap.*' -print)" ]]
fi
echo 'PASS: staged Go/TS selection, index content, exclusions, errors, empty and missing-tool cases'
