---
id: ADR-0055
title: Radio may keep a short signal track per device on disk, only when the owner turns it on
status: proposed
date: 2026-11-14
subsystems: [network-audit]
supersedes:
superseded_by:
---

# ADR-0055: Radio may keep a short signal track per device on disk, only when the owner turns it on

Proposed 2026-11-14 after the owner's decision in conversation: **off by default; 24 or 48 hours when on; a delete control; turning it off deletes.** It narrows one line of [ADR-0053](ADR-0053-radio-keeps-a-keyed-tag-for-other-peoples-devices-and-seals-the-owners-with-hard-size-and-age-limits.md) ("nothing finer than a day") for this one case and changes nothing else in it. It becomes accepted when the owner accepts it in the PR; the legal question below stays an **open risk the owner accepts, not advice taken**.

## Context
The radar (v0.89.0) draws a device's distance from its signal. The track of one device over a run is built in memory only (v0.90.0): it shows what happened this evening and is gone when listening stops or the service restarts. The owner wants it to survive a restart and to look back a day or two. ADR-0053 allows only day-level rows for other people's devices. Facts that bound the choice: a keyed tag is still personal data in the ICO's reading (pseudonymisation does not change that); a per-cycle series says **when** a device is near the house, which a day row does not; the household-exemption question was not found by search.

## Decision
We will keep **a per-cycle signal track on disk, off by default**, as a setting `track_hours` with three values: `0` (off, the default), `24`, `48`. 48 is a hard ceiling in code, not a configurable maximum.

- **What a point holds:** the keyed tag (the same one-way tag as ADR-0053, never an address), the time of the cycle, the mean signal in dBm and the first channel. Nothing else: no name, no frame, no network name.
- **Off deletes.** Setting `0` removes every stored track point at once, in the same request. The default store has none.
- **Forget deletes.** "Forget neighbours" (key rotation) removes every track point along with the rows it already removes.
- **Age and size.** A point older than `track_hours` is pruned (the daily job and each write); one table cap of 300,000 rows evicts the oldest first, inside the existing 20 MB file cap.
- **Memory stays the base.** With the setting off, the track is memory-only as in v0.90.0. With it on, the route reads the disk, which holds what the run wrote.
- **Never leaves the stack as detail.** The agent digest and reports keep neighbours as counts by class; a track is shown only on the owner's Radio page.
- **Open and recorded, not assumed away:** a tag with a time series is still pseudonymous personal data. Short age, off by default and two delete paths reduce the risk; they do not remove it. The owner accepts it, or takes advice first, before turning the setting on for real.

## Consequences
- A restart no longer loses the evening's track when the setting is on; one table (`air_track`) and one setting are added; a schema step runs once on the live file.
- "Off" and "forget" must both delete, and a test must prove it, and that the file holds no address of another person.
- A default install stores exactly what ADR-0053 allowed.
- A longer age needs a new ADR; it is not a setting.

## Alternatives considered
- **On by default for 24 hours** — it makes the most intrusive capture the default while the legal question is open.
- **Owner's devices only on disk** — already covered by the 30-day hourly strip; it does not answer the neighbours question.
- **Hourly points instead of per cycle** — smaller, but it loses the movement the track exists to show; the age limit does the work instead.
- **A configurable age up to 14 days** — turns a bounded exception into the rule.

## Links
[ADR-0053](ADR-0053-radio-keeps-a-keyed-tag-for-other-peoples-devices-and-seals-the-owners-with-hard-size-and-age-limits.md) · plan [`radio-stack.md`](../PLANS/cross-cutting/radio-stack.md) · handoff [`radio-stack-handoff-2026-11-14.md`](../PLANS/cross-cutting/radio-stack-handoff-2026-11-14.md).
