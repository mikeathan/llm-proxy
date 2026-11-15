---
id: ADR-0056
title: Radio may watch for up to seven days under one session, and a stopped watch is offered again, never resumed by itself
status: proposed
date: 2026-11-15
subsystems: [network-audit]
supersedes:
superseded_by:
---

# ADR-0056: Radio may watch for up to seven days under one session, and a stopped watch is offered again, never resumed by itself

Proposed 2026-11-15; becomes accepted when the owner merges the pull request that carries the amended Constitution I.16 and I.17. Amends [ADR-0054](ADR-0054-a-scheduled-privileged-action-runs-under-a-bounded-session-a-person-approved.md) (its twelve-hour limit and its note that "a longer or indefinite sensor needs an amendment"). ADR-0054's own status line still says `proposed` although I.17 has been in force since the radio branch merged; the owner's merge of this change accepts both.

## Context
A bounded session lasts at most 12 hours and ends with the service. The owner deploys after almost every change, so a long watch could never be assembled from 12-hour pieces without a person starting each one, and each deploy ended the watch with no word that it had. The owner asked for a longer watch and for it to be clear when it stops.

Measured on 2026-11-15 on a development machine: recording one full 4,000-frame window and finishing its cycle took 182 ms (10.7 ms after this change); a finished cycle over 10,000 stored devices took about 110 ms. Nothing is measured on the server yet (helper CPU, file growth over a day, kernel drops); the handoff lists it as the acceptance check.

## Decision
- **Ceiling: 7 days** (the owner chose 7 over 3 on 2026-11-15). The session's hard limits become 7 days and 40,320 requests (the helper's own 240 an hour over 7 days). The helper's hourly cap, the three gates, the scope and the action's switch are unchanged and still checked on every request. The Radio page offers 1, 4, 12 hours, 1, 3 and 7 days.
- **The request count of a session is derived from the approved plan** (windows an hour times the hours, plus a margin), never a flat number, so a stack cannot spend requests faster than the plan a person approved.
- **No approval outlives the process.** A restart still ends every session. Nothing is stored that could authorise a window (no token, no sealed approval). Rejected: a sealed approval in the core with an expiry; it would be a standing credential, the thing ADR-0054 prevents.
- **A stopped watch is offered again.** The Radio stack keeps one small record of the watch (adapter, end time, and why it ended) in its own database. When the watch ended before its time through a restart, a crash, a failing adapter or a refusal, the Listening tab says so and offers **Resume listening** for the hours left, or **No, leave it stopped**. Resuming opens a new session and asks for the passphrase like any start. A watch the operator stopped, that ran out its time or that hit its frame budget is not offered again.
- **The frame budget is a rate** (170,000 frames an hour, at least 100,000), not a flat 2 million a run. A busy home was measured at about 9,000 frames a cycle (2.6 million a day), so the flat figure would have ended a long watch in under a day.
- **Alerts go to the bell only**, not to the connectors (the owner's choice). The Radio stack publishes two alerts through the Events port, `radio_watch_ending` (a fifth of a watch of 12 hours or more is left, once per watch) and `radio_watch_stopped` (a restart or crash, an adapter that went away, or a session that was ended, told once). The SDK's `Alert` gains `Hours`; the core words each alert from its own fixed table (`core/radiowatch`) and logs it for the bell with a new delivery, `in_app`, so nothing looks undelivered (`AlertLog` and `ErrInAppOnly`); the publish path the agent watch used is now the shared `core/stackalerts.Sink`. A restart cannot be told by the run that it ended, so the stack tells it at its next start, from its own watch record.

## Consequences
- A watch can span days. Between a restart and the owner's tap there is no listening; the page says so and the gap is visible, never silent.
- The standing risk the owner accepted in ADR-0054 grows from 12 hours to 7 days of listening under one approval; it is receive-only and bounded by the same limits.
- The per-frame database writes were the one measurable cost; a repeat frame of a device already recorded this cycle with a signal inside the stored range is no longer written.
- The lighter plan (fewer channels per cycle) is not part of this: it needs the server measurements first.

## Alternatives considered
- **Raise the ceiling only (no resume):** rejected; a deploy would end most watches and say nothing.
- **A sealed approval kept across restarts:** rejected as above.
- **3 or 5 days:** the owner chose 7; the ceiling is one constant (`session.MaxTTL`) if it should come down.
