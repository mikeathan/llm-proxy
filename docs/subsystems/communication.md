---
subsystem: communication
specs: [SPEC-009]
---

# Communication connectors

Outbound messages and inbound webhooks (Telegram and others), auto-reply and channel conversations.

Start here, then open only what the task needs.

| | |
|---|---|
| **Contract (current behavior)** | [SPEC-009](../SPECS/communication.md) |
| **Code** | `backend/internal/core/tools/communication.go`, `backend/internal/transport/http/handlers/webhook_handlers.go` |
| **Architecture** | [map](../architecture/map.md) · [contracts](../architecture/contracts.md) · [checklists](../architecture/checklists.md) · [pitfalls](../architecture/pitfalls.md) — search pitfalls for: connector, webhook, telegram |
| **Decisions (why)** | _none yet_ — decisions still live in the plans below |
| **Open plans (next)** | [`cross-cutting/connector-auto-reply.md`](../PLANS/cross-cutting/connector-auto-reply.md)<br>[`autonomy/cross-channel-conversations-and-remote-approvals.md`](../PLANS/autonomy/cross-channel-conversations-and-remote-approvals.md) |
| **Learnings (tried before)** | [`learnings/communication.md`](../learnings/communication.md) — read before proposing an approach |
| **Evidence (audits, evals)** | _none yet_ |
| **Skills** | [`connector-patterns`](../../.agents/skills/connector-patterns/SKILL.md) |
| **Guides** | — |
| **Archived plans** | others by topic in [`ARCHIVE/README.md`](../PLANS/ARCHIVE/README.md) |
