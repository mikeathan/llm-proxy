# Docs and Agent Harness Restructure

**Status:** proposed (2026-10-25; re-baselined 2026-10-26 on `main` 76a87f3) — plan only; no file has been moved yet
**Related:** `AGENTS.md` (operating contract), `CONSTITUTION.md` II.13 (prompts SSOT, unaffected), `docs/SPEC-change-management.md`
**Adjacent (not superseded):** `agents-md-layering-guardrails.md` covers the *product* AGENTS.md layer the assistant reads at run time; this plan covers the *repo* agent harness and docs tree.

---

## 1. Why

`docs/` is the repo's long-term memory (work done, things learned, future work), and it has outgrown its shape.
Measured on `main` 76a87f3 (v0.12.0, 2026-10-26; the first draft was measured on 70944ca, and 11 PRs — #137–#145, the network-audit work — merged since; every figure below is re-measured, and §9 has the commands to refresh them):

| Finding | Evidence |
|---|---|
| The live tracker mixes finished and open work | `docs/PLANS/README.md` is 30 KB and still growing (+2 rows in the last 11 PRs); its "Active Plans" table lists 31 `complete` plans; "Remaining Work" (meant to hold only non-complete plans) contains two `complete` rows (Run pipeline cleanup; Agent OS Sandboxing, `complete (pending Linux-CI)`) |
| Finished plans stay in the active tree | `docs/PLANS/` is 2.2 MB (53 plan files already in `ARCHIVE/`); `cross-cutting/frontend-redesign-retro.md` is 128 KB, `agent-os-sandboxing.md` 95 KB, both `complete`, not in `ARCHIVE/`. The newest live plan, `cross-cutting/network-audit.md` (`partial`, Phases 0–6 merged), is already 65 KB, so the size problem recurs on every large feature |
| One doc does four jobs | `docs/architecture.md` is 64 KB: directory map, contracts, checklists, pitfalls |
| Learnings are chronological, not retrievable | `docs/audits/` holds 25 files with no per-subsystem ledger; decisions live inside plans; no ADRs |
| Skills are heavy | 19 skills, 3 152 lines; 7 over 235 lines (`clean-code` 382, `assistant-ui-chat` 335, `agent-loop` 319, `event-streaming-patterns` 261, `automation` 259, `lifecycle-events` 236, `testing-guide` 194); none has a `references/` folder |
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
3. Be harness-neutral. The owner uses opencode, pi agent, command code, codex, and Claude Code (Claude models in Claude Code only; the others run any model). `AGENTS.md` is the canonical contract and names no tool or vendor. The only Claude-named files are `CLAUDE.md` (a pointer) and the `.claude/skills` symlink Claude Code needs to discover skills.
4. Keep the harness check authoritative: every move below updates `scripts/check-agent-harness.sh`, `.githooks/pre-commit`, and CI in the same phase.
5. Nothing is lost: old paths either keep a one-line stub or every inbound link is rewritten; history stays in git.

**Non-goals:** changing code behavior, SPEC contents, `CONSTITUTION.md` text, the product's embedded prompts, or the backend/frontend rule *content* (only how it is loaded).

## 3. Harness compatibility contract

Principle: **one canonical, vendor-neutral source; the only vendor-named file is a pointer.**

| Harness (owner uses all five) | Reads | Plan |
|---|---|---|
| opencode, pi agent, command code, codex | `AGENTS.md` (assumed; verify each in Phase 0). Skills from `.agents/skills/` where supported. | Nothing tool-specific is added. They get `AGENTS.md` and `.agents/skills/` as they are. |
| Claude Code (Claude models only) | `CLAUDE.md`, not `AGENTS.md`; skills via `.claude/skills` | `CLAUDE.md` stays ≤ 20 lines: `@AGENTS.md` import + Claude-only notes. `.claude/skills` stays a symlink to `.agents/skills` (the one other Claude-named path). |

**Decision: no further Claude-named files.** The earlier idea of `.claude/rules/*.md` path-scoped loaders and a `.claude/settings.json` hook is dropped. Path-scoped loading comes instead from nested `AGENTS.md` files (§5.2), which is the cross-harness convention, and the harness check runs from the git hook and CI, which are tool-neutral. `GEMINI.md` and Copilot adapters are not wanted (not used).

**No vendor references outside the allow-list.** Neutral surfaces say "the agent" or "the harness", never a vendor or model name. Allow-list (the only places the string `claude` may appear): `CLAUDE.md`, the `.claude/skills` symlink, `scripts/check-agent-harness.sh` and `.githooks/pre-commit` (they must name the adapter to check it). Product code, tests, plans, audits and archives are out of scope for this rule: Claude appears there as provider/model data or history, not as an instruction target. A harness check enforces it (§7).

Assumptions to **verify in Phase 0 before relying on them** (from general knowledge, not checked against the installed versions):
- Each of opencode, pi agent, command code and codex reads root `AGENTS.md`, and which of them honor a nested `AGENTS.md` (nearest wins). If one does not, the root `AGENTS.md` "Before coding" step already tells the agent which rule file to read, so nothing breaks; only the automatic path scoping is lost for that tool.
- Which of them discover `.agents/skills/` natively, and if any needs its own skills directory, whether a symlink is accepted (any such directory would be a new tool-named path; decide then whether the owner accepts it or the tool is told the path via `AGENTS.md`).
- Claude Code resolves `@AGENTS.md` imports from `CLAUDE.md` and follows the `.claude/skills` symlink (the symlink is already relied on today).
- Symlinks: Windows checkouts may materialise symlinks as text files. If any contributor is on Windows, replace `.claude/skills` with a generated copy (harness check verifies byte equality).

Adapter rule: `CLAUDE.md` holds no unique content except the Claude-only notes block; the harness check fails otherwise.

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
guardrails, assistant-ui, discovery/admin-ui, prompt-profiles, reminders, autonomy, mcp, data-layout, plus the two added since the first draft: **network-audit** (`core/netaudit`, the `network_scan` tool, the Security auditor agent; plan `cross-cutting/network-audit.md`, no SPEC of its own yet — it cites SPEC-001/003/006/007/009/012) and **tool-registry** (`core/toolregistry`, the one tool catalog; documented in `architecture.md` and the network-audit plan, no SPEC). A hub may name "no SPEC yet" and link the governing plan instead.

**Lifecycle rule ("harvest"):** a plan is only `complete` after it is harvested:
1. architecture / SPEC updated to the as-built state;
2. an ADR written for each real decision (alternatives, reason);
3. lessons and dead ends appended to `learnings/<subsystem>.md` (one line each, linking the evidence);
4. the plan reduced to an archive stub: title, outcome, PR links, pointers to 1–3.
Git history retains the full text.

**Plan hygiene:** a live plan is a front page (status, scope, phases with acceptance, Remaining Work) kept under ~15 KB; longer designs split by phase. Merged phases are harvested out of a `partial` plan as they land (so `network-audit.md`, 65 KB with Phases 0–6 merged, shrinks to its open scope), not only when the whole plan completes.

**Learnings entry shape** (frontmatter on audits/learnings docs): `subsystem`, `tags`, `question`, `result` (worked | failed | inconclusive), `links`.
The per-subsystem ledger line format: `- tried <X> → <result>; why: <Y>; evidence: <link>`. Agents read the ledger before proposing an approach.

**Memory rule (written into `AGENTS.md`):** the repo is the shared, durable memory. A harness's own auto-memory is per-user/per-machine and is a cache for personal preferences only. Test for repo docs: *would a fresh clone need this?* If yes → pitfall, ADR, learning, or SPEC — not auto-memory.

## 5. Target agent harness structure

### 5.1 Always-loaded files
- `AGENTS.md`: keep contract; reorder so Commands and "Before coding" are first; replace the vague Memory section with the rule above; point at `docs/subsystems/` hubs for the "classify first" step; drop anything duplicated in `CONTRIBUTING.md`.
- `CLAUDE.md`: unchanged shape (import + Claude-only notes: the skills symlink, how auto-memory maps to the memory rule); updated only for new paths. It is the only place that names Claude.
- Strip any vendor/model wording from every neutral file (today: one sample line in `.agents/skills/assistant-ui-chat/SKILL.md`, "Claude Haiku 5.5", becomes a neutral example model name). `AGENTS.md` wording stays generic ("the agent", "the harness").

### 5.2 Rules (`.agents/rules/`)
Keep the two full files as canonical. Add nested `backend/AGENTS.md` and `frontend/AGENTS.md`, each ≤ 15 lines: the must-not-break core plus a link to the full rule file. Tools that honor nested `AGENTS.md` get path-scoped loading; every tool, including those that do not, still gets the root "Before coding" instruction. No vendor-named loader files. The harness check verifies each nested file's link target exists, and the product-template rename in Phase 6 must land first so the nested files are not confused with it.

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
- Confirm the assumptions in §3 for each of opencode, pi agent, command code, codex and Claude Code (root and nested `AGENTS.md`, `.agents/skills/` discovery, symlink handling) and whether any contributor is on Windows.
- Confirm the allow-list for the `claude` string (§3). The harness stays vendor-neutral: no GEMINI/Copilot/Claude rules/settings files.
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
- Before moving anything: `git fetch origin main`, merge/rebase onto it, re-run the §9 measurements and update §1 (the tree moved 11 PRs between the first draft and this revision).
- Harvest then archive completed plans, largest first (`frontend-redesign-retro`, `agent-os-sandboxing`, `xdg-config-data-relocation`, `search-tool-calling`, …); for the `partial` `network-audit.md`, harvest only its merged phases (0–6, PRs #137–#144) and keep the open scope (its §13) live: for each, perform the §4 harvest and leave a stub in `ARCHIVE/`. Move files with `git mv` so history follows.
- Fix the `complete` row in Remaining Work.
- Update check 7 and 10 to the new layout (see §7); turn on the "no `complete` row in live tables" check.
- **Accept when:** live `PLANS/README.md` < 10 KB; `docs/PLANS/` (excluding ARCHIVE) contains only non-complete plans; every archived plan has a stub with harvest links; harness PASS.

### Phase 4 — Architecture split and subsystem hubs
- Split `docs/architecture.md` into `docs/architecture/{map,contracts,checklists,pitfalls}.md`; keep `architecture.md` as the entry point. Give pitfalls stable IDs (`PIT-001…`) and keep the existing numbering as aliases so references like "pitfall 12" stay resolvable.
- Rewrite references in `AGENTS.md`, `CONTRIBUTING.md`, skills, `docs/INDEX.md`, rules, and the harness (`task-planning` and `documentation-stewardship` skills both cite `docs/architecture.md` sections by name).
- Create one hub per subsystem listed in §4 (including network-audit and tool-registry). Pilot with **memory** first and review before the rest. `architecture.md`'s package map now lists `core/netaudit` and `core/toolregistry`; carry both into `architecture/map.md`.
- **Accept when:** each SPEC has a hub; hubs list SPEC, architecture section, ADRs, open plans, ledger, skill; harness check validates hub links; harness PASS.

### Phase 5 — Learnings and decisions
- Move or index `docs/audits/*` under the learnings catalog (keep raw evidence in `audits/`, add the frontmatter fields from §4 to each; `git mv` only if the owner prefers a single folder).
- Seed `learnings/<subsystem>.md` ledgers from existing audits and from "Lessons" in archived plans; seed ADRs for decisions already recorded in plans (start with those cited by SPECS).
- Close or re-status `audits/codebase-audit-report.md` (currently `active`).
- **Accept when:** every audit has the frontmatter; every subsystem hub links a non-empty ledger; the first ADR batch is listed in `decisions/README.md`.

### Phase 6 — Harness adapters and stray files
- Do the rename first (the nested files below must not be confused with it). Rename `backend/internal/core/assistant/prompts/AGENTS.md` → `AGENTS.template.md` and update the `//go:embed AGENTS.md` at `backend/internal/core/assistant/prompts/templates.go:17` and every Go reference that names the file (`rg -n 'AGENTS.md' backend --include=*.go`: e.g. `prompts/default_agents_md_test.go`, `system_prompt.go`, `models/workspace.go` — only the *embedded default* file is renamed, not the per-workspace `AGENTS.md` those files also mention). Run `cd backend && go build ./... && go test ./...`.
- Fold `docs/superpowers/` into `docs/PLANS/autonomy/` (reminders design) and remove the folder; add `docs/api-reference.md` to `docs/INDEX.md`.
- Then `AGENTS.md` reorder and memory-rule rewrite (§5.1, vendor-neutral wording); `CLAUDE.md` update; add nested `backend/AGENTS.md` and `frontend/AGENTS.md` (§5.2); neutralise the one sample line in `assistant-ui-chat/SKILL.md`.
- No `.claude/rules`, no `.claude/settings.json`: enforcement stays in the git hook and CI.
- **Accept when:** a fresh read of `AGENTS.md` shows commands and "Before coding" in the first screen; `rg -il claude` over the neutral surfaces (§7 check) returns only the allow-list; build/tests green; harness PASS.

### Phase 7 — Turn warnings into gates
- Flip the Phase 1 warn-only checks to failing; run the full gate; write the final "as-built" summary into this plan and mark it `complete` after harvesting it with its own §4 rule.

## 7. Harness script, hook, and CI changes (checklist)

`scripts/check-agent-harness.sh`
- **DOCS list:** add `docs/architecture/*.md`, `docs/subsystems/*.md`, `docs/decisions/*.md`, `docs/learnings/*.md`, `docs/PLANS/ARCHIVE/README.md`; add skills' `references/*.md` to the link/path scans.
- **[1] skill frontmatter:** keep; add warning when `last_reviewed` is older than 60 days or missing; add cap `SKILL.md` ≤ 150 lines (use a ratchet allow-list during Phase 2, remove it in Phase 7).
- **[3] links:** extend to `references/` files and the nested `backend/AGENTS.md` / `frontend/AGENTS.md` targets.
- **[6]:** INDEX skills table check stays; consider generating the table from skill files to remove the hand-kept duplicate.
- **[7]:** archive stubs are indexed by `ARCHIVE/README.md` instead of being skipped; add catalogs for `decisions/`, `learnings/`, `subsystems/`.
- **[8]:** point the template check at `testing-guide/references/automation-templates.md`.
- **[10]:** read status from the live `PLANS/README.md` table only; fail if a live-table row is `complete`.
- **New checks:** (a) `CLAUDE.md` is ≤ 20 lines, imports `@AGENTS.md`, and nothing else in the repo duplicates `AGENTS.md` content; (a2) **vendor-neutrality:** case-insensitive `claude` is absent from the neutral surfaces (`AGENTS.md`, `CONTRIBUTING.md`, `README.md`, `docs/INDEX.md`, `docs/architecture*`, `docs/subsystems/`, `docs/decisions/`, `docs/learnings/`, `docs/SPECS/`, `docs/guides/`, `.agents/**`), with the §3 allow-list as the only exemptions; the same check could cover other vendor names if the owner wants; (b) every `docs/subsystems/*.md` links an existing SPEC, or states `no SPEC yet` and links an existing plan; (c) ADR files are numbered, have `status`, and are listed in `decisions/README.md`; (d) archived plans have harvest links; (e) the product template `AGENTS.template.md` exists, and the only `AGENTS.md` files under `backend/` and `frontend/` are the two nested pointer files.
- Keep POSIX-bash + coreutils only, as today.

`.githooks/pre-commit` (line 29 regex): add `CLAUDE\.md`, `(backend|frontend)/AGENTS\.md`, `docs/(architecture|subsystems|decisions|learnings)/`, and keep `AGENT\.md`.

`.github/workflows/ci.yml` (`agent-harness` job, line 50): the job runs the script as-is; add path filters only if the workflow has them. No new job.

`CONTRIBUTING.md` (line 36, line 78 hooks section): update the file lists and mention the new layout.

## 8. Risks and mitigations

| Risk | Mitigation |
|---|---|
| Dead links after moves (plans cite files, skills cite `architecture.md` sections) | Move with `git mv`; keep `architecture.md` as entry point; run the harness (check 3) after every phase; PLANS/audits are not link-scanned today, so add a one-time `rg` pass for old paths in the Phase 3 and 4 PRs |
| A harness silently stops loading something | Phase 0 verification per tool; the root `AGENTS.md` "Before coding" step is the fallback for tools that ignore nested files; `CLAUDE.md` is a pointer, so Claude Code falls back to `AGENTS.md` |
| A tool the owner uses wants its own skills directory or config file | Not assumed. Decide in Phase 0; prefer telling the tool the path through `AGENTS.md` over adding a tool-named path |
| Harvest loses nuance from big plans | Stubs link the pre-archive commit SHA; git history keeps the full text; harvest is reviewed per plan |
| Skill split hides content from agents | "Read next" table in every `SKILL.md` names each reference and when to open it; the check requires every reference be linked |
| Symlink breakage on Windows | Decision in Phase 0; fallback is a generated copy verified by the harness |
| Concurrent work: the repo merged 11 PRs (many touching `docs/`, `PLANS/README.md`, skills and `architecture.md`) while this plan was a draft, so moves will conflict | One phase per PR, merged quickly; rebase and re-measure immediately before each move (Phase 3 first step); do skill splits (Phase 2) one skill per PR so a concurrent skill edit conflicts in at most one file; agree a short freeze on the file being moved, or land content edits to a moved file in the same PR |
| Scope creep into code | Only Phase 6 touches Go (one rename); everything else is docs/scripts |

## 9. Verification (per phase and at the end)

```bash
# refresh the §1 figures after any rebase
wc -c docs/PLANS/README.md docs/architecture.md; du -sh docs/PLANS docs/audits docs/SPECS
awk '/^## Active Plans/,/^## Remaining Work/' docs/PLANS/README.md | grep -c '| complete'   # live-table complete rows (31 at 76a87f3)
awk '/^## Remaining Work/,0' docs/PLANS/README.md | grep '^| complete'                    # must be empty after Phase 3
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
