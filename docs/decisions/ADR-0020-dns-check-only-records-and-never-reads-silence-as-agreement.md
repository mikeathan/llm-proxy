---
id: ADR-0020
title: The DNS check only records, compares sets over DNS-over-HTTPS, and a missing answer is never read as agreement
status: accepted
date: 2026-11-01
subsystems: [network-audit]
supersedes:
superseded_by:
---

# ADR-0020: The DNS check only records, compares sets over DNS-over-HTTPS, and a missing answer is never read as agreement

## Context
A router that has been taken over can answer DNS with attacker addresses. The router's DNS servers are not exposed by UPnP, but a name can be asked of the router and of an outside resolver and the answers compared. That raises false alarms for names whose address varies by place and time (CDNs), so the roadmap (Phase 3) allows only a narrow form: names chosen for stable answers, a collector that logs and raises nothing, and a finding only if a week of real comparisons shows no false alarms. Real captures (2026-11-01): the router and Cloudflare's JSON endpoint gave the same address sets for `dns.google`, `one.one.one.one` and `dns.quad9.net`, in a different order and with different TTLs.

## Decision
- **Record only.** The collector logs each comparison (name, outcome, whether the answers shared an address, both address lists) and raises no finding, alert or notification. It stays this way until the owner has seen a week of results and chosen a rule; no check title, remediation or guide exists until then.
- **Off by default, step-up to switch on** (`PUT /admin/api/netaudit/dns-check`, in `stepUpExact`): the outside resolver learns the names asked and the public address. The switch says so.
- **Compare sets, not order**; IPv4 address records only; each answer capped at 16 addresses.
- **A side that gave nothing is "could not compare"**, with a fixed reason (`no_router`, `router_no_answer`, `outside_failed`), never a match and never a difference. The outside resolver is not asked when the router gave no answer. With internet access off, or local network access off (both are off by default in Guardrails), the cycle stops and logs nothing and the card says which setting stopped it; a refusal is not a comparison.
- **The outside question is DNS-over-HTTPS through the guarded client** (`NetworkTools.OpenAccepting`, Constitution I.2; no amendment). A plain UDP query to an outside address could be answered by the compromised router itself and would hide the thing being looked for. The router is asked with the standard library's resolver over the guarded dialer, port 53 of the default gateway only.
- **Everything is a setting**: names (1 to 5, validated host names), the resolver address (https only), the interval (15 minutes to 6 hours, default 60), and the router's DNS address (empty means the default gateway found from the host's routing table, so it works the same on macOS and Linux; when set it must be a private IPv4 address, so a loopback stub such as systemd-resolved's `127.0.0.53` and any public address are refused, and plain UDP never leaves the local network).
- **Nothing a resolver says is kept** except parsed addresses; names come from the operator's list.
- The log is one file, bounded to 2000 rows. The owner brings back the summary (counts and the newest differences), which the card can copy.

## Consequences
It catches wholesale redirection of the chosen names, not a router that lies about one site, and not one that proxies the outside question too. A name pinned in `/etc/hosts` on the host is answered locally by the Go resolver. About 72 outside requests a day at the default interval for three names. Whether a rule can be quiet is unknown until the week's data is read; if it cannot be, the phase is dropped and the collector removed.

## Alternatives considered
- **Read the router's address only from the gateway** — a network that hands out a different DNS server (a Pi-hole, a mesh system) would read "could not compare" for ever.
- **Plain UDP to a public resolver** — interceptable by the router under test.
- **Compare against the system resolver** — on Linux it is often a local stub (systemd-resolved) and on macOS mDNSResponder, not the router.
- **A hand-written DNS parser** — the ledger records hand-decoding errors; the standard library does it.
- **Raise a finding from the first difference** — CDNs and rotated answers would produce false alarms; the week decides.
- **Nightly only** — seven samples cannot show "zero false alarms".

## Links
- Plan: [`network-security-roadmap.md`](../PLANS/cross-cutting/network-security-roadmap.md) Phase 3.
- Related: [ADR-0019](ADR-0019-outside-lookup-is-opt-in-and-fails-open-to-unknown.md), [ADR-0013](ADR-0013-vulnerability-data-import-is-step-up.md).
