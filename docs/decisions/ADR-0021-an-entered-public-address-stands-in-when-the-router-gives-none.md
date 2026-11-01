---
id: ADR-0021
title: An entered public address stands in when the router gives none, and the router's own answer wins
status: accepted
date: 2026-11-01
subsystems: [network-audit]
supersedes:
superseded_by:
---

# ADR-0021: An entered public address stands in when the router gives none, and the router's own answer wins

## Context
[ADR-0019](ADR-0019-outside-lookup-is-opt-in-and-fails-open-to-unknown.md) took the public address only from the router's UPnP answer and rejected any other source. The owner turned UPnP off (the audit's own `upnp_enabled` finding advises it), so the Internet view can no longer look and carries its last finding for ever. That is the situation the ADR named as a consequence. The owner asked for a way to type the address in.

## Decision
- **The router's public answer wins.** The entered address is used only when UPnP gives none, or gives a private, shared (CGNAT) or link-local address (a router behind another router). If the router states a public address that differs from the entered one, the router's is used and the status says the entered one may be out of date (`differs`).
- **Only a public IPv4 address is kept**, validated by `outsidelookup.Classify` when it is saved and again when it is used, so a stored value that is no longer public (a hand-edited file) is ignored and nothing else is ever sent. An empty value clears it.
- **The status says where the address came from** (`source`: `router` or `entered`) and the card says "at the address you entered". The finding wording is unchanged: it still never contains the address, and `Evidence` is not part of the finding key, so no history churns. `Finding.Device` is unchanged (the router's LAN address).
- **It is step-up.** It rides the existing `PUT /admin/api/netaudit/outside-lookup` route, which is already in `stepUpExact`.
- **Where it lives.** `outside_lookup_address` in `settings.yml` (`models.NetauditConfig`), returned by `GET /admin/api/netaudit/outside-lookup` to a signed-in admin. It is not a secret, but it identifies the household, so it is never put in a finding, alert, log line or message.
- **Where the box appears.** In the simple view only when the last look could not be made because the router gave no usable address (`upnp_off`, `no_address`, `private_address`) or an address is already set; always in the advanced view. Nothing is shown for CGNAT or link-local, where an entered address would not be the household's.

## Consequences
With UPnP off the Internet view works again, but a provider that changes the address leaves a stale one: the router cannot contradict it when it is silent, so the hint says to update it. A wrong but public address makes the look report another household's ports; the operator typed it, and the card says which address source was used. A refused value is a 400 that says why and changes nothing.

## Alternatives considered
- **Ask a "what is my IP" service** — a second outside party learns the address (the reason ADR-0019 refused it).
- **Entered address always wins** — a stale entry would silently override a correct router answer.
- **A required cross-check against the router** — impossible while UPnP is off, which is the case this exists for.

## Links
- Amends [ADR-0019](ADR-0019-outside-lookup-is-opt-in-and-fails-open-to-unknown.md) (the address source). Plan: [`network-security-roadmap.md`](../PLANS/cross-cutting/network-security-roadmap.md) Phase 2.
