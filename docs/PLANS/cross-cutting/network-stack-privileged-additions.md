---
status: proposed
date: 2026-11-06
last_reviewed: 2026-11-06
related_specs: [SPEC-006]
constitution_references: [I.1, I.7, I.8, I.10, II.3, IV.6, V.1]
evidence: ADR-0023 (rejected, parked and built network ideas); docs/architecture/network-audit.md Appendix A (what packet capture would need); docs/services/llm-proxy.service (RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6, empty CapabilityBoundingSet); ADR-0030 (btmp with the utmp group). Read 2026-11-06 at origin/main 55466a3b. Nothing built or measured for this plan.
---

# The Network stack's privileged additions (blocked)

**Status:** proposed, and **blocked** (2026-11-06). These are the things ADR-0023 rejected or parked **because of a privilege** the service's
unit refuses. The owner left out running a stack as its own process ([`stacks-as-packages-and-network-stack.md`](stacks-as-packages-and-network-stack.md) §6),
and no stack may declare a privilege (Constitution IV.6, ADR-0033), so none of this can be built until one of the paths in §2 is chosen.
Things rejected for low value stay rejected (§4).
**Related:** [ADR-0023](../../decisions/ADR-0023-network-ideas-rejected-or-already-built.md), [ADR-0032](../../decisions/ADR-0032-a-stack-is-a-package-with-a-manifest-kept-apart-by-tests.md),
[ADR-0033](../../decisions/ADR-0033-the-sandbox-is-core-and-stacks-gain-no-way-around-it.md).

## 1. Items

| # | Item | Privilege it needs | Value | Open questions |
|---|---|---|---|---|
| A | **Passive ARP, DHCP and mDNS listener.** Read link-layer headers of broadcast and multicast frames the host already receives on a switched LAN: real-time ARP-spoofing and gateway-MAC changes, a second DHCP server, a new device the moment it announces. Today the neighbour table is read on a timer. | `CAP_NET_RAW` and `AF_PACKET` (the unit allows only `AF_UNIX AF_INET AF_INET6` and has no capabilities) | Real-time detection without a mirror port, since the frames are broadcast | Headers only, no payload kept; a bound on frames per second; a Constitution item (I.7 says the program "never opens a raw socket", so this is a new rule); macOS uses BPF, unmeasured |
| B | **`nmap` UDP scans and OS detection** (parked in ADR-0023: need root). | `CAP_NET_RAW`; measure whether `nmap` needs more | Finds UDP services (DNS, SNMP, NTP) and device kinds | The scope stays private IPv4 only (II.3 item 3); a time and rate cap |
| C | **`auth.log` for SSH sources and usernames** (rejected in ADR-0023 for needing the `adm` group; ADR-0030 chose `btmp` with `utmp`). | The `adm` group, which also exposes every other system log | Where failed logins come from, not just when | Owner's decision on whether that width is acceptable in the one unit |
| D | **Readers for Zeek and Suricata logs.** Separate daemons the operator installs; the service reads their output directory through a group, as it reads `btmp`. | A read group on the log directory | Connection and DNS records, signature alerts | **No vantage point today:** ADR-0023 records no mirror port and no router login, so these tools would see only this computer. Suricata may use a JIT: check it against `MemoryDenyWriteExecute` first |

## 2. Ways forward (a decision for the owner, not made here)

1. **Widen the main unit on purpose** for one item, with its own ADR and Constitution item (for example one group for D). Each widening
   applies to the agent side too, which is the cost ADR-0029 to ADR-0031 already paid three times.
2. **Run a stack as its own process,** the parked idea, once an item justifies it (A and B are the cases that need a capability). Constitution
   IV.6 keeps it a contained piece of work; the cost list is in the parent plan §6.
3. **Leave the item out.**

## 3. Rule for every item, whichever path is chosen

Off by default; a Constitution item merged by the owner before it runs; an ADR if there is a real alternative; measured on vertex before it
is called working; the model sees typed fields, never packets or payloads (Appendix A of the architecture doc). No item adds an external
program before the Go-only items (A, B, C) are done.

## 4. Stays rejected (value, not privilege)

The Feodo threat list (5 addresses when measured), a public-address change finding, version drift, new-destination alerts for this
computer's outbound connections, counters read from the host's own interfaces, router traffic counters (need UPnP, which the owner keeps off),
`dns-recursion`, `http-title`. nuclei is not reopened: the built-in web checks replaced it (ADR-0029). A proposal for any of these must first
say what changed (ADR-0023, Consequences).

## 5. What can be done now

**N0 - Measure on vertex (read-only, no code):** what `nmap -sU` and `-O` need when run by hand with only `CAP_NET_RAW`; whether a raw
`AF_PACKET` socket sees ARP and DHCP frames on the host's interface, and at what rate; the permissions of `auth.log`. *Accept when:* the
results are in `docs/audits/` and the ledger. This gives the owner the facts for §2.

## Remaining Work

Everything. N0 can run at any time; the rest waits for the owner's choice in §2.
