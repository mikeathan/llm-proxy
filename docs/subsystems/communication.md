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
| **Decisions (why)** | [ADR-0072](../decisions/ADR-0072-a-stack-hands-the-core-a-structured-notice-and-the-core-words-and-sends-it.md) A stack hands the core a structured notice and the core words and sends it (proposed) |
| **Open plans (next)** | [`cross-cutting/connector-auto-reply.md`](../PLANS/cross-cutting/connector-auto-reply.md)<br>[`autonomy/cross-channel-conversations-and-remote-approvals.md`](../PLANS/autonomy/cross-channel-conversations-and-remote-approvals.md)<br>[`cross-cutting/sources-and-more-connectors.md`](../PLANS/cross-cutting/sources-and-more-connectors.md)<br>[`cross-cutting/email-connector.md`](../PLANS/cross-cutting/email-connector.md) (proposed 2026-11-20: send-only SMTP, TLS required, secret out of reach of models and agents) |
| **Learnings (tried before)** | [`learnings/communication.md`](../learnings/communication.md) — read before proposing an approach |
| **Evidence (audits, evals)** | _none yet_ |
| **Skills** | [`connector-patterns`](../../.agents/skills/connector-patterns/SKILL.md) |
| **Guides** | — |
| **Archived plans** | others by topic in [`ARCHIVE/README.md`](../PLANS/ARCHIVE/README.md) |
