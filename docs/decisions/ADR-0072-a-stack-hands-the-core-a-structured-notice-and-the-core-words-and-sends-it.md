---
id: ADR-0072
title: A stack hands the core a structured notice and the core words and sends it
status: proposed
date: 2026-11-20
subsystems: [platform, autonomy, communication]
supersedes:
superseded_by:
---

# ADR-0072: A stack hands the core a structured notice and the core words and sends it

Proposed: in force when the owner merges the pull request that carries it. It adds to, and does not replace, [ADR-0036](ADR-0036-a-stack-offers-findings-by-id-and-the-core-words-them.md) and [ADR-0068](ADR-0068-reminders-are-a-stack-with-its-own-store-a-notify-port-and-a-schedule-port.md), which stay accepted.

## Context
- ADR-0036: the core words anything a stack finds; a stack never puts free model prose on the owner's phone. Findings are pulled by id.
- ADR-0068: the `notify` port sends the owner's own text, and was given to the Reminders stack only.
- The Newsroom stack finds items on a timer and must tell the owner. A pull (as findings) cannot confirm delivery, and a failed send has to leave the items unrecorded so the next run retries.

## Decision
1. **One more port, `PortNotice`**: `Send(ctx, channel, NoticeRequest) error`, a synchronous call. The request is data: the topic, items (title, link, source) and changes (a was and now pair). No message text.
2. **The core words it.** `stackport.Notice` builds the message from fixed wording, one line per field, control characters dropped, only http and https links, plain text. The stack cannot put its own sentence on the owner's phone.
3. **The channel is a connector type, never an address**, and it must be an enabled connector; otherwise the call is refused with `CodeInvalid` and the stack shows "no channel". A connector that cannot be reached is `CodeUnavailable`; nothing was sent and the stack retries.
4. **Bounded**: at most `MaxNoticeItems` (50) items; over it is refused, never cut.
5. **The stack never sends.** Granted to a stack whose manifest requires it, while its module is on.

## Consequences
- Easier: a stack that finds things can notify without owning connectors or wording; a failed send leaves nothing recorded.
- Harder: the SDK has one more port and the core one more adapter, with a round-trip test (IV.6).
- Cost: the core's wording is fixed, so a notice reads the same for every topic.

## Alternatives considered
- **A pull offer like findings** — the stack cannot learn that delivery failed, so it cannot retry honestly.
- **Give the Newsroom the notify port** — it would let a stack send its own text, which ADR-0036 rules out.

## Links
- `stacks/sdk/ports.go` (`Notice`), `backend/internal/core/stackport/notice.go`, `backend/internal/app/stacks.go` (`notifyPort`), `stacks/newsroom/newsroom.go` (`noticeSender`); plan `docs/PLANS/ARCHIVE/autonomy/topic-watch.md`.
