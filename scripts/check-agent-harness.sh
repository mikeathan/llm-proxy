#!/usr/bin/env bash
# check-agent-harness.sh — verify the agent instruction system and the docs memory stay consistent.
#
# Guards the contract in AGENTS.md and docs/decisions/ADR-0001 / ADR-0002: skills follow the Agent Skills
# format and stay small, every repo path and relative link in agent-facing docs resolves, every catalog
# lists the files it describes, plan statuses agree, no finished plan sits in the live tracker, subsystem
# hubs cover every open plan and ADR, and agent-facing files stay vendor-neutral. Run before finishing any
# change that touches AGENTS.md, CLAUDE.md, README.md, CONTRIBUTING.md, docs/**, .agents/**,
# backend/AGENTS.md or frontend/AGENTS.md.
#
# Portable bash + POSIX tools (runs on Linux and macOS). Usage: ./scripts/check-agent-harness.sh
set -u

ROOT=$(git rev-parse --show-toplevel 2>/dev/null || pwd)
cd "$ROOT" || exit 2

fail=0
fail() { printf 'FAIL: %s\n' "$1"; fail=1; }
warn() { printf 'WARN: %s\n' "$1"; }
section() { printf '\n%s\n' "$1"; }

SKILL_MAX_LINES=150      # SKILL.md body budget; detail goes in references/ (ADR-0001, Agent Skills format)
SKILL_STALE_DAYS=120     # warn (not fail) when metadata.last_reviewed is older than this

# Files whose referenced paths and relative markdown links must resolve. Plans
# (docs/PLANS/*/*.md, ARCHIVE) and audits are historical records that legitimately cite files
# that no longer exist, and docs/service_setup.md cites build outputs, so they are
# not scanned; their catalogs are (README.md in each directory).
DOCS=(
  AGENTS.md CLAUDE.md CONTRIBUTING.md README.md backend/AGENTS.md frontend/AGENTS.md
  docs/INDEX.md docs/PLANS/README.md docs/PLANS/ARCHIVE/README.md docs/SPECS/README.md docs/audits/README.md
  docs/architecture.md docs/SPEC-change-management.md docs/api-reference.md docs/data-layout.md
)
for f in .agents/skills/*/SKILL.md .agents/skills/*/references/*.md .agents/rules/*.md docs/SPECS/*.md \
         docs/guides/*.md docs/architecture/*.md docs/subsystems/*.md docs/decisions/*.md docs/learnings/*.md; do
  [ -e "$f" ] && DOCS+=("$f")
done

# Frontmatter value of key $2 in file $1 (top-level keys only).
fm() { awk -v k="$2" 'NR==1 && $0!="---"{exit} NR>1 && $0=="---"{exit} NR>1 && index($0, k": ")==1 {sub("^" k ": *", ""); print; exit}' "$1"; }

# --- 1. Skills: Agent Skills frontmatter, size budget, references wiring ---
section "[1] skills (frontmatter, size, references)"
n=0
today=$(date +%Y%m%d)
for f in .agents/skills/*/SKILL.md; do
  [ -e "$f" ] || continue
  n=$((n + 1))
  dir=$(dirname "$f"); skill=$(basename "$dir")
  name=$(fm "$f" name); desc=$(fm "$f" description)
  [ "$name" = "$skill" ] || fail "$f: frontmatter name '${name:-<missing>}' != directory '$skill'"
  printf '%s' "$name" | grep -qE '^[a-z0-9]+(-[a-z0-9]+)*$' && [ ${#name} -le 64 ] \
    || fail "$f: name must be lowercase letters, digits and single hyphens, ≤64 chars"
  [ -n "$desc" ] || fail "$f: missing non-empty description"
  [ ${#desc} -le 1024 ] || fail "$f: description longer than 1024 chars"
  # Only Agent Skills keys at the top level; repo bookkeeping goes under metadata.
  bad=$(awk 'NR==1{next} $0=="---"{exit} /^[A-Za-z_-]+:/{k=$0; sub(/:.*/, "", k); print k}' "$f" \
        | grep -vxE 'name|description|license|compatibility|metadata|allowed-tools' || true)
  [ -z "$bad" ] || fail "$f: non-standard top-level frontmatter key(s): $(echo $bad) — move them under metadata"
  lr=$(awk 'NR==1{next} $0=="---"{exit} /^  last_reviewed:/{gsub(/[^0-9-]/, "", $2); print $2}' "$f")
  if [ -z "$lr" ]; then
    fail "$f: missing metadata.last_reviewed"
  else
    lrn=$(printf '%s' "$lr" | tr -d '-')
    # Approximate day arithmetic (no GNU date): 372-day years, 31-day months — fine for a warning.
    age=$(( (10#${today:0:4} - 10#${lrn:0:4}) * 372 + (10#${today:4:2} - 10#${lrn:4:2}) * 31 + 10#${today:6:2} - 10#${lrn:6:2} ))
    [ "$age" -le "$SKILL_STALE_DAYS" ] || warn "$f: last_reviewed $lr is over $SKILL_STALE_DAYS days old — review it"
  fi
  lines=$(wc -l < "$f" | tr -d ' ')
  [ "$lines" -le "$SKILL_MAX_LINES" ] || fail "$f: $lines lines > $SKILL_MAX_LINES — move detail into references/"
  grep -q '^when_to_use:' "$f" && fail "$f: deprecated 'when_to_use' field — fold the trigger into description"
  if [ -d "$dir/references" ]; then
    for r in "$dir"/references/*; do
      [ -e "$r" ] || continue
      [ -d "$r" ] && fail "$r: references/ is one level deep — no subdirectories"
      [ -f "$r" ] && { grep -qF "references/$(basename "$r")" "$f" || fail "$f: does not link $r"; }
    done
  fi
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
catalog() { # catalog <readme> <file>... : every file's basename must appear in the readme
  local readme=$1; shift
  for f in "$@"; do
    [ -e "$f" ] || continue
    [ "$(basename "$f")" = "README.md" ] && continue
    grep -qF "$(basename "$f")" "$readme" || fail "$readme does not list $f"
  done
}
catalog docs/audits/README.md docs/audits/*.md
catalog docs/decisions/README.md docs/decisions/ADR-*.md
catalog docs/learnings/README.md docs/learnings/*.md
catalog docs/subsystems/README.md docs/subsystems/*.md
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
for f in docs/PLANS/ARCHIVE/*/*.md; do
  [ -e "$f" ] || continue
  rel=${f#docs/PLANS/ARCHIVE/}
  grep -q "($rel)" docs/PLANS/ARCHIVE/README.md || fail "docs/PLANS/ARCHIVE/README.md does not link $rel"
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

# --- 10. Live plans: table status matches each plan, and nothing finished stays live ---
# Rows: | [Name](path) | status | date | specs | what's left |. The plan's own status is its
# `status:` frontmatter or a `**Status:**` line in the first lines; plans that declare neither are skipped.
section "[10] plan status vs docs/PLANS/README.md"
while IFS='|' read -r rel table_status; do
  case "$table_status" in complete|superseded|reverted)
    fail "docs/PLANS/README.md: $rel is '$table_status' — harvest it and move it to docs/PLANS/ARCHIVE/" ;;
  esac
  f="docs/PLANS/$rel"
  [ -e "$f" ] || continue
  own=$(head -15 "$f" | sed -nE 's/^status: *//p; s/^\*\*Status:?\*\*:? *//p' | head -1 \
        | tr -d '`*' | awk '{print tolower($1)}' | sed 's/[—:,.-]*$//')
  [ -n "$own" ] || continue
  [ "$own" = "$table_status" ] || fail "docs/PLANS/README.md: $rel is '$table_status' in the table but '$own' in the plan"
done < <(
  sed -nE 's/^\| \[[^]]+\]\(([^)]+\.md)\) \| ([^|]+) \|.*$/\1|\2/p' docs/PLANS/README.md \
    | awk -F'|' '{ split($2, w, " "); print $1 "|" tolower(w[1]) }' | tr -d '*`'
)

# --- 11. Subsystem hubs: every open plan and ADR is reachable from a hub; hubs point at real SPECs ---
section "[11] subsystem hubs"
for h in docs/subsystems/*.md; do
  [ "$(basename "$h")" = "README.md" ] && continue
  slug=$(basename "$h" .md)
  [ "$(fm "$h" subsystem)" = "$slug" ] || fail "$h: frontmatter subsystem must be '$slug'"
  [ -e "docs/learnings/$slug.md" ] || fail "$h: no learnings ledger docs/learnings/$slug.md"
  grep -q '\.\./SPECS/[a-z0-9-]*\.md' "$h" || grep -q 'no SPEC yet' "$h" \
    || fail "$h: links no SPEC and does not say 'no SPEC yet'"
done
while IFS= read -r p; do
  grep -qF "(../PLANS/$p)" docs/subsystems/*.md || fail "open plan $p is not linked from any docs/subsystems/ hub"
done < <(sed -nE 's/^\| \[[^]]+\]\(([^)]+\.md)\) \|.*$/\1/p' docs/PLANS/README.md)
for a in docs/decisions/ADR-*.md; do
  [ -e "$a" ] || continue
  for s in $(fm "$a" subsystems | tr -d '[],'); do
    [ -e "docs/subsystems/$s.md" ] || { fail "$a: unknown subsystem '$s'"; continue; }
    grep -qF "$(basename "$a")" "docs/subsystems/$s.md" || fail "docs/subsystems/$s.md does not link $(basename "$a")"
  done
done

# --- 12. ADRs and audits carry the frontmatter their catalogs rely on ---
section "[12] ADR and audit frontmatter"
for a in docs/decisions/ADR-*.md; do
  [ -e "$a" ] || continue
  num=$(basename "$a" | sed -nE 's/^ADR-([0-9]{4})-[a-z0-9-]+\.md$/\1/p')
  [ -n "$num" ] || { fail "$a: name must be ADR-NNNN-<kebab-slug>.md"; continue; }
  [ "$(fm "$a" id)" = "ADR-$num" ] || fail "$a: frontmatter id must be ADR-$num"
  case "$(fm "$a" status)" in proposed|accepted|superseded|deprecated) ;; *) fail "$a: status must be proposed|accepted|superseded|deprecated" ;; esac
done
for f in docs/audits/*.md; do
  [ "$(basename "$f")" = "README.md" ] && continue
  s=$(fm "$f" subsystem)
  [ -n "$s" ] && [ -e "docs/subsystems/$s.md" ] || fail "$f: frontmatter subsystem '${s:-<missing>}' is not a docs/subsystems/ hub"
  case "$(fm "$f" result)" in worked|failed|inconclusive|reference) ;; *) fail "$f: frontmatter result must be worked|failed|inconclusive|reference" ;; esac
done

# --- 13. Harness adapters and vendor-neutral agent instructions (ADR-0001) ---
section "[13] harness adapters and vendor neutrality"
grep -qx '@AGENTS.md' CLAUDE.md || fail "CLAUDE.md must import @AGENTS.md"
[ "$(wc -l < CLAUDE.md | tr -d ' ')" -le 20 ] || fail "CLAUDE.md must stay a ≤20-line pointer to AGENTS.md"
[ -L .claude/skills ] && [ "$(readlink .claude/skills)" = "../.agents/skills" ] \
  || fail ".claude/skills must be a symlink to ../.agents/skills"
extra=$(git ls-files .claude | grep -vx '.claude/skills' || true)
[ -z "$extra" ] || fail "only .claude/skills may exist under .claude/ (ADR-0001): $(echo $extra)"
nested=$(git ls-files '*AGENTS.md' | grep -v '^AGENTS.md$' | grep -vxE 'backend/AGENTS.md|frontend/AGENTS.md' || true)
[ -z "$nested" ] || fail "unexpected AGENTS.md files (product templates use AGENTS.template.md): $(echo $nested)"
for f in backend/AGENTS.md frontend/AGENTS.md; do
  [ -e "$f" ] || continue
  grep -qF '](../AGENTS.md)' "$f" || fail "$f: must send the agent to ../AGENTS.md first"
done
NEUTRAL=(AGENTS.md backend/AGENTS.md frontend/AGENTS.md CONTRIBUTING.md README.md docs/INDEX.md docs/architecture.md)
for f in .agents/skills/*/SKILL.md .agents/skills/*/references/*.md .agents/rules/*.md docs/architecture/*.md \
         docs/subsystems/*.md docs/decisions/*.md docs/learnings/*.md docs/SPECS/*.md docs/guides/*.md; do
  case "$f" in docs/decisions/ADR-0001-*) continue ;; esac   # the compatibility record has to name the tools
  [ -e "$f" ] && NEUTRAL+=("$f")
done
hits=$(grep -il 'claude' "${NEUTRAL[@]}" 2>/dev/null || true)
[ -z "$hits" ] || fail "vendor name in agent-facing docs (keep it in CLAUDE.md only): $(echo $hits)"

printf '\n%s\n' "=============================================="
if [ "$fail" -eq 0 ]; then
  printf 'PASS: agent harness is consistent.\n'
else
  printf 'FAIL: agent harness has inconsistencies (see above).\n'
fi
exit "$fail"
