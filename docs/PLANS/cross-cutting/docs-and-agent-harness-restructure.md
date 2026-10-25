# Docs and Agent Harness Restructure

**Status:** proposed (2026-10-25) — plan only; no file has been moved yet
**Related:** `AGENTS.md` (operating contract), `CONSTITUTION.md` II.13 (prompts SSOT, unaffected), `docs/SPEC-change-management.md`
**Adjacent (not superseded):** `agents-md-layering-guardrails.md` covers the *product* AGENTS.md layer the assistant reads at run time; this plan covers the *repo* agent harness and docs tree.

---

## 1. Why

`docs/` is the repo's long-term memory (work done, things learned, future work), and it has outgrown its shape.
Measured on branch `HEAD` 70944ca (all numbers from this repo, 2026-10-25):

| Finding | Evidence |
|---|---|
| The live tracker mixes finished and open work | `docs/PLANS/README.md` is 29 KB; its "Active Plans" table lists ~25 `complete` plans; "Remaining Work" (meant to hold only non-complete plans) contains a `complete` row (Run pipeline cleanup) |
| Finished plans stay in the active tree | `docs/PLANS/` is 2.1 MB; `cross-cutting/frontend-redesign-retro.md` is 128 KB, `agent-os-sandboxing.md` 95 KB, both `complete`, not in `ARCHIVE/` |
| One doc does four jobs | `docs/architecture.md` is 64 KB: directory map, contracts, checklists, pitfalls |
| Learnings are chronological, not retrievable | `docs/audits/` holds 25 files with no per-subsystem ledger; decisions live inside plans; no ADRs |
| Skills are heavy | 19 skills, 3 148 lines; 7 over 235 lines (`clean-code` 382, `assistant-ui-chat` 335, `agent-loop` 319, `event-streaming-patterns` 261, `automation` 254, `lifecycle-events` 236, `testing-guide` 195); none has a `references/` folder |
| Stray / orphaned files | `backend/internal/core/assistant/prompts/AGENTS.md` is a *product* template that agent tools will read as instructions for that directory; `docs/superpowers/` (1 plan + 1 spec) is in no catalog and no harness scan; `docs/api-reference.md` is linked from `README.md` but absent from `docs/INDEX.md` |
| Staleness is unenforced | `last_reviewed` frontmatter exists but nothing checks it; oldest are 2026-07-11 (`connector-patterns`, `llamacpp-setup`) |
| Rules are loaded by instruction, not by path | `AGENTS.md` → "Before coding" asks the agent to remember to read `.agents/rules/*` |

What already works and must be kept: the instruction hierarchy, `CLAUDE.md` as a thin `@AGENTS.md` import (18 lines),
`.claude/skills` → `.agents/skills` symlink, one authoritative catalog per doc kind, SPEC frontmatter, and
`scripts/check-agent-harness.sh` (10 checks, wired into CI `agent-harness` job and `.githooks/pre-commit`).

## 2. Goals and non-goals

**Goals**
1. Separate knowledge by lifecycle: *architecture/SPECS* (current truth), *decisions* (immutable), *roadmap/plans* (future, short-lived), *learnings* (what we tried), linked per subsystem.
2. Make every always-loaded file small; push depth into on-demand files (`SKILL.md` ≤ 150 lines + `references/`).
3. Work across harnesses: `AGENTS.md` stays the canonical contract; every other harness file is a thin pointer.
4. Keep the harness check authoritative: every move below updates `scripts/check-agent-harness.sh`, `.githooks/pre-commit`, and CI in the same phase.
5. Nothing is lost: old paths either keep a one-line stub or every inbound link is rewritten; history stays in git.

**Non-goals:** changing code behavior, SPEC contents, `CONSTITUTION.md` text, the product's embedded prompts, or the backend/frontend rule *content* (only how it is loaded).

## 3. Harness compatibility contract

Principle: **one canonical source, thin adapters.** Anything a harness needs beyond `AGENTS.md` is a pointer, never a copy.

| Harness | Reads | Plan |
|---|---|---|
| Codex, Cursor, Copilot agent, Windsurf, Aider-style tools using the AGENTS.md convention | `AGENTS.md` (nearest file wins for nested ones) | Canonical file. Keep ≤ ~120 lines of always-needed content. |
| Claude Code | `CLAUDE.md` (not `AGENTS.md`), `.claude/skills`, `.claude/rules/*.md`, `.claude/settings.json` | `CLAUDE.md` stays ≤ 20 lines: `@AGENTS.md` + Claude-only notes. Add path-scoped `.claude/rules/*.md` as thin loaders (see §5). |
| Gemini CLI | `GEMINI.md` by default; configurable via `context.fileName` | Add root `GEMINI.md` = `@AGENTS.md` import only **if** the team uses it (decide in Phase 0). |
| GitHub Copilot (IDE) | `.github/copilot-instructions.md` | Same decision: pointer file only, if used. |
| Skills (open Agent Skills format) | `SKILL.md` with `name` + `description` frontmatter; optional `references/`, `scripts/`, `assets/` | Keep `.agents/skills/` canonical; `.claude/skills` stays a symlink. |

Assumptions to **verify in Phase 0 before relying on them** (from general knowledge, not checked against installed versions):
- Claude Code supports `.claude/rules/*.md` with a `paths:` frontmatter glob for conditional loading.
- Claude Code resolves `@path` imports from `CLAUDE.md`, and a symlinked `.claude/skills` is discovered (the latter is already relied on today).
- Which of Gemini CLI / Copilot / Cursor the owner actually uses.
- Symlinks: Windows checkouts may materialise symlinks as text files. If any contributor is on Windows, replace `.claude/skills` and any new link with a harness-generated copy (harness check verifies byte equality) instead of a symlink.

Rules for every adapter file: no unique content; the harness check fails if an adapter contains anything other than an import/pointer plus an allow-listed "Claude-specific notes" block.

## 4. Target docs structure

```
docs/
  INDEX.md                  router (subsystem-first, then doc kind)
  subsystems/<name>.md      NEW hub per subsystem: SPEC · architecture section · ADRs · open plans · learnings ledger · skill
  architecture/             SPLIT from architecture.md: map.md, contracts.md, checklists.md, pitfalls.md
  architecture.md           kept as a short entry point (links to the split files) so existing links resolve
  SPECS/                    unchanged
  decisions/                NEW ADR-NNNN-<slug>.md (template + README catalog); immutable once accepted
  PLANS/                    LIVE only: README.md = short tracker; one short file per open plan
  PLANS/ARCHIVE/            condensed stubs for completed plans + ARCHIVE/README.md index
  learnings/                NEW (from audits/): README catalog + per-subsystem ledgers (learnings/<subsystem>.md)
  audits/                   kept as the raw-evidence layer; README gains required frontmatter fields (see §6)
  guides/                   unchanged (operator how-tos)
```

Subsystem list for hubs (from the existing SPEC/plan folders): agent-loop, memory, orchestrator, automation, communication,
guardrails, assistant-ui, discovery/admin-ui, prompt-profiles, reminders, autonomy, mcp, data-layout.

**Lifecycle rule ("harvest"):** a plan is only `complete` after it is harvested:
1. architecture / SPEC updated to the as-built state;
2. an ADR written for each real decision (alternatives, reason);
3. lessons and dead ends appended to `learnings/<subsystem>.md` (one line each, linking the evidence);
4. the plan reduced to an archive stub: title, outcome, PR links, pointers to 1–3.
Git history retains the full text.

**Plan hygiene:** a live plan is a front page (status, scope, phases with acceptance, Remaining Work) kept under ~15 KB; longer designs split by phase.

**Learnings entry shape** (frontmatter on audits/learnings docs): `subsystem`, `tags`, `question`, `result` (worked | failed | inconclusive), `links`.
The per-subsystem ledger line format: `- tried <X> → <result>; why: <Y>; evidence: <link>`. Agents read the ledger before proposing an approach.

**Memory rule (written into `AGENTS.md`):** the repo is the shared, durable memory. A harness's own auto-memory is per-user/per-machine and is a cache for personal preferences only. Test for repo docs: *would a fresh clone need this?* If yes → pitfall, ADR, learning, or SPEC — not auto-memory.

## 5. Target agent harness structure

### 5.1 Always-loaded files
- `AGENTS.md`: keep contract; reorder so Commands and "Before coding" are first; replace the vague Memory section with the rule above; point at `docs/subsystems/` hubs for the "classify first" step; drop anything duplicated in `CONTRIBUTING.md`.
- `CLAUDE.md`: unchanged shape (import + Claude-only notes), updated for `.claude/rules` and the new paths.
- Optional adapters (`GEMINI.md`, `.github/copilot-instructions.md`): only if Phase 0 says they are used.

### 5.2 Rules (`.agents/rules/`)
Keep the two full files as canonical. Add `.claude/rules/go.md` (`paths: ["backend/**"]`) and `.claude/rules/vue.md`
(`paths: ["frontend/**"]`), each ≤ 15 lines: the must-not-break core plus a link to the full rule file. Other harnesses keep using
the AGENTS.md "Before coding" instruction. The harness check verifies each adapter's link target exists.

### 5.3 Skills: slim `SKILL.md`, move depth under the skill folder
Standard layout (one level deep, as the Agent Skills format expects):

```
.agents/skills/<name>/
  SKILL.md            ≤ 150 lines: when to use, the 5–15 rules that matter, a "Read next" table (reference file → when)
  references/<topic>.md   detail; any file > 100 lines starts with a contents list
  scripts/            only for deterministic helpers (none required by this plan)
```
`SKILL.md` keeps `name`, `description` (what + when, trigger-first), `last_reviewed`. Detail moves verbatim (no rewording) except where a heading becomes a file title.

Planned splits (line numbers are `##` headings today):

| Skill (lines) | Stays in SKILL.md | Moves to `references/` |
|---|---|---|
| `clean-code` (382) | §0 Mindset, §1–2 condensed, §13 Applying This in This Repo, pointer table | `names-functions.md` (§1–2), `comments-formatting.md` (§3–4), `objects-errors.md` (§5–6), `tests-design.md` (§7–11), `smells-checklist.md` (§12, the 95-line checklist) |
| `assistant-ui-chat` (335) | Layout summary, Common Pitfalls, pointer table | `message-builder.md`, `sse-timing-and-events.md` (inactivity, ordering, connection timing, dedup), `segments-and-turns.md` (segments, turn grouper, natural completion), `animation-and-scroll.md`, `webhook-sessions.md` |
| `agent-loop` (319) | Execution Flow, Key Constants, Important Gotchas, pointer table | `sieve-and-budget.md`, `stuck-and-spiral-detection.md`, `completion-and-control-plane.md`, `reasoning-gbnf-fallback.md`, `tool-batches-and-data-flow.md`, `execute-plan.md` |
| `event-streaming-patterns` (261) | Architecture, Key Files | `agent-events.md`, `observers-and-guardrail-flow.md`, `sse-composables-and-dedup.md`, `heartbeat-cleanup.md` |
| `automation` (254) | Architecture, Execution Lifecycle, Important Gotchas | `components.md`, `result-delivery.md`, `run-output-and-overrides.md`, `templates.md`, `run-lane.md` |
| `lifecycle-events` (236) | Overview, Event Contract, Phases | `retry-and-failure.md`, `refresh-and-resume.md`, `frontend-handling.md`, `known-issues-and-compat.md`, `webhook-flow.md` |
| `testing-guide` (195) | Test Types, Common Pitfalls, pointer table | `smoke-and-run-analysis.md`, `llm-test-doubles.md`, `record-replay.md`, `frontend-testing.md`, `automation-templates.md` (**harness check 8 must follow this file**) |
| `memory-system`, `connector-patterns`, `engineering-practices`, `debugging`, `llamacpp-setup`, `tdd-guide` | Review only; split if > 150 lines after the pass (`connector-patterns` outbound section is 120 lines → `references/outbound-connector.md`) | |
| Remaining small skills | No change | |

Also: tighten each `description` to *what it does + when to use it* (several currently only name the topic), and re-date `last_reviewed` for any skill whose content is touched.

## 6. Phases

Each phase ends with `./scripts/check-agent-harness.sh` → PASS and is its own commit/PR-sized unit. Phase 0 decides scope; later phases are independent except where noted.

### Phase 0 — Decide and verify (no moves)
- Confirm the assumptions in §3 (harnesses in use; `.claude/rules` `paths:` behavior; Windows contributors).
- Decide whether `GEMINI.md` / `.github/copilot-instructions.md` adapters are wanted.
- Record the answers in this plan (edit §3) and in one ADR (first file in `docs/decisions/`).
- **Accept when:** §3 has no unverified assumptions left; the owner approved the target structure in §4.

### Phase 1 — Skeleton and guardrails (additive only)
- Create `docs/decisions/` (README catalog + `TEMPLATE.md`), `docs/learnings/` (README + `TEMPLATE.md`), `docs/subsystems/` (README + hub template), `docs/PLANS/ARCHIVE/README.md`.
- Extend `scripts/check-agent-harness.sh` (see §7) with the new checks in **warn-only** mode so nothing fails yet.
- **Accept when:** harness PASS; new directories are in `docs/INDEX.md`; no existing file moved.

### Phase 2 — Skills slimming (largest context saving, lowest risk)
- Split in the order of size: `clean-code`, `assistant-ui-chat`, `agent-loop`, `event-streaming-patterns`, `automation`, `lifecycle-events`, `testing-guide`; then review the rest.
- One skill per commit. Move text verbatim; add the "Read next" table.
- Update check 8 (`testing-guide` template list) to scan `references/`; add the `SKILL.md` line cap and `references/` link checks as **failing** checks once all skills pass.
- **Accept when:** every `SKILL.md` ≤ 150 lines; all `references/*.md` linked from their `SKILL.md`; `wc -l` before/after table pasted into the PR; harness PASS.

### Phase 3 — Plans: split live from done
- Rewrite `docs/PLANS/README.md` as the short tracker (open plans only: status legend, one table, Remaining Work). Move the full completed-plan table to `docs/PLANS/ARCHIVE/README.md`.
- Harvest then archive completed plans, largest first (`frontend-redesign-retro`, `agent-os-sandboxing`, `xdg-config-data-relocation`, `search-tool-calling`, …): for each, perform the §4 harvest and leave a stub in `ARCHIVE/`. Move files with `git mv` so history follows.
- Fix the `complete` row in Remaining Work.
- Update check 7 and 10 to the new layout (see §7); turn on the "no `complete` row in live tables" check.
- **Accept when:** live `PLANS/README.md` < 10 KB; `docs/PLANS/` (excluding ARCHIVE) contains only non-complete plans; every archived plan has a stub with harvest links; harness PASS.

### Phase 4 — Architecture split and subsystem hubs
- Split `docs/architecture.md` into `docs/architecture/{map,contracts,checklists,pitfalls}.md`; keep `architecture.md` as the entry point. Give pitfalls stable IDs (`PIT-001…`) and keep the existing numbering as aliases so references like "pitfall 12" stay resolvable.
- Rewrite references in `AGENTS.md`, `CONTRIBUTING.md`, skills, `docs/INDEX.md`, rules, and the harness (`task-planning` and `documentation-stewardship` skills both cite `docs/architecture.md` sections by name).
- Create one hub per subsystem listed in §4. Pilot with **memory** first and review before the rest.
- **Accept when:** each SPEC has a hub; hubs list SPEC, architecture section, ADRs, open plans, ledger, skill; harness check validates hub links; harness PASS.

### Phase 5 — Learnings and decisions
- Move or index `docs/audits/*` under the learnings catalog (keep raw evidence in `audits/`, add the frontmatter fields from §4 to each; `git mv` only if the owner prefers a single folder).
- Seed `learnings/<subsystem>.md` ledgers from existing audits and from "Lessons" in archived plans; seed ADRs for decisions already recorded in plans (start with those cited by SPECS).
- Close or re-status `audits/codebase-audit-report.md` (currently `active`).
- **Accept when:** every audit has the frontmatter; every subsystem hub links a non-empty ledger; the first ADR batch is listed in `decisions/README.md`.

### Phase 6 — Harness adapters and stray files
- `AGENTS.md` reorder and memory-rule rewrite (§5.1); `CLAUDE.md` update.
- Add `.claude/rules/go.md` and `vue.md` (after Phase 0 verification); add optional adapters if approved.
- Rename `backend/internal/core/assistant/prompts/AGENTS.md` → `AGENTS.template.md` and update the `//go:embed AGENTS.md` at `backend/internal/core/assistant/prompts/templates.go:17` and every Go reference that names the file (`rg -n 'AGENTS.md' backend --include=*.go`: e.g. `prompts/default_agents_md_test.go`, `system_prompt.go`, `models/workspace.go` — only the *embedded default* file is renamed, not the per-workspace `AGENTS.md` those files also mention). Run `cd backend && go build ./... && go test ./...`.
- Fold `docs/superpowers/` into `docs/PLANS/autonomy/` (reminders design) and remove the folder; add `docs/api-reference.md` to `docs/INDEX.md`.
- Optional: minimal `.claude/settings.json` with a hook that runs the harness check (ask first: settings and hooks are a behavior change).
- **Accept when:** a fresh read of `AGENTS.md` shows commands and "Before coding" in the first screen; build/tests green; harness PASS.

### Phase 7 — Turn warnings into gates
- Flip the Phase 1 warn-only checks to failing; run the full gate; write the final "as-built" summary into this plan and mark it `complete` after harvesting it with its own §4 rule.

## 7. Harness script, hook, and CI changes (checklist)

`scripts/check-agent-harness.sh`
- **DOCS list:** add `docs/architecture/*.md`, `docs/subsystems/*.md`, `docs/decisions/*.md`, `docs/learnings/*.md`, `docs/PLANS/ARCHIVE/README.md`; add skills' `references/*.md` to the link/path scans.
- **[1] skill frontmatter:** keep; add warning when `last_reviewed` is older than 60 days or missing; add cap `SKILL.md` ≤ 150 lines (use a ratchet allow-list during Phase 2, remove it in Phase 7).
- **[3] links:** extend to `references/` files and `.claude/rules/*.md` targets.
- **[6]:** INDEX skills table check stays; consider generating the table from skill files to remove the hand-kept duplicate.
- **[7]:** archive stubs are indexed by `ARCHIVE/README.md` instead of being skipped; add catalogs for `decisions/`, `learnings/`, `subsystems/`.
- **[8]:** point the template check at `testing-guide/references/automation-templates.md`.
- **[10]:** read status from the live `PLANS/README.md` table only; fail if a live-table row is `complete`.
- **New checks:** (a) adapter files contain only pointers/imports; (b) every `docs/subsystems/*.md` links an existing SPEC; (c) ADR files are numbered, have `status`, and are listed in `decisions/README.md`; (d) archived plans have harvest links; (e) the product template `AGENTS.template.md` exists and no `AGENTS.md` exists under `backend/` or `frontend/`.
- Keep POSIX-bash + coreutils only, as today.

`.githooks/pre-commit` (line 29 regex): add `CLAUDE\.md`, `GEMINI\.md`, `\.claude/`, `docs/(architecture|subsystems|decisions|learnings)/`, and keep `AGENT\.md`.

`.github/workflows/ci.yml` (`agent-harness` job, line 50): the job runs the script as-is; add path filters only if the workflow has them. No new job.

`CONTRIBUTING.md` (line 36, line 78 hooks section): update the file lists and mention the new layout.

## 8. Risks and mitigations

| Risk | Mitigation |
|---|---|
| Dead links after moves (plans cite files, skills cite `architecture.md` sections) | Move with `git mv`; keep `architecture.md` as entry point; run the harness (check 3) after every phase; PLANS/audits are not link-scanned today, so add a one-time `rg` pass for old paths in the Phase 3 and 4 PRs |
| A harness silently stops loading something | Phase 0 verification; adapters are pointers, so worst case falls back to `AGENTS.md` |
| Harvest loses nuance from big plans | Stubs link the pre-archive commit SHA; git history keeps the full text; harvest is reviewed per plan |
| Skill split hides content from agents | "Read next" table in every `SKILL.md` names each reference and when to open it; the check requires every reference be linked |
| Symlink breakage on Windows | Decision in Phase 0; fallback is a generated copy verified by the harness |
| Scope creep into code | Only Phase 6 touches Go (one rename); everything else is docs/scripts |

## 9. Verification (per phase and at the end)

```bash
./scripts/check-agent-harness.sh                      # must PASS after every phase
wc -l .agents/skills/*/SKILL.md | sort -n | tail      # Phase 2: all ≤ 150
wc -c docs/PLANS/README.md docs/architecture.md       # Phase 3/4 size budgets
cd backend && go build ./... && go test ./...         # Phase 6 only (template rename)
```
Final review follows the `AGENTS.md` Pre-Completion Review, reported gate by gate.

## 10. Plan size note

This plan is ~20 KB, above the ~15 KB live-plan budget it proposes (§4). It is accepted as the specification of the restructure itself; Phase 3 splits it by phase if the budget becomes a gate before it completes.

## 11. Remaining Work

Everything. Next action is Phase 0 (owner decisions in §3), then Phase 1.
