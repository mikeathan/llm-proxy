---
id: ADR-0034
title: The privileged helper is core, not a stack, and it decides who is calling
status: proposed
date: 2026-11-07
subsystems: [network-audit, guardrails, platform]
supersedes:
superseded_by:
---

# ADR-0034: The privileged helper is core, not a stack, and it decides who is calling

Proposed: it is in force when the owner accepts it. The owner approved the design's decisions D1 to D9 on 2026-11-07 (design section 12); acceptance of this ADR is the merge of the pull request that carries it and the Constitution item I.11. Companion of [ADR-0032](ADR-0032-a-stack-is-a-package-with-a-manifest-kept-apart-by-tests.md) and
[ADR-0033](ADR-0033-the-sandbox-is-core-and-stacks-gain-no-way-around-it.md).

## Context
Some Network checks need what the service's systemd unit refuses (`docs/services/llm-proxy.service`: an empty capability set, `AF_UNIX AF_INET AF_INET6`, no log group): UDP
scans and OS detection, a raw capture of ARP and DHCP, reading `auth.log`. ADR-0023 rejected or parked them, ADR-0033 says a stack gains no way around the sandbox, and the
owner (2026-11-07) chose a **generic helper process, owned by the core, that runs a closed list of networking actions outside the service's sandbox** while the service and the agent keep today's limits.

Measured on vertex (2026-11-07, `docs/audits/2026-11-07-privileged-helper-n0.md`): `nmap -sU` and `-O` need `CAP_NET_RAW`, `AF_PACKET`, `AF_NETLINK` and `--privileged`, and no
`CAP_NET_ADMIN`; the agent's children run as the service's account (uid 995, gid 982), Landlock ABI 8 still lets them connect to a socket outside their grant, and the
seccomp filter that refuses `socket(AF_UNIX)` is the only thing between them and a local socket; a descriptor connected before the filter keeps working. A check of the
caller's group or uid therefore cannot keep a child out.

## Decision
We will build the privileged helper as **core code, not a stack and not a stack as its own process**, with these rules:
1. **A closed catalogue of actions in core code,** each with typed parameters, arguments built by the helper (never an argv it was given), a scope check, caps and a jail profile. A stack's manifest cannot add or widen one (IV.6, ADR-0033).
2. **One systemd unit per privilege class,** started on demand by socket activation with one process per connection, installed by the operator as root, and holding only what its class needs (`net`: `CAP_NET_RAW` and the five address families; `log`: the `adm` group and no network). The helper is a separate small binary under an import allowlist.
3. **The helper decides who is calling:** it accepts a request only from the service's main process (peer process handle against systemd's `MainPID`), not from a group, uid, parent or cgroup; the rule holds only if a child cannot ptrace the service, write its memory or steal its connected descriptor, which N2 measures on vertex first. The service's own refusal to call unless the sandbox reports file confinement and Unix-socket denial is a safety interlock, not the control.
4. **Manual only:** a step-up request mints a one-use, in-memory grant bound to the action and its typed parameters, minted by the core for a step-up route that declares the action, with the body being exactly those parameters; admin tokens already get 403 on step-up routes. Each action also has a settings switch changed only by step-up, and the unit's action list is a root-owned third gate. All are off by default.
5. **Programs the helper starts run under `isolation`** (file jail, no Unix sockets) and the helper checks the target against its own fixed rule (one private IPv4 host) and never trusts the caller for it. The operator's declared scan ranges are enforced by the service before it asks (owner decision, 2026-11-07): a root-held copy would only be copied from a file the service can write, and would force a re-run after every change. The `MainPID` query is a jailed program (Unix sockets, no network, no capability).
6. **Untrusted data is parsed outside the privilege:** the helper returns bounded raw bytes or fixed-size records; the unprivileged service parses them into typed fields in a store the stack owns.
7. **The Constitution is amended before the first action runs** (I.7's "never a raw socket", a new item for the helper, IV.6's wording); the drafts are in the design (§9).

## Consequences
- The blocked checks become possible for whoever installs a unit and turns an action on; everyone else's security is unchanged, and no existing path goes through the helper, so the P3b baselines do not move.
- The helper holds more power than the service. Its protection is the caller rule, the closed catalogue and the unit's confinement; **if the service process itself is compromised the helper cannot tell**, and what remains is the catalogue, the helper's fixed scope rule and caps, and the three gates (a compromised service can ask the helper to scan any private host, which it can already do unprivileged).
- A new operational piece: a binary, a socket and a service per class, an install step in `setup.sh`, and an under-unit check on every release that touches the sandbox.
- The `log` class costs the `adm` group, which reads 67 files on vertex; it is separable and the owner may decline it.
- The caller rule needs `MainPID` from systemd (design D3), a kernel with a peer process handle for the race-free form (Linux 6.5 and later), and `ptrace_scope` and the unit's syscall filter to keep same-account takeover out (design §2, D8); measured on vertex 2026-11-07: `ptrace_scope` 1, none of the four takeover calls in the filter, and the real agent terminal could not create a Unix socket ([audit](../audits/2026-11-07-privileged-helper-n0.md)).
- The SDK gains two plain-data fields (a route's `Action` and a request's `Grant`) in N2a; a stack still declares no privilege.
- Not settled here, and decided before N2: the netlink neighbour listener (no capability, so it may belong in the main unit instead), the capture window and frame cap, and whether ambient `CAP_NET_RAW` survives the runner's re-exec inside the unit (an N2 test).

## Alternatives considered
- **Widen the main unit** (add `AF_PACKET`, `CAP_NET_RAW` or `adm`) — every child inherits it, so the agent's terminal could run a privileged `nmap` or read the system logs; ADR-0029 to ADR-0031 already paid this cost three times.
- **File capabilities on `nmap`** — works only with `NoNewPrivileges` removed, which covers every process the service starts.
- **A stack as its own process** (the parked idea in ADR-0032) — it makes the privilege a property of one feature and invites a manifest that asks for it; the privilege is the core's to hold.
- **Trust the socket's group or the peer's uid** — measured identical for the service and its children, so it protects nothing.
- **A shared secret in a file or the environment** — readable by the same account, and the jail's read list includes `/proc`.
- **A D-Bus client or polkit rule for the caller check** — a new dependency (AGENTS asks first) for one integer; the design prefers a fixed `systemctl show` call (D3).
- **Run everything as root** — no.

## Links
- Design: [`network-privileged-helper-design.md`](../PLANS/cross-cutting/network-privileged-helper-design.md). Plan: [`network-stack-privileged-additions.md`](../PLANS/cross-cutting/network-stack-privileged-additions.md). Audit: [`2026-11-07-privileged-helper-n0.md`](../audits/2026-11-07-privileged-helper-n0.md).
- Constitution I.5, I.7, I.10, II.3, IV.6; ADR-0023, ADR-0030, ADR-0031, ADR-0033.
