# Skill evaluation: does a skill change what an agent does?

**Status:** proposed (2026-11-22). Kit built and self-checked; no evaluation run yet.
**Related Specs:** none (agent harness, no SPEC yet). **Decisions:** ADR-0001 (harness-neutral instructions) applies: nothing here
names a tool or model.
**Origin:** a review of the repo's agent documents against a prompting checklist found no evidence that any skill changes an outcome.
The harness check tests shape only. Learnings entry: [`learnings/agent-harness.md`](../../learnings/agent-harness.md).

## 1. Problem

19 skills in `.agents/skills/` cost context every time they load, and each needs upkeep. The repo cannot say whether any of them makes
an agent do better than it would unaided. A skill that does nothing should be cut; one that helps should be kept with evidence.

## 2. What is built

- Runbook: [`docs/guides/skill-eval.md`](../../guides/skill-eval.md): purpose, goal, who runs it and when, cost, steps, how to read the result.
- Tool: `scripts/skill-eval.sh` (`check-tasks`, `plan`, `setup`, `prompt`, `guard`, `collect`, `verify`, `report`, `cleanup`). It never starts an
  agent, so any agent on any model can be tested. Git use is limited to adding and removing detached worktrees.
- Kit: `scripts/skill-eval/`: three tasks for `tdd-guide` (T1 a search provider's usage limit, T2 a helper time limit, T3 a jailed
  `iw` step), the rules text every run is given, and the checks for `tdd-guide`.
- Self-check 2026-11-22: `scripts/skill-eval.sh check-tasks`: all three reference tests fail on the parent of the fix and pass with
  the real fix. `verify` and `report` were run on a hand-made diff (PASS for the real fix, FAIL for an empty one). `setup`, `collect` and
  `cleanup` (the worktree steps) were **not** run, because creating a worktree is the owner's call.

## 3. Phases

| Phase | What | Acceptance |
|---|---|---|
| P0 | Pilot: one task (T1), 2 repeats per arm (4 runs), one cheaper agent and model | All 4 runs finish; tokens and time per run measured and written into the guide; the checks can tell a good run from a poor one (not all runs pass all checks) |
| P1 | Full `tdd-guide` evaluation: T1 to T3, 3 repeats per arm (18 runs) | A ledger line with per-arm numbers and a keep, trim or remove decision; raw tables in `docs/audits/` |
| P2 | Next skills: `task-planning`, `clean-code` (most likely to repeat what a model knows) | Per skill: tasks and a checks file added to the kit, result recorded |
| P3 | Decide whether a standing rule is worth it (re-evaluate a skill when its `last_reviewed` goes stale) | Owner decision recorded; if yes, a line in `documentation-stewardship` |

## 4. Remaining Work

1. Owner: approve the pilot and pick the agent and model to test (a cheaper one is fine; say which in the ledger).
2. Run P0 following the runbook; replace the cost estimates in the guide with the measured figures.
3. Owner: decide on P1 from the P0 numbers.
4. `task-planning` and `clean-code` have no tasks or checks yet; each needs both before P2.

## 5. Traps found while building it

- `sort -R` groups identical lines; the plan step shuffles unique lines with `awk` and `sort -n` instead.
- Two of the three tasks come from one commit; their prompts must name the interface the reference test needs (the `timeout` field,
  the exact reason text), or a correct fix by a different name fails the test.
- AGENTS.md names `tdd-guide` in both arms; the *without* arm can see the pointer to a missing file.
