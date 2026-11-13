---
id: ADR-0052
title: Radio is its own stack with generic sensors, and the core keeps the capture and the judgement
status: proposed
date: 2026-11-13
subsystems: [network-audit]
supersedes:
superseded_by:
---

# ADR-0052: Radio is its own stack with generic sensors, and the core keeps the capture and the judgement

Proposed: in force when the owner accepts it. Follows [ADR-0028](ADR-0028-stack-module-registry.md), [ADR-0032](ADR-0032-a-stack-is-a-package-with-a-manifest-kept-apart-by-tests.md),
[ADR-0034](ADR-0034-the-privileged-helper-is-core-not-a-stack-and-decides-who-is-calling.md), [ADR-0036](ADR-0036-a-stack-offers-findings-by-id-and-the-core-words-them.md) and
[ADR-0038](ADR-0038-the-network-stack-owns-the-sensors-and-the-core-keeps-the-judgement.md).

## Context
The owner has USB Wi-Fi adapters that can listen in monitor mode and asked how to use what they hear: identify, group, classify and show devices, their distance, movement and position, with fingerprints that can be
re-classified as more is learnt. Measured on vertex (2026-11-13): one adapter hears one band and cannot decode this router's Wi-Fi 6 data, so the number and kind of adapters matters and must not be assumed
(`docs/learnings/network-audit.md`, `docs/audits/2026-11-13-wifi-sensing-research.md`). The Network stack is IP and LAN; the data here (signal tracks, identity claims about people's devices, neighbours) has a different shape and a stricter
life cycle. Owner decision 2026-11-13: a new Radio stack.

## Decision
We will build **Radio as a third stack** (`stacks/radio`), separate from Network, owning its sensors, its tracker and its storage.

- **Sensors are generic.** A sensor is any discovered radio, with a plain `Capabilities` record; the stack takes one, many or none, and works from what each can do. A driver per radio kind implements one interface (Wi-Fi monitor first; Bluetooth,
  802.15.4 and SDR later). Capabilities the code does not use are listed in the UI as unused (by design, not yet built, or no driver). With no radio, and with the stack off, it does nothing.
- **Capture is the core's** (ADR-0034): a new helper action class `radio` in the closed catalogue under a new Constitution item (draft in the plan, §6); the stack passes back a grant through `PortPrivileged` and parses what comes back; a stack cannot add an action.
- **The core keeps the judgement** (ADR-0038): the stack reports typed facts and offers findings by id; the core words, rates, keys and places them, keeps alerts and the agent tools, and joins a radio device to Network's inventory by address (stacks never call each other).
- **The agent reads a typed digest, never frames.** Device-chosen strings are fenced (I.8); other people's devices appear only as counts by class.
- **Switchable like the others:** modules in the registry, the stack as a whole on or off.

## Consequences
- Network stays IP-shaped; Radio can grow to Bluetooth, 802.15.4 and SDR without touching it.
- The stack platform gets its third stack, so the stacks overview page (ADR-0028) is due.
- Two SDK additions: the `vault` port (ADR-0053) and a storage report (plan `data-management-page`).
- A cross-stack join goes through the core, which costs a port call per lookup but keeps IV.6 intact.
- A new Constitution item and a new helper unit class are needed before capture is built.

## Alternatives considered
- **A Radio layer inside Network** — reuses Devices, Findings and alerts at once, but mixes neighbour data into LAN stores with different retention, and Network is already the largest stack.
- **Hard-coded two adapters** — rejected by the owner; the count is the user's.
- **Kismet as the sensor** — complete, but an external dependency the owner wants to avoid; its datasource and device-record shape is the pattern we copy.

## Links
Plan [`radio-stack.md`](../PLANS/cross-cutting/radio-stack.md) · ADR-0053 · research `docs/audits/2026-11-13-wifi-sensing-research.md` · the measurement scripts (removed from the tree; recovery in the plan handoff).
