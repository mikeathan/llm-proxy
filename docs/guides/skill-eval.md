# Skill evaluation — does a skill change what an agent does?

**Audience:** maintainers. Non-normative (an operator how-to; `AGENTS.md` and the SPECs stay authoritative).
**Tool:** `scripts/skill-eval.sh` with the kit in `scripts/skill-eval/`. Plan: [skill-eval](../PLANS/cross-cutting/skill-eval.md).

## Purpose

A skill in `.agents/skills/` is text an agent loads on demand. A skill that has never been tested may change nothing, or
make the agent worse, and it still costs context and upkeep. The harness check (`scripts/check-agent-harness.sh`) tests a skill's
**shape** (size, front matter, links, a `Skip:` line). It cannot tell whether the skill **changes an outcome**. This evaluation does.

## Goal

For one skill, answer one question with numbers: *on tasks the skill is meant to help with, does an agent with the skill do
measurably better than the same agent without it?* The result decides whether the skill is kept, cut down, or removed, and goes
into the learnings ledger so nobody repeats the question.

## How it works, in one paragraph

Each task is a real bug the repo fixed earlier. The agent starts from the commit **before** the fix, is given a plain bug report
(no mention of tests, TDD or skills), and works in a throwaway folder. Half the runs have the skill folder deleted from that
folder (arm *without*), half keep it (arm *with*). Afterwards: (1) the repo's own test for that fix, taken from the fix commit,
is run against the agent's change (automatic), and (2) a checklist of what the skill claims to add (for `tdd-guide`: test first,
seen failing, minimal change, …) is scored from the transcript and the diff (by a person or a separate judge).

## Who runs it, and when

- **A maintainer starts it**, on purpose. It runs coding agents and costs tokens and time; AGENTS.md also makes any git change
  (here: adding and removing disposable worktrees) the owner's call.
- **Any coding agent on any model can be the one being tested**: this repo's own harness-neutral rule (ADR-0001) applies.
  Use a cheaper model for the bulk of runs. Results apply **only to the agent and model used**; say which in the ledger line.
  A different model may need the skill more or less, so a result is not portable.
- **The scorer is not the agent that did the work**, and does not see which arm a run was (see Blinding).
- **When:** when a skill is created or rewritten materially; when the model or harness used day to day changes; when a skill's
  `last_reviewed` passes the staleness warning. Not on every change and not in CI: it is slow, costs tokens and varies run to run.

## Cost (estimates, not measurements)

Per run, a coding agent working a small Go bug plausibly uses a few hundred thousand to over a million input tokens (mostly cached
repeats of a growing context) and tens of thousands of output tokens, over roughly 10 to 25 minutes. The kit's tasks are small.
A full pilot (3 tasks × 2 arms × 3 repeats = 18 runs) is therefore a few hours and millions of tokens. **Start with the 4-run
pilot** (one task, 2 repeats per arm) and write the measured tokens and time from the first runs into the ledger, replacing
these estimates. Each run's `runlog.md` has places for them.

## Safety

- Worktrees are **detached** (no branch). Nothing is committed, merged or pushed. Remove them with `cleanup`.
- The agent is told (in `scripts/skill-eval/preamble.md`) to stay in its folder, not touch git, not start the server and not use
  ports. That is an instruction, not a wall: an agent given a path can still edit elsewhere. Run `skill-eval.sh guard`
  before and after each batch; the main checkout's status must not change. Never run it while the local server is in use on
  its port.
- Use a scratch location for the output (`SKILL_EVAL_OUT`, default a folder next to the repo). Never run it in the working copy.

## Steps

All commands are run from the repository root as `./scripts/skill-eval.sh <command>`; below, the folder is left out.

1. **Check the tasks still work** (no agent, no worktree, about a minute):
   `skill-eval.sh check-tasks` — each task's reference test must fail before the fix and pass after it. If one says
   BAD, the repo has moved too far from that commit; fix or replace the task before going on.
2. **Plan the runs:** `skill-eval.sh plan tdd-guide T1 2` (skill, tasks, repeats per arm). It writes a random order of
   with/without runs and the secret key (`KEY.tsv`). For the pilot use one task and 2 repeats.
3. **Note the main checkout:** `skill-eval.sh guard`.
4. **For each run, in the printed order:**
   - `skill-eval.sh setup rNN` makes the folder (the skill is deleted from it when the run is a *without* run).
   - `skill-eval.sh prompt rNN` prints the text to give the agent. Give exactly that text to the agent you are testing,
     working in that folder, in a **fresh session** with nothing else in its context. Use the same agent and model for every run.
   - When it finishes: save the agent's transcript or session log as `runs/rNN/transcript.txt` in the output folder, fill in
     `runs/rNN/runlog.md` (tokens, time, anything odd), then `skill-eval.sh collect rNN` and `skill-eval.sh verify rNN`.
5. **Run `skill-eval.sh guard` again.** Compare with step 3.
6. **Score** each run's `runs/rNN/score.md` against `scripts/skill-eval/skills/<skill>.checks.md`. Do this without opening `KEY.tsv`.
7. **Report:** `skill-eval.sh report` gives the reference-test passes and the checks ticked, per arm.
8. **Clean up:** `skill-eval.sh cleanup all`.
9. **Record** the result (see below).

## Reading the result

A paired comparison: same task, same agent, with versus without.

- **Keep the skill** when the *with* arm ticks clearly more checks (not only within one check of noise) and its reference-test
  passes are not lower.
- **Trim or remove the skill** when the arms are equal or *without* is better on every task, after at least 3 repeats per arm.
  The agent already does this unaided, or the skill misleads it. Cut the lines the transcripts show were ignored.
- **Inconclusive** when the arms differ by about one check in either direction, or the runs vary wildly between repeats: add
  repeats before deciding; do not decide on 2 runs.
- If *every* run in *both* arms passes *every* check, the checks or tasks are too easy; harder tasks teach more than more repeats.

## Recording the result

One line in [`docs/learnings/agent-harness.md`](../learnings/agent-harness.md) in the ledger format: **Tried** (skill, tasks, agent and
model, repeats) → **numbers** (per arm: reference passes, checks ticked, tokens and time per run) → **Lesson** (kept, trimmed or
removed, and what was cut). Put the raw tables in a new file in `docs/audits/` with its front matter and catalog row (see
[learnings/README.md](../learnings/README.md)). Then change the skill if the result says so, bump its `last_reviewed`, and run
`./scripts/check-agent-harness.sh`.

## Blinding, and its limit

Arms are shuffled and runs have neutral ids, and the scorer reads only the diff and transcript. A scorer who knows the skill's
contents can still often tell from a transcript whether the agent used it. That is why the checks are written as observable facts
(was the failing test shown?) and why the reference test runs automatically.

## Known limits

- **Few runs.** The pilot is a sanity check, not statistics.
- **The pointer in AGENTS.md stays.** AGENTS.md names `tdd-guide` in the *without* arm too; the agent may look for the missing
  file. That slightly understates the skill's value, and is noted in the runlog if it happens.
- **The reference test is a contract.** A test from the fix commit may use names the agent could not guess, so each task prompt
  names the interface the test needs. `check-tasks` proves each test fails before and passes after the real fix, not that every
  correct fix passes it; read a FAIL by hand before counting it against an arm.
- **One agent, one model, one repo moment.** Repeat after a model change.

## Adding a skill or a task

- A task is two files in `scripts/skill-eval/tasks/`: `<id>.env` (the fix commit, the source and test files, the package and the
  test name) and `<id>.prompt.md` (the bug report in plain words, naming any interface the test depends on). Pick a merged fix with
  a small source change and a test that fails without it. Run `check-tasks`.
- A skill's checks are `scripts/skill-eval/skills/<skill>.checks.md`: observable facts, one per line as a `- [ ]` item.
