#!/usr/bin/env bash
# skill-eval.sh — compare an agent skill WITH and WITHOUT, on past tasks, in throwaway worktrees.
# Guide (purpose, goal, who runs it, how to score): docs/guides/skill-eval.md. Plan: docs/PLANS/cross-cutting/skill-eval.md.
#
# This script never starts an agent. It prepares the folders, prints the prompt, and collects and checks what an agent left
# behind. Any coding agent on any model can be pointed at a prepared folder.
#
# Git use: `setup` and `cleanup` add and remove DETACHED worktrees (no branch, nothing committed, nothing pushed). Everything
# else only reads git. Worktrees live outside the repository, under $SKILL_EVAL_OUT (default: <repo>-skill-eval, next to it).
#
# Usage:
#   skill-eval.sh check-tasks [T1,T2,...]   self-test the tasks: the reference test must fail before the fix and pass after it
#   skill-eval.sh plan <skill> <tasks> <n>   random order of n with/without pairs per task; writes KEY.tsv (arms) and prints the order
#   skill-eval.sh setup <run>                make the worktree for a run (arm "without" has the skill folder deleted)
#   skill-eval.sh prompt <run>               print the text to give the agent (rules + task)
#   skill-eval.sh guard                      show the main checkout's state; run before and after a batch, it must not change
#   skill-eval.sh collect <run>              save the agent's diff and start the score sheet
#   skill-eval.sh verify <run>               run the reference test against the agent's change (no worktree needed)
#   skill-eval.sh report                     counts per arm: reference test result and checks ticked on the score sheets
#   skill-eval.sh cleanup <run|all>          remove the worktree(s)
set -euo pipefail

ROOT=$(git rev-parse --show-toplevel)
OUT=${SKILL_EVAL_OUT:-"$(dirname "$ROOT")/$(basename "$ROOT")-skill-eval"}
KIT="$ROOT/scripts/skill-eval"

die() { echo "skill-eval: $*" >&2; exit 1; }

load_task() { # load_task <id>: sets SKILL FIX SRC TEST_FILES PKG TEST_RUN
  [ -f "$KIT/tasks/$1.env" ] || die "unknown task '$1' (files in $KIT/tasks/)"
  # shellcheck disable=SC1090
  . "$KIT/tasks/$1.env"
}

key_row() { # key_row <run>: prints "task<TAB>arm" for a run
  [ -f "$OUT/KEY.tsv" ] || die "no $OUT/KEY.tsv: run 'plan' first"
  awk -F'\t' -v r="$1" '$1==r {print $2 "\t" $3; found=1} END {exit !found}' "$OUT/KEY.tsv" || die "unknown run '$1'"
}

# Export the parent of FIX (the backend only) into a fresh folder; used by verify and check-tasks. No worktree, no git change.
export_parent() { # export_parent <dir>
  mkdir -p "$1"
  git -C "$ROOT" archive "$FIX^" backend | tar -x -C "$1"
}

overlay_tests() { # overlay_tests <dir>: put the reference test files from FIX into the folder
  local f
  for f in $TEST_FILES; do
    mkdir -p "$1/$(dirname "$f")"
    git -C "$ROOT" show "$FIX:$f" > "$1/$f"
  done
}

run_reference() { # run_reference <dir>: runs the reference test; prints PASS or FAIL
  if (cd "$1/backend" && go test "$PKG" -run "^($TEST_RUN)\$" -count=1 >"$1/reference.log" 2>&1); then echo PASS; else echo FAIL; fi
}

cmd_check_tasks() {
  local ids=${1:-} t d before after bad=0
  [ -n "$ids" ] || ids=$(find "$KIT/tasks" -name '*.env' -exec basename {} .env \; | sort | paste -sd, -)
  for t in ${ids//,/ }; do
    load_task "$t"
    d=$(mktemp -d)
    export_parent "$d"; overlay_tests "$d"; before=$(run_reference "$d")
    git -C "$ROOT" show "$FIX:$SRC" > "$d/$SRC"; after=$(run_reference "$d")
    if [ "$before" = FAIL ] && [ "$after" = PASS ]; then echo "$t ok (fails before the fix, passes after)"
    else echo "$t BAD: before=$before after=$after (see $d/reference.log)"; bad=1; continue; fi
    rm -rf "$d"
  done
  return $bad
}

cmd_plan() {
  [ $# -eq 3 ] || die "usage: plan <skill> <task,task> <n>"
  local skill=$1 tasks=$2 n=$3 t r i=0 order
  [ -d "$ROOT/.agents/skills/$skill" ] || die "no skill $skill"
  mkdir -p "$OUT"
  [ ! -e "$OUT/KEY.tsv" ] || die "$OUT/KEY.tsv exists: finish or delete the earlier evaluation first"
  order=$(for t in ${tasks//,/ }; do
    load_task "$t"
    # shellcheck disable=SC2153  # SKILL is set by load_task
    [ "$SKILL" = "$skill" ] || die "task $t is for skill $SKILL, not $skill"
    for r in $(seq "$n"); do echo "$t with $r"; echo "$t without $r"; done
  done | awk 'BEGIN { srand() } { print rand() "\t" $0 }' | sort -n | cut -f2-)
  printf 'run\ttask\tarm\n' > "$OUT/KEY.tsv"
  while read -r t arm _; do i=$((i + 1)); printf 'r%02d\t%s\t%s\n' "$i" "$t" "$arm" >> "$OUT/KEY.tsv"; done <<< "$order"
  echo "Run in this order (the arm is in $OUT/KEY.tsv; the person scoring must not open it):"
  awk -F'\t' 'NR>1 {print $1 "  " $2}' "$OUT/KEY.tsv"
}

cmd_setup() {
  [ $# -eq 1 ] || die "usage: setup <run>"
  local run=$1 task arm wt
  IFS=$'\t' read -r task arm < <(key_row "$run")
  load_task "$task"
  wt="$OUT/wt/$run"
  [ ! -e "$wt" ] || die "$wt already exists"
  git -C "$ROOT" worktree add --detach "$wt" "$FIX^" >/dev/null
  [ -d "$wt/.agents/skills/$SKILL" ] || die "skill $SKILL does not exist at $FIX^"
  [ "$arm" = without ] && rm -rf "$wt/.agents/skills/$SKILL"
  mkdir -p "$OUT/runs/$run"
  cat > "$OUT/runs/$run/runlog.md" <<EOF
# $run — fill in by hand (the arm is deliberately not written here)
- agent and model:
- started / finished:
- tokens in / out as the tool reports them (or "not reported"):
- cost as the tool reports it (or "not reported"):
- transcript saved as transcript.txt in this folder (yes/no):
- anything odd (stalls, asked a question, hit a limit):
EOF
  echo "worktree: $wt"
  echo "next: skill-eval.sh prompt $run   then give that text to the agent, working in $wt"
}

cmd_prompt() {
  [ $# -eq 1 ] || die "usage: prompt <run>"
  local run=$1 task arm
  IFS=$'\t' read -r task arm < <(key_row "$run")
  sed "s|{{WORKTREE}}|$OUT/wt/$run|g" "$KIT/preamble.md"
  cat "$KIT/tasks/$task.prompt.md"
}

cmd_guard() {
  echo "branch: $(git -C "$ROOT" rev-parse --abbrev-ref HEAD)  commit: $(git -C "$ROOT" rev-parse --short HEAD)"
  echo "--- status of the main checkout (must be unchanged by a batch)"; git -C "$ROOT" status --short
  echo "--- worktrees"; git -C "$ROOT" worktree list
}

cmd_collect() {
  [ $# -eq 1 ] || die "usage: collect <run>"
  local run=$1 task arm wt r="$OUT/runs/$1"
  IFS=$'\t' read -r task arm < <(key_row "$run"); load_task "$task"
  wt="$OUT/wt/$run"; [ -d "$wt" ] || die "no worktree $wt"
  mkdir -p "$r"
  git -C "$wt" add -N . 2>/dev/null   # include new files in the diff; this worktree is disposable
  git -C "$wt" diff HEAD --binary > "$r/agent.diff"
  git -C "$wt" status --short > "$r/status.txt"
  [ -f "$r/score.md" ] || { echo "# $run score sheet"; echo; sed '1,/^$/d' "$KIT/skills/$SKILL.checks.md"; } > "$r/score.md"
  echo "saved $r/agent.diff ($(wc -l < "$r/agent.diff") lines). Also save the agent's transcript as $r/transcript.txt."
}

cmd_verify() {
  [ $# -eq 1 ] || die "usage: verify <run>"
  local run=$1 task arm d res r="$OUT/runs/$1"
  IFS=$'\t' read -r task arm < <(key_row "$run"); load_task "$task"
  [ -f "$r/agent.diff" ] || die "run 'collect $run' first"
  d=$(mktemp -d); export_parent "$d"
  if [ -s "$r/agent.diff" ]; then
    (cd "$d" && git apply --include='backend/*' "$r/agent.diff") || { echo "FAIL (the diff does not apply)" | tee "$r/verify.txt"; return 0; }
  fi
  overlay_tests "$d"
  res=$(run_reference "$d"); echo "$res" | tee "$r/verify.txt"
  cp "$d/reference.log" "$r/reference.log"; rm -rf "$d"
}

cmd_report() {
  [ -f "$OUT/KEY.tsv" ] || die "no $OUT/KEY.tsv"
  local run task arm v c
  { printf 'run\ttask\tarm\treference\tchecks_ticked\n'
  while IFS=$'\t' read -r run task arm; do
    [ "$run" = run ] && continue
    v=$(cat "$OUT/runs/$run/verify.txt" 2>/dev/null || echo -)
    c=$(grep -c '^- \[x\]' "$OUT/runs/$run/score.md" 2>/dev/null || true)
    printf '%s\t%s\t%s\t%s\t%s\n' "$run" "$task" "$arm" "$v" "${c:-0}"
  done < "$OUT/KEY.tsv"; } | tee "$OUT/report.tsv"
  echo "--- per arm (runs, reference passes, checks ticked)"
  awk -F'\t' 'NR>1 {n[$3]++; if ($4 ~ /^PASS/) p[$3]++; c[$3]+=$5} END {for (a in n) printf "%s: runs=%d reference_pass=%d checks=%d\n", a, n[a], p[a], c[a]}' "$OUT/report.tsv"
}

cmd_cleanup() {
  [ $# -eq 1 ] || die "usage: cleanup <run|all>"
  local w
  if [ "$1" = all ]; then
    for w in "$OUT"/wt/*; do [ -d "$w" ] && git -C "$ROOT" worktree remove --force "$w"; done
  else
    git -C "$ROOT" worktree remove --force "$OUT/wt/$1"
  fi
  git -C "$ROOT" worktree prune
}

cmd=${1:-}; [ $# -gt 0 ] && shift
case "$cmd" in
  check-tasks) cmd_check_tasks "$@" ;;
  plan) cmd_plan "$@" ;;
  setup) cmd_setup "$@" ;;
  prompt) cmd_prompt "$@" ;;
  guard) cmd_guard ;;
  collect) cmd_collect "$@" ;;
  verify) cmd_verify "$@" ;;
  report) cmd_report ;;
  cleanup) cmd_cleanup "$@" ;;
  *) sed -n '2,23p' "$0"; exit 2 ;;
esac
