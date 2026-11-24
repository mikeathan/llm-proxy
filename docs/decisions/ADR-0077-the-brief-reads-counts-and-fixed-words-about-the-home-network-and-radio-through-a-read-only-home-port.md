---
id: ADR-0077
title: The Brief reads counts and fixed words about the home network and Radio through a read-only Home port
status: proposed
date: 2026-11-23
subsystems: [autonomy, network-audit]
supersedes:
superseded_by:
---

# ADR-0077: The Brief reads counts and fixed words about the home network and Radio through a read-only Home port

## Context

The owner wants the Briefs edition to include what Network and Radio know ("it is important to get updates when I am not home", plan `morning-paper.md`, D-answers of 2026-11-21). The Newsroom stack is the one stack that **reads the open internet and talks to a model**, so anything it is given can in principle be steered by a hostile web page, and anything it stores is shown on the Home card and, later, sent to Telegram. Until now no stack could read the core's network facts: ports run the other way (a stack *offers* findings by id, ADR-0036; the core words them). `NetworkDevice.Name` is documented as "never put in a finding, a prompt or a message"; radio frames and tags are other people's data (ADR-0053; Constitution I.16).

## Decision

We add one **read-only, pulled** port, `PortHome` ("briefhome"), with one method, `Lines(ctx) ([]HomeLine, error)`.

- **The data class is counts and the core's own words.** `HomeLine` is `{ID, Count, Words}`. `ID` names a fact in a table inside the core, `Count` is clamped (0 to 100000), `Words` is the sentence the core wrote for that ID. No address, hardware address, device name, network name, radio tag or anything a device, a frame or a resolver sent is ever read into a line. A test reflects over `HomeLine` and fails if another field is added, and a test feeds device names, addresses and a hostile "claim" through the port and fails if any reaches the stack.
- **Network lines** come from the stored inventory (not a refresh, no look is started): devices the network sees now, devices not marked as the owner's, addresses that changed hands. A presence check that is off or stale gives one fixed "cannot say" line, not "0 online".
- **Radio lines** reuse `core/radiodigest`: only finding ids the core has words for, in the core's words (the stack's own wording is not read), at most 16. Radio off, unreadable or quiet each give one fixed line.
- **One part failing does not hide the other.** At most `MaxHomeLines` (16) lines.
- **Direction and use.** The core never pushes. The stack calls the port when it writes an edition that asked for Home (an edition switch, off by default), stores the lines **after** the model has written the sections, and shows them as text. The SDK doc states the rule and the stack tests pin it: the lines are never put in a model prompt or a fetch. **Amended 2026-11-24 (ADR-0084): `HomeLine` also carries `Level`, one of three fixed words set by the core from its own table, and the reflecting test lists four fields.** **Amended 2026-11-24 (ADR-0083):** they reach a connector only as id and count, which the core words again from its own table; the sentence a stack stored is never sent on. They are kept with the edition (30 days) and are served only by the admin-only issue routes.
- **No name leaves the house.** Because the lines are counts, the Home block can later go to Telegram without carrying a device name; the block says "Counts only. Names and addresses stay on the Network and Radio pages" and links to them.

## Consequences

- Easier: the Brief shows the home without a core change per fact; a new fact is one table row in the core.
- Harder: `PortHome` joins `portNames`, `Host`, `checkPorts` (which now also checks the notice port it had left out) and the manifest of the one stack that requires it; the port's wording table is a second place to keep (radio's stays in radiodigest).
- Accepted cost: the brief cannot say *which* device joined at 02:14. That is deliberate (names are the owner's own labels but reach a stack that reads the web); a later, separate decision could add a per-device line for the dashboard only.
- Done later (ADR-0083, 2026-11-24): Your day (a Day port, same pattern) and Telegram carrying the Home lines as id and count.

## Alternatives considered

- **A stack reads the Network and Radio stacks' files or routes.** Couples stacks and hands the Newsroom device names and tags.
- **Push findings into the Newsroom through `Events`.** Free text by type; cannot be held to counts.
- **Give the stack `StackFinding`s.** Those are offers *from* a stack; the join needs device identity, which is exactly what this port keeps out.
- **Per-device lines now (names, times).** Rejected for this slice: name text would sit next to a web-reading, model-calling stack and, later, in a message.
