---
subsystem: automation
last_updated: 2026-10-26
---

# Learnings — Automation dispatcher and heartbeat (SPEC-007)

What was tried and what happened. Read before proposing an approach in this area; add a line when a run,
eval, audit or merged phase teaches something (newest last within each section). Format and rules:
[`README.md`](README.md).

## Dead ends — do not retry without new evidence

- **Tried:** Model-written memory as the heartbeat's "seen" ledger → **rejected**. **Lesson:** non-deterministic; the model cannot know delivery succeeded. Evidence: [heartbeat-seen-ledger.md](../PLANS/automation/heartbeat-seen-ledger.md).
- **Tried:** Injecting the whole 60-day seen ledger into every prompt → **rejected**. **Lesson:** unbounded prompt growth; filter by link in code. Evidence: [heartbeat-seen-ledger.md](../PLANS/automation/heartbeat-seen-ledger.md).

## Root causes and fixes that held

- **Tried:** Heartbeat `alert` recorded even when nothing was sent → **found 2026-10-15**. **Lesson:** record delivery honestly (tracked). Evidence: [2026-10-15-next-work-analysis.md](../audits/2026-10-15-next-work-analysis.md), [heartbeat-seen-ledger.md](../PLANS/automation/heartbeat-seen-ledger.md).
