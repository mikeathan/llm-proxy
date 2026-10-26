---
subsystem: automation
specs: [SPEC-007]
---

# Automation and heartbeat

Scheduled and triggered runs: dispatcher, executor, run lifecycle, playbooks/templates, heartbeat and delivery.

Start here, then open only what the task needs.

| | |
|---|---|
| **Contract (current behavior)** | [SPEC-007](../SPECS/automation-dispatcher.md) |
| **Code** | `backend/internal/core/automation/`, `backend/data/templates/` |
| **Architecture** | [map](../architecture/map.md) · [contracts](../architecture/contracts.md) · [checklists](../architecture/checklists.md) · [pitfalls](../architecture/pitfalls.md) — search pitfalls for: automation, dispatcher, heartbeat, notify |
| **Decisions (why)** | _none yet_ — decisions still live in the plans below |
| **Open plans (next)** | [`automation/automation-learning-journal.md`](../PLANS/automation/automation-learning-journal.md)<br>[`automation/chat-to-automation.md`](../PLANS/automation/chat-to-automation.md)<br>[`automation/heartbeat-seen-ledger.md`](../PLANS/automation/heartbeat-seen-ledger.md)<br>[`automation/playbook-library-management.md`](../PLANS/automation/playbook-library-management.md) |
| **Learnings (tried before)** | [`learnings/automation.md`](../learnings/automation.md) — read before proposing an approach |
| **Evidence (audits, evals)** | _none yet_ |
| **Skills** | [`automation`](../../.agents/skills/automation/SKILL.md), [`testing-guide`](../../.agents/skills/testing-guide/SKILL.md) |
| **Guides** | [`guides/automation-digest.md`](../guides/automation-digest.md) |
| **Archived plans** | [`ARCHIVE/automation/`](../PLANS/ARCHIVE/automation/); others by topic in [`ARCHIVE/README.md`](../PLANS/ARCHIVE/README.md) |
