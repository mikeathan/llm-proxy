---
id: ADR-0019
title: The outside lookup of open ports is opt-in, sends only the router's stated public address, and never reads a failure as "nothing is open"
status: accepted
date: 2026-11-01
subsystems: [network-audit]
supersedes:
superseded_by:
---

# ADR-0019: The outside lookup of open ports is opt-in, sends only the router's stated public address, and never reads a failure as "nothing is open"

## Context
The audit can say what is open on devices inside the network, not what the internet can reach. An outside scan database can (roadmap Phase 2). Using one sends something out: the network's public address, which identifies the household. The router can state that address through UPnP (`GetExternalIPAddress`), which the router check already speaks to. Real captures (2026-11-01): the router answered with a public address; the service (Shodan InternetDB, keyless, free for non-commercial use, refreshed about weekly) answered `200` with `{"ports":[1194],...}` for the owner's address and `404 {"detail":"No information available"}` for an address it has nothing on. Its full terms could not be read (the terms page returned 404); the rate limit is not stated.

## Decision
- **Off by default, step-up to switch on** (`PUT /admin/api/netaudit/outside-lookup`, in `stepUpExact`), with a switch that says the outside service learns the public address. One request per nightly audit (a look made in the last six hours for the same address is reused, because a rescan of one device runs the router check too), through the guarded client (`NetworkTools.Open`), so Constitution I.2 holds and no amendment is needed.
- **The address comes only from the router's own UPnP answer** (asked by the lookup itself, only when it is switched on) and is sent only if it is public. A private, carrier-grade (100.64/10), link-local or unparseable address is never sent: the check records "cannot look" with a fixed reason. The address is not guessed, not read from a web service, and never put in a finding, alert or log line.
- **The service is a setting** (`outside_lookup_url`, https only, default InternetDB), so another service with the same answer shape can replace it. Only the `ports` list is read; `vulns`, `cpes`, `hostnames` and `tags` are the service's guesses or outside text and are dropped. Ports are validated to 1–65535 and capped at 256.
- **Findings** `internet_exposed_port` are keyed on the router's LAN address (like `upnp_enabled`), one per port, in fixed wording that says what the database lists and whether an enabled UPnP mapping accounts for it, and never names a cause (double NAT, a DMZ and a manual forward look the same from here). They say the information can be a week old.
- **A look that could not be made is "unknown", never "clean".** The status (looked, cannot look with a reason, failed) is shown on the card, and the earlier findings of this check are carried into the next report (`Exposure.Carry`) while the last attempt could not look, so a night the service is down or UPnP is off does not report every exposed port as fixed. Switching the lookup off drops them.
- With internet access off in the network settings (the default) it says so and sends nothing (`internet_off`). A `404` from the service is its answer for "no data" and is read as an empty list; any other non-200 is an error (`tools.StatusError`).

## Consequences
The look works only while the router's UPnP is on, and the audit's own `upnp_enabled` finding advises turning UPnP off, so in the recommended state the card says it cannot look. On the owner's double-NAT setup the address is the provider box's, and a port the service sees may be forwarded by that box and not by the router, which the wording allows for. Findings can lag a fix by up to the service's refresh time. The status is in memory only and is empty after a restart until the next audit.

## Alternatives considered
- **Ask a "what is my IP" web service for the address** — a second outside party learns the address, and for a household behind another NAT it would return the wrong one.
- **Always on** — sends the address without the operator's choice.
- **Match the service's text for "not found"** — brittle; the typed status error carries the code.
- **Report only on a successful look and say nothing otherwise** — silence reads as safe; the card states why it could not look.
- **Use `vulns` and `cpes` for severity** — they are guesses by the service, not evidence of this network.

## Links
- Plan: [`network-security-roadmap.md`](../PLANS/cross-cutting/network-security-roadmap.md) Phase 2.
- Related: [ADR-0013](ADR-0013-vulnerability-data-import-is-step-up.md) (step-up download through the guarded client), [ADR-0010](ADR-0010-no-level-2-assisted-fixes.md).
