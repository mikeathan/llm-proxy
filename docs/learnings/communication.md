---
subsystem: communication
last_updated: 2026-10-26
---

# Learnings — Communication connectors (SPEC-009)

What was tried and what happened. Read before proposing an approach in this area; add a line when a run,
eval, audit or merged phase teaches something (newest last within each section). Format and rules:
[`README.md`](README.md).

## Dead ends — do not retry without new evidence

_None recorded yet._

## Root causes and fixes that held

- 2026-10-29 — An empty `webhook_token` meant an open webhook (`webhook_handlers.go`, E8 in the admin-auth design). Made
  mandatory (v0.19.0). Traps: the UI saves the whole `communication` block, so a secret created only on the server
  would be wiped by the next Settings save; the secret is therefore created in the UI form (and at startup, before
  connectors are built, for already-registered webhooks), never by the `create` action. Changing a secret needs a
  second step, Register, to tell Telegram; until then Telegram's calls fail with 401 and it retries.
