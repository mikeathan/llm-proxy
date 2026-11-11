---
id: ADR-0045
title: A route trace is unprivileged UDP probes on the operator's press, for an address the last look saw
status: proposed
date: 2026-11-11
subsystems: [network-audit]
supersedes:
superseded_by:
---

# ADR-0045: A route trace is unprivileged UDP probes on the operator's press, for an address the last look saw

## Context

On 2026-11-11 the owner asked where the traffic goes, "how many hops it does, location and info on each hop", like other popular tools (mtr, PingPlotter). A capture
(I.13) sees only the two ends of a conversation, never the routers between, so hops need active probes with a limited time to live. Constraints, read from the repo: the
service's unit allows `AF_UNIX AF_INET AF_INET6`, has no capabilities and no IP address filter (`docs/services/llm-proxy.service`); vertex's `ping_group_range` is "1 0", so
an unprivileged ICMP socket is refused; nmap may not touch public addresses (scan scope); `NetworkTools` cannot set a time to live.

Measured on 2026-11-11: an unprivileged UDP socket with `IP_RECVERR` on Linux (Docker, `golang:1.26`, uid 65534) gets the kernel's "port unreachable" from loopback and the
first router's "time exceeded" (`172.17.0.1`, 0.9 ms); hops past it were silent because Colima's user-mode NAT drops ICMP answers, while the Mac's own traceroute over the
same link saw hops 1, 2, 3 and 6. So the mechanism works unprivileged; a full trace is first proven on vertex.

## Decision

We will trace a route with `platform/routeprobe`: one UDP datagram of 24 zero bytes per probe, time to live 1 to 30, ports 33434 and up, three probes a distance, one
second each, the answer read from the socket's error queue (`IP_RECVERR` / `IPV6_RECVERR`), Linux only. `core/netaudit/traceroute` runs one trace at a time on the
application's lifecycle context (started from `netauditLoops`), stops at the answer, an unreachable, five silent distances, 90 seconds or shutdown, names each internet
router from the owner and city tables kept on this computer, and keeps the last eight traces in memory. The route is a step-up `POST /admin/api/netaudit/route-trace`
for an address the last kept look listed, while the guardrails allow internet access and the blocked list does not name it. Constitution I.14 carries the bounds.

## Consequences

- The Traffic detail of an internet place shows the route step by step, with "no answer" for silent routers, and the map draws a line through the routers with a city.
- A trace sends new traffic towards the internet; it needs the amendment and the passphrase, and it cannot probe an address the operator never talked to.
- It does not work on macOS (no unprivileged way to read the answers): the screen says so.
- Backbone routers are often given the wrong city by the table; the screen says a city is approximate and the line starts at the first router with a city.
- Nothing persists: a restart forgets the traces.

## Alternatives considered

- **Unprivileged ICMP echo (`SOCK_DGRAM`, `IPPROTO_ICMP`).** Refused on vertex (`ping_group_range` "1 0"); changing a sysctl for this is the owner's host, not ours.
- **A helper action with a raw socket.** More privilege than needed: the UDP + `IP_RECVERR` path needs none.
- **Run `traceroute` or `mtr` as a guarded program.** A new binary to install and parse, and the scan-scope rule forbids public targets for guarded programs.
- **A per-hop online lookup (ipinfo, RIPE).** Sends every router of the path to a third party; the tables kept on this computer answer it (ADR-0043, ADR-0044).
- **Trace any address the operator types.** Makes the page a general probing tool; tying it to the last look keeps it an explanation of traffic that already happened.

## Links

- Plan: `docs/PLANS/cross-cutting/network-traffic-view-redesign.md`; Constitution I.14.
- Code: `backend/internal/platform/routeprobe/`, `backend/internal/core/netaudit/traceroute/`, `backend/internal/transport/http/handlers/netaudit_routetrace_handlers.go`.
