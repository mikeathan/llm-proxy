#!/usr/bin/env bash
# check-agent-harness.sh — verify the agent instruction system stays internally consistent.
#
# Guards the contract in AGENTS.md: skills declare `name` + `description` only, every
# repo path referenced by agent docs exists, every plan linked from the plans catalog
# exists, and the SPEC / plan catalogs agree with the files they describe. Run before
# finishing any change that touches AGENTS.md, README.md, docs/INDEX.md, docs/architecture.md,
# docs/SPECS/**, docs/PLANS/**, docs/audits/**, docs/guides/**, or .agents/**.
#
# Usage: ./scripts/check-agent-harness.sh
set -u

ROOT=$(git rev-parse --show-toplevel 2>/dev/null || pwd)
cd "$ROOT" || exit 2

fail=0
fail() { printf 'FAIL: %s\n' "$1"; fail=1; }
section() { printf '\n%s\n' "$1"; }

# Files whose referenced paths and relative markdown links must resolve. Plans
# (docs/PLANS/*/*.md) and audits are historical records that legitimately cite files
# that no longer exist, and docs/service_setup.md cites build outputs, so they are
# not scanned; their catalogs are (README.md in each directory).
DOCS=(
  AGENTS.md CLAUDE.md CONTRIBUTING.md README.md
  docs/INDEX.md docs/PLANS/README.md docs/SPECS/README.md docs/audits/README.md
  docs/architecture.md docs/SPEC-change-management.md docs/api-reference.md docs/data-layout.md
)
for f in .agents/skills/*/SKILL.md .agents/rules/*.md docs/SPECS/*.md docs/guides/*.md; do
  [ -e "$f" ] && DOCS+=("$f")
done

# --- 1. Skill frontmatter (name == dir, non-empty description, no when_to_use) ---
section "[1] skill frontmatter"
n=0
for f in .agents/skills/*/SKILL.md; do
  [ -e "$f" ] || continue
  n=$((n + 1))
  dir=$(basename "$(dirname "$f")")
  name=$(sed -n 's/^name: *//p' "$f" | head -1)
  desc=$(sed -n 's/^description: *//p' "$f" | head -1)
  [ "$name" = "$dir" ] || fail "$f: frontmatter name '${name:-<missing>}' != directory '$dir'"
  [ -n "$desc" ] || fail "$f: missing non-empty description"
  grep -q '^when_to_use:' "$f" && fail "$f: deprecated 'when_to_use' field — fold the trigger into description"
done
printf '  checked %d skills\n' "$n"

# --- 2. Backticked repo paths referenced by agent docs must exist ---
section "[2] referenced repo paths"
for f in "${DOCS[@]}"; do
  while IFS= read -r p; do
    [ -e "$p" ] || fail "$f: referenced path does not exist -> $p"
  done < <(
    grep -oE '`(docs|\.agents|backend|frontend|scripts|\.github)/[^`]*`' "$f" 2>/dev/null \
      | tr -d '`' | sed 's/#.*$//; s/[.,;:)]*$//' \
      | grep -vE '[*<>{}]|\.\.\.' | sort -u
  )
done

# --- 3. Relative markdown links must resolve ---
section "[3] relative markdown links"
for f in "${DOCS[@]}"; do
  [ -e "$f" ] || continue
  dir=$(dirname "$f")
  while IFS= read -r link; do
    case "$link" in http*|mailto:*|'#'*|'') continue ;; esac
    case "$link" in *'*'*|*'{'*|*'<'*|*'>'*|*...*) continue ;; esac
    target=${link%%#*}
    [ -z "$target" ] && continue
    [ -e "$dir/$target" ] || fail "$f: dead link -> $link"
  done < <(grep -oE '\]\([^)]+\)' "$f" 2>/dev/null | sed -E 's/^\]\(//; s/\)$//')
done

# --- 4. Plans catalog links ---
section "[4] docs/PLANS/README.md plan links"
while IFS= read -r p; do
  [ -e "docs/PLANS/$p" ] || fail "docs/PLANS/README.md: dead plan link -> $p"
done < <(grep -oE '\]\([A-Za-z0-9_./-]+\.md\)' docs/PLANS/README.md | sed -E 's/^\]\(//; s/\)$//')

# --- 5. Stale SPEC filename pattern (IDs live in docs/SPECS/README.md, not filenames) ---
section "[5] stale SPEC path pattern"
if grep -rnE 'SPEC-(NNN|[0-9][0-9][0-9])[-_][*<]' AGENTS.md CONTRIBUTING.md docs .agents/skills 2>/dev/null; then
  fail "use docs/SPECS/<name>.md (SPEC IDs live in docs/SPECS/README.md, not filenames)"
fi

# --- 6. docs/INDEX.md skills table matches the skill directories ---
section "[6] INDEX skills table vs .agents/skills/"
actual=$(for d in .agents/skills/*/; do basename "$d"; done | sort)
listed=$(sed -n '/^## Skills/,/^## Agent rules/p' docs/INDEX.md | grep -oE '^\| [a-z0-9-]+ ' | sed 's/^| //; s/ $//' | sort)
if [ "$actual" != "$listed" ]; then
  fail "docs/INDEX.md Skills table is out of sync with .agents/skills/"
  diff <(printf '%s\n' "$actual") <(printf '%s\n' "$listed") || true
fi

# --- 7. Canonical catalogs list every file ---
section "[7] canonical catalogs complete"
for f in docs/audits/*.md; do
  [ -e "$f" ] || continue
  [ "$(basename "$f")" = "README.md" ] && continue
  grep -q "$(basename "$f")" docs/audits/README.md || fail "docs/audits/README.md does not list $f"
done
for f in docs/SPECS/*.md; do
  [ -e "$f" ] || continue
  [ "$(basename "$f")" = "README.md" ] && continue
  grep -q "\`$(basename "$f")\`" docs/SPECS/README.md || fail "docs/SPECS/README.md does not list $f"
done
for f in docs/PLANS/*.md docs/PLANS/*/*.md; do
  [ -e "$f" ] || continue
  [ "$(basename "$(dirname "$f")")" = "ARCHIVE" ] && continue
  [ "$(basename "$f")" = "README.md" ] && continue
  rel=${f#docs/PLANS/}
  grep -q "($rel)" docs/PLANS/README.md || fail "docs/PLANS/README.md does not link $rel"
done

# --- 8. testing-guide lists every automation template ---
section "[8] testing-guide vs backend/data/templates"
for f in backend/data/templates/*.md; do
  [ -e "$f" ] || continue
  b=$(basename "$f")
  grep -q "\`$b\`" .agents/skills/testing-guide/SKILL.md || fail "testing-guide does not list template $b"
done

# --- 9. Each SPEC file's id and status match its docs/SPECS/README.md row ---
section "[9] SPEC frontmatter vs docs/SPECS/README.md"
for f in docs/SPECS/*.md; do
  [ -e "$f" ] || continue
  b=$(basename "$f")
  [ "$b" = "README.md" ] && continue
  id=$(sed -n 's/^id: *//p' "$f" | head -1)
  st=$(sed -n 's/^status: *//p' "$f" | head -1)
  row=$(grep -F "\`$b\`" docs/SPECS/README.md | head -1)
  [ -n "$id" ] || { fail "$f: frontmatter has no id"; continue; }
  printf '%s\n' "$row" | grep -q "^| $id |" || fail "$f: frontmatter id '$id' does not match its docs/SPECS/README.md row"
  cat_status=$(printf '%s\n' "$row" | awk -F'|' '{gsub(/ /, "", $5); print $5}')
  [ "$cat_status" = "$st" ] || fail "$f: status '$st' but docs/SPECS/README.md says '${cat_status:-<none>}'"
done

# --- 10. Plans table status matches each plan's own status line ---
# Compares the first word of the table's Status cell with the plan's own status
# (`status:` frontmatter or a `**Status:**` line in the first lines). Plans that
# declare neither are skipped.
section "[10] plan status vs docs/PLANS/README.md"
while IFS='|' read -r rel table_status; do
  f="docs/PLANS/$rel"
  [ -e "$f" ] || continue
  own=$(head -15 "$f" | sed -nE 's/^status: *//p; s/^\*\*Status:?\*\*:? *//p' | head -1 \
        | tr -d '`*' | awk '{print tolower($1)}' | sed 's/[—:,.-]*$//')
  [ -n "$own" ] || continue
  [ "$own" = "$table_status" ] || fail "docs/PLANS/README.md: $rel is '$table_status' in the table but '$own' in the plan"
done < <(
  sed -nE 's/^\| \[`[^`]+`\]\(([^)]+)\) \|.*\| ([^|]*[^| ]) \| [^|]+ \| [^|]+ \|$/\1|\2/p' docs/PLANS/README.md \
    | awk -F'|' '{ split($2, w, " "); print $1 "|" tolower(w[1]) }' | tr -d '*'
)

printf '\n%s\n' "=============================================="
if [ "$fail" -eq 0 ]; then
  printf 'PASS: agent harness is consistent.\n'
else
  printf 'FAIL: agent harness has inconsistencies (see above).\n'
fi
exit "$fail"
