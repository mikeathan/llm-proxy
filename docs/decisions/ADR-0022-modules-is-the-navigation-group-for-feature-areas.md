---
id: ADR-0022
title: Modules is the navigation group for self-contained feature areas, and a page keeps its own settings
status: accepted
date: 2026-11-02
subsystems: [admin-ui, network-audit]
supersedes:
superseded_by:
---

# ADR-0022: Modules is the navigation group for self-contained feature areas, and a page keeps its own settings

## Context
The Network page was added as an eighth sidebar entry, and its switches were spread over three places: the Devices tab (the new-device check, device names, IPv6, decoys, router log, internet view, DNS check), the Overview (weekly summary, notices in this browser) and Settings → Safety → Network scan (what may be scanned, vulnerability data, scan time, confinement). The owner expects more feature areas of this kind (smart home, messaging views, memory views), and a flat sidebar of eight entries on a phone already needs a More sheet. See the plan [`network-modules-redesign`](../PLANS/cross-cutting/network-modules-redesign.md) and SPEC-003 changelog 2.24.

## Decision
- We will group self-contained feature areas under one navigation heading, **Modules**: an expandable group in the sidebar (open by default, the fold remembered, open while a destination in it is) and a heading in the phone's More sheet. Network is the first destination. No placeholder destination is shown for a module that does not exist.
- A module's URLs stay under its own path (`/network/...`); there is no `/modules` page until there are several.
- A module keeps **its own settings on its own page** (a Settings tab), not in the global Settings. Network's Settings tab holds every switch and limit of the page; Settings → Network scan is removed, not redirected, because the product is new and carries no compatibility shims.
- The name is generic on purpose. Rejected names: *Infrastructure* (Network is the only member and Messaging is not infrastructure), *Tools* (collides with the model's tools), *Connections* (collides with the Connectors settings group), *Stack*.
- Memory stays a workspace section for now; whether it joins Modules is a later decision.

## Consequences
A new module adds one navigation entry with `group: 'modules'` and its page; the sidebar, the More sheet and the unread badges follow from that one list. A module's settings are found where the module is, but global Settings no longer shows everything configurable. A readiness fix the server names as `network-scan` is read by the UI as the Network Settings tab; the server may keep that word.

## Alternatives considered
- **A flat list of nine entries** — works until the next module, then the phone's More sheet becomes a long unlabelled list.
- **Keep the settings in Settings** — one place for all configuration, but the page then points away from itself for every change and cannot say what to do on the spot.
- **A redirect from the old settings tab** — a shim for a path nobody has bookmarked.

## Links
- Plan: [`network-modules-redesign`](../PLANS/cross-cutting/network-modules-redesign.md)
- SPEC-003 changelog 2.24
