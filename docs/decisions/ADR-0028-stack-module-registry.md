---
id: ADR-0028
title: Switchable capabilities are modules in stacks, described by one registry and switched in one place
status: accepted
date: 2026-11-05
subsystems: [network-audit, admin-ui]
supersedes:
superseded_by:
---

# ADR-0028: Switchable capabilities are modules in stacks, described by one registry and switched in one place

## Context
By v0.47.1 the Network module had twelve switches (presence check, scan on arrival, device names, IPv6 discovery, decoys, router log,
Internet view, router DNS check, Nearby, weekly summary, daily check-in), each with its own setting, handler, route, step-up entry and
settings card, and dependencies only implied (scan on arrival and the weekly summary need the presence check; Nearby and the DNS check
follow the Guardrails network switches). Agent safety features had no switch at all. The owner could not see what runs, what it sends or
what needs what, and asked for "stacks" of "togglable and modular" capabilities, with the service expected to become a stack platform.
Owner decisions D1–D6 are in plan `stack-modules-and-new-detection` §1.

## Decision
We will describe every switchable capability as a **module** in one registry (`internal/core/stack`), grouped as **Stack → Layer →
Module**: a stack is a top-level area (Network, Agents), a layer is a stack's tier, and a module may depend only on modules of its own stack
on its own layer or below. Each module states its stack, layer, purpose, what it sends (nothing, LAN, internet, listens), whether
switching it needs the passphrase, what it needs, its health (a `MonitorPart` id) and its readiness, and switches through an adapter to the
setting that already holds it: no persisted setting moves. Each stack's **Modules tab** is the one place to switch its modules (the
owner's one-path rule); detail pages keep only details. One API lists and switches modules, and **checks step-up per module in the
handler**, because a single route cannot be listed per module in `api.stepUpExact`. A stack may also declare actions (lock down for
Agents); an action is never a module.

**Relation to ADR-0022.** This amends ADR-0022 in one point only: the navigation group is named **Stacks**, which ADR-0022 had
rejected; the owner chose it on 2026-11-05 for the stack platform the service is becoming. The rest of ADR-0022 stands and is how this is
built: a stack keeps its own settings on its own page (its Modules tab is the switchboard, its Settings tab the details), URLs stay under
the stack's path, and no overview page exists until there are several stacks (Network and Agents are two; the overview comes with a third).

## Consequences
- Adding a capability means registering one module; the API and the Modules tab need no new code. A new stack is a registration too.
- The registry is descriptive: features keep their own packages, settings and detail routes, so it can never become a second source of
  truth for a setting.
- A test must walk every module marked as needing the passphrase and see it refused without it, or the single route weakens step-up.
- The switches leave Network → Settings; the old per-feature `PUT` routes stay for details and keep their own step-up.
- Neutral names (`stack`, `module`) keep the later rename (OneStack or similar) independent of this change.

## Alternatives considered
- **A status-only page linking to the existing switches** — leaves two paths per job; the owner chose one place to switch (D1).
- **Moving every switch into a new `stack` settings block** — a migration of persisted settings for no user benefit, and two sources of
  truth during it.
- **One flat list without stacks** — does not fit the stack platform the owner described; the second stack (Agents) is needed now.

## Links
- Plan [`stack-modules-and-new-detection.md`](../PLANS/cross-cutting/stack-modules-and-new-detection.md) (it links the owner's mock-up).
- Amends [ADR-0022](ADR-0022-modules-is-the-navigation-group-for-feature-areas.md).
