---
id: ADR-0068
title: Reminders are a stack: it keeps its own store, sends the owner's own words through a notify port, and schedules through a schedule port
status: proposed
date: 2026-11-19
subsystems: [platform, admin-ui, autonomy]
supersedes:
superseded_by:
---

# ADR-0068: Reminders are a stack: it keeps its own store, sends the owner's own words through a notify port, and schedules through a schedule port

Proposed: it is in force when the owner merges the pull request that carries it. Carries out [ADR-0032](ADR-0032-a-stack-is-a-package-with-a-manifest-kept-apart-by-tests.md)
for a fourth stack after Network, Agents ([ADR-0051](ADR-0051-the-agents-stack-takes-the-watch-and-the-core-keeps-the-walk-the-words-and-the-gate.md)) and
Radio, and **amends [ADR-0036](ADR-0036-a-stack-offers-findings-by-id-and-the-core-words-them.md)** in one point (the notify port, below).

## Context
- Reminders (SPEC-011) were core code: `core/reminders` (about 1,700 lines), rows in the main SQLite database, a delivery loop started from the app
  lifecycle, eight admin routes, three assistant tools and the `/remind` command. The owner asked (2026-11-19) to move behaviour out of the core into a
  new stack and to keep going in one branch. Reminders is the candidate with the smallest coupling to the run path: its callers are the tools, the
  command and two zone helpers (an import count of 2026-11-19: handlers, app, commands, chat; `memorycapture` and `delegation` are called from the run path and
  stay).
- Three things the existing stacks did not need: a store the stack keeps **as a database** (Radio is the precedent: `modernc.org/sqlite` in the data folder), the
  sending of the **owner's own stored text** to their connectors (ADR-0036 says a stack never puts its own words on the owner's phone: an alert is worded by the
  core from a fixed table), and the **cron grammar** that automations already use.
- Constitution IV.6 holds: a stack imports only the SDK, the SDK requires nothing (a test fails on a dependency in `stacks/sdk/go.mod`), tools are declared as data.

## Decision
1. **`stacks/reminders`** (module `llm-proxy-stacks/reminders`) takes the store, the schedule and the delivery loop, the late/missed/retry policy
   (`Policy.Advance`), the channel policy, the natural-language reading of `/remind` (`ParseCommand`), the routes the screen reads (eight, byte-identical) and
   the tools. The core keeps what the run path needs: the assistant's tool schemas (`tools/manifests/reminders.json`, unchanged, so the property order the model
   sees does not move), the `/remind` entry, `models.ChatZone` and the conversation's zone and origin connector, which it adds from its own context and never takes
   from the model's arguments.
2. **Tools are declared in the manifest** (`reminders_schedule`, `reminders_list`, `reminders_cancel`, `reminders_command`) and called by name with JSON; each
   answers with the sentence the person is shown, so the wording lives in one place. `internal/app/stacks.go` adapts them to `tools.ReminderService` and
   `commands.Reminders`. A stack that is off or not running answers "Reminders is switched off" / unavailable in plain words; the run path never depends on it.
3. **Own store, and a one-time copy.** The stack keeps `reminders.db` in the data folder (its one manifest `Storage` entry) and creates it on the first write: an
   install that never sets a reminder has no file. The rows the service kept in its main database are read by the composition root and **staged on the stack before it starts**; the stack copies them in its own `Start`, before its
   loop runs and before a route or a tool can answer, so nothing can be set ahead of them and take an id they hold. The copy keeps each id, status (`scheduled`, `paused`,
   finished), revision, attempts, outcome and times, in one transaction that also sets a marker in the stack's file, so it never repeats and new ids continue after the highest
   copied; an id already taken keeps the old reminder under the next free id instead of failing the copy. **The old table is left as it was** (nothing deletes it), and **a factory reset erases the reminders** (owner decision, 2026-11-19; before the stack they survived it because they lived in `orchestrator.db`, which the reset keeps): the stack empties its database in place and the old table is emptied too, so nothing comes back. A rollback to an earlier version would find its old rows, and
   they would be stale. Removing the old table is a later, owner-approved step.
4. **The notify port (amends ADR-0036).** `PortNotify`: `Channels()` lists the enabled connector types, `Send(channel, text)` sends the owner's own text, unchanged,
   as plain text to that connector type. The core checks every call: the channel must be an enabled connector, the text non-empty and at most 16 KiB, it is never
   parsed as markup, and a refusal is a code and a fixed sentence. The stack names a type and never an address, token or chat. **Only the Reminders stack is given the
   port**, the owner's text is never logged (only id, event and channel), and no agent, automation or prompt can reach it: the assistant tools are scoped to attended runs
   as before. ADR-0036's rule stays for everything a stack *finds or reports* (an alert is worded by the core); a reminder is not a finding, it is the owner's own
   sentence stored for later.
5. **The schedule port.** `PortSchedule`: `Next(expr, after)` evaluates the service's one cron grammar, supplied by the core from `automation.ParseCron` (five
   fields or a descriptor, never seconds). The SDK cannot carry a scheduler library (it requires nothing), and a second parser would break "one cron parser"; so the
   stack accepts exactly what an automation accepts. Rejected: a copy of the grammar in the SDK, a cron library in the SDK or in the stack's go.mod (a boundary test
   forbids it), and a standalone shared module (`stacks/cron`; the same test).
6. **A module with a switch, under Stacks.** One module, `reminders`, sends a message, needs no passphrase to switch (it sends only what the owner asked for). The
   switch is an "off" flag, `reminders.disabled` in the existing `reminders:` settings section, so an install that has always had reminders keeps them on. The
   Settings form never changes it (`applySettingsFields` keeps the stored value). While off nothing is delivered and no timer is armed (an overdue reminder would fire one at once and spin the loop), a change is refused with "Reminders is switched
   off", and what fell due is handled by the late/missed policy when it is switched on again. The menu entry moves into the Stacks group, `/reminders` has a Modules tab, and
   the Reminders card follows the stack's state (ADR-0066). The assistant's three reminder tools are listed only while the module is on (owner decision, 2026-11-19), so the model is never offered a tool that can only refuse.
7. **What did not move, on purpose:** `memorycapture`, `delegation`, the engine, guardrails and Lock down (the run path calls them); the `ChatZone` helper (a pure time-zone
   function used by chat and webhooks, now in `models`).

## Consequences
- Easier: reminders change without touching the engine; the stack's tests run alone (`GOWORK=off`); a second stack that must send the owner's own words has a
  bounded, tested port to ask for.
- Harder: the SDK has two more ports (`notify`, `schedule`) and the core two small adapters (`stackport.Notify`, `stackport.Schedule`); the stack's cron tests use a small
  test-only evaluator, and `internal/app` tests the stack over the real grammar.
- Behaviour kept: the eight routes' JSON and statuses, the reply sentences, the `(late) ` prefix, the 50-open and 15-minute limits, the revision guard on a write-back, and
  the error words (including the cron grammar's own). Behaviour changed, on purpose: a message over 16 KiB is refused at the port, and a reminder cannot be set while the module is off.
- Cost: no added latency on the run path (an in-process call); one SQLite connection for the stack's file.
- Open risk: the copy was tested against the old schema in `internal/app` and not yet run on the owner's real database (see the plan).

## Alternatives considered
- **A SQL port to the core's database** — a generic SQL surface for every stack, for a table one stack uses.
- **Keep delivery in the core and make the stack only a store** — splits the late/missed policy from the timer that applies it.
- **Move `memorycapture` or `delegation` first** — both are called from the run path and need a run port.
- **A Reminders board on the dashboard** — the Automations board already holds the card; the page's list is the stack's own page.

## Links
- Plan [`reminders-stack.md`](../PLANS/cross-cutting/reminders-stack.md); SPEC-011; `stacks/reminders/`, `stacks/sdk/ports.go`, `backend/internal/core/stackport/notify.go`, `backend/internal/app/stacks.go`.
