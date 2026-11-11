---
id: ADR-0047
title: Other devices' connections come from the router's connection table, read over SSH on the operator's press
status: proposed
date: 2026-11-12
subsystems: [network-audit]
supersedes:
superseded_by:
---

# ADR-0047: Other devices' connections come from the router's connection table, read over SSH on the operator's press

## Context

The Traffic tab shows this computer's own traffic. The owner asked for what the other devices connect to. Vertex's Wi-Fi card in managed mode sees only its own and broadcast frames (Constitution I.13), and the router's DNS log (I.8) has names and no addresses or bytes. The router is an Asus RT-AX86U on stock firmware (ADR-0018). On 2026-11-12 the owner measured it: with SSH on for the local network, `/proc/net/nf_conntrack` is readable, holds 245 lines, and its lines carry the device's own address in the first half (`src=192.168.50.220 dst=52.48.210.116 sport=55936 dport=8883`) with the router's WAN address rewritten only in the reply half, so each connection ties to a device. `nf_conntrack_acct` is off: no `bytes=` or `packets=` fields.

## Decision

We will read that file over SSH only when the operator presses a button, with `golang.org/x/crypto/ssh` (the module is already in `go.mod`; no new module), in `platform/routerssh`, on the terms of Constitution I.15: one private IP literal, one account, one key stored encrypted in the secrets store, the router's host key pinned after the operator compares the fingerprint (a handshake stopped before any credential is sent), one fixed command, a 4 MiB / 20 second / 20,000 line bound, step-up on every route that changes the setup or reaches the router. `core/netaudit/routerconn` parses each line into typed values (protocol, the device's address, the remote address and port, a tcp state from a fixed list), keeps only device-to-internet connections, groups them per device and place, names the places from the local tables and keeps the last result in memory. The Traffic tab's "Other devices" panel has two views: what they looked up (ADR-0018's watch) and where they connected. A place can be sent to the route trace card (ADR-0045). No byte counts are shown.

## Consequences

- Per-device connections with remote addresses, without changing the network, on stock firmware. Everything built for the computer's own traffic (operator, city, trace) applies to them.
- The program holds a key that can sign in to the router. The text of I.15 and the screen say to make a key for this alone and to limit SSH to the local network; the key is never shown again and is removed by a factory reset.
- A snapshot, not a history: connections that ended before the press are missing. The router's table is a hint, not evidence.
- Bytes need `nf_conntrack_acct` on, which does not survive a reboot on stock firmware; a later amendment may read `bytes=` when a line has it.

## Alternatives considered

- **The router's web API (`appGet.cgi`, Traffic Analyzer).** Undocumented, changes with firmware, needs the admin web password.
- **SNMP.** Interface counters only, no per-connection data (ADR-0023 rejects counters).
- **Port mirroring, or making vertex the gateway.** Changes the network.
- **Capturing other devices' Wi-Fi traffic.** A managed-mode card sees only its own and broadcast frames.
- **Trust on first use of the router's host key.** A forged router on first contact would receive the key's signature; the operator's fingerprint check costs one step.
- **A password instead of a key.** A worse secret to hold, and the router account's password would sit in the program.
- **Reading on a schedule.** A scheduled SSH login is a standing credential use; the press keeps it an explicit act, like the traffic look. *Amended 2026-11-12 by ADR-0048: the owner approved a nightly read made by the audit's own code, behind its own step-up switch, with no way for an agent to reach it.*

## Links

- Plan: `docs/PLANS/cross-cutting/network-traffic-more-handoff-2026-11-11.md` (B2); Constitution I.15.
- Code: `backend/internal/platform/routerssh/`, `backend/internal/core/netaudit/routerconn/`, `backend/internal/transport/http/handlers/netaudit_routerconn_handlers.go`, `frontend/src/components/netaudit/TrafficRouterSetup.vue` (the setup and the read) and `frontend/src/components/netaudit/TrafficDevicePane.vue` (the devices it shows).
