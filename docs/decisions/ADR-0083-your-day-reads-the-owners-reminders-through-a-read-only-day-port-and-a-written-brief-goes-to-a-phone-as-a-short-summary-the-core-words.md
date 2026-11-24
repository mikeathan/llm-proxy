---
id: ADR-0083
title: Your day reads the owner's reminders through a read-only Day port, and a written brief goes to a phone as a short summary the core words
status: proposed
date: 2026-11-24
subsystems: [autonomy]
supersedes:
superseded_by:
---

# ADR-0083: Your day reads the owner's reminders through a read-only Day port, and a written brief goes to a phone as a short summary the core words

## Context

Two things the plan (`morning-paper.md`, phases 5 and 6) left open were answered by the owner on 2026-11-24:

- **Your day.** The Brief may show the owner's own reminders. ADR-0068 keeps the Reminders stack's store private and ADR-0077's Home block carries "counts and fixed sentences only", so a reminder title (the owner's own words, which might name people or places) is a new data class beside a stack that reads the web and calls a model. The owner said yes to showing titles.
- **Telegram.** The owner decided that a brief goes to Telegram so updates reach them away from home (D3/D4 of 2026-11-21), and on 2026-11-24 chose "top stories plus the Home lines" in a message that fits a phone, with the link reusing the connector's public host and text only while none is set (they are on http; a webhook needs https). A quiet day (D2) stays "dashboard only, no push": they did not change it. ADR-0036 says the core words anything a stack finds, and the `HomeLine` documentation said a home line is never part of a message to a connector.

## Decision

1. **A read-only Day port** (`PortDay`, "briefday"): `Items(ctx, tomorrow bool) ([]DayItem, error)`. `DayItem` is `{Clock "15:04", Title, Repeats}` and nothing else (no id, channel, connector, zone or error text). The core (`stackport.Day`) reads the Reminders stack's own list route in process (`stackHost.Read`), keeps only reminders still `scheduled` whose next time falls on the day asked about in the owner's zone, clips each title to one line (160 runes, control characters dropped), orders by time and returns at most 16. The owner's zone is the Reminders setting, else this computer's.
2. **Reminders off or unreadable is `CodeUnavailable`, never an empty day.** The stack stores `DayBlock.Unavailable` and the screen says so, so a quiet day and a switched-off stack are not confused.
3. **Morning shows today, an evening brief tomorrow** (an edition ready from 12:00 on). A repeating reminder shows by its next time. There is no calendar or agenda source in the product, so "Your day" is reminders only.
4. **The titles are stored after the model has written the sections and are shown as text, nothing more.** They are not in a model prompt (a test pins it), not a fetch target, and **not in the Telegram message**: the owner approved titles for the dashboard only.
5. **The brief summary** is a new method on the existing Notice port, `Brief(ctx, channel, BriefNoticeRequest)`: the brief's name, up to five top stories (headline and link as the source gave them) and the home facts **by id and count only**. The core words the message (`stackport.BriefText`): it re-words every home fact from its own tables by id (`HomeWords`, the network table and Radio's), drops an id it has no words for, and never reads the sentence the stack stored. The text is plain, at most 3,000 bytes before the address (Telegram refuses 4,096 characters), and sent through the existing notify path, so the channel must be an enabled connector.
6. **The address of the brief** is `BriefPage(webhookURL)`: the origin of the connector's own stored webhook address, only when it is https and has a host, plus `/newsroom/briefs`. Nothing is guessed and no setting is added; with none the message is text only.
7. **An edition has two switches, both off by default:** `day` and `send_to` (a connector type such as `telegram`). A scheduled issue's summary is sent once: the issue records `sent_at` only after the connector took it, a failed or refused send is tried again on the next minute's pass until `PublishCatchUp` (3 hours) is over, and an issue the owner writes by hand ("Write a new one now") is never pushed. An issue with no story and no home fact sends nothing and is marked.
8. **Amends ADR-0077.** Its "the lines are never part of a message to a connector" is replaced by "a home line reaches a connector only as id and count, worded again by the core". The `HomeLine` documentation says so. The reason is the owner's: the network and radio findings are the point of the phone message, and the id-and-count form keeps a device name, address or tag out of Telegram by construction.

## Consequences

- Easier: the phone gets the brief without opening vertex; "Your day" adds no way for the owner's reminder text to reach a connector or a model.
- Accepted cost: the network findings (counts and fixed sentences, no names) and the stories' headlines and links go to Telegram's servers, as the owner accepted on 2026-11-21 and again on 2026-11-24.
- Not done: a calendar or agenda source; a message only on a day with news (D2 stays "dashboard only" for a brief with nothing to say: it sends nothing); the Briefs page does not yet say that Telegram refused a summary because the connector is not enabled (the stack log has a warning for a failure; a refusal is silent); the sidebar dot and the card changes are separate (SPEC-003).
- A recurring reminder shows only on the day of its next time, so an evening brief sees tomorrow's morning reminder because today's has passed.

## Alternatives considered

- **Read the Reminders stack's files or routes from the Newsroom** — couples stacks (IV.6) and hands the Newsroom ids, channels and errors.
- **Counts only for reminders ("3 today, the next at 14:00")** — the owner chose titles; a count alone is not much use.
- **A new port for the summary** — the Notice port already carries "the core words and sends"; one more method keeps one place for the rules.
- **Send the stored home sentence** — the stack could then put its own words on the phone (ADR-0036); the id-and-count form cannot.
- **Add a public-host setting for the link** — the connector's stored webhook address already says where the service is reached from outside.

## Links

- SPEC-013 1.22; `stacks/sdk/ports.go` (`Day`, `DayItem`, `BriefNoticeRequest`), `backend/internal/core/stackport/day.go`, `notice.go`, `home.go` (`HomeWords`), `stacks/newsroom/brief/day.go`, `send.go`; ADR-0036, ADR-0068, ADR-0077.
