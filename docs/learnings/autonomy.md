---
subsystem: autonomy
last_updated: 2026-10-26
---

# Learnings — Autonomy and reminders (SPEC-011)

What was tried and what happened. Read before proposing an approach in this area; add a line when a run,
eval, audit or merged phase teaches something (newest last within each section). Format and rules:
[`README.md`](README.md).

## Dead ends — do not retry without new evidence

_None recorded yet._

## Root causes and fixes that held

- 2026-11-19 (Reminders stack, ADR-0068): **a stack's database file must not be created at start.** The first build opened `reminders.db` in `Start`; the app boot tests then failed with "directory not empty" because the host stops a stack from `context.AfterFunc` (asynchronous) while the test removed its temp folder. The store creates its file on the first write and reads a missing file as empty (`stacks/reminders/store.go`, `TestStore_NoFileUntilTheFirstWriteAndReadsOfNothingAreEmpty`).
- 2026-11-19: **an off module must arm no timer** (an overdue reminder's zero-length timer spun the loop), and **the one-time copy must run inside `Start`** (a reminder set at boot took id 1 and made the copy fail forever); both found by review, not by the first tests (the fake clock hides the spin). A factory reset keeps `orchestrator.db`, so reminders keep surviving it.
- 2026-11-19: **the cron grammar cannot go in the SDK** (the SDK requires nothing; core may not import an SDK sub-package): a `schedule` port keeps the one parser in the core. The stack's own tests use a small test evaluator; `internal/app/stackhost_reminders_test.go` runs the stack over the real grammar.
- 2026-11-19: **`reminders.disabled` sits in a block the Settings form saves whole**, so `applySettingsFields` keeps the stored value.
