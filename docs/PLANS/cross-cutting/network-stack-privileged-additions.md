---
status: proposed
date: 2026-11-06
last_reviewed: 2026-11-07
related_specs: [SPEC-006]
constitution_references: [I.1, I.7, I.8, I.10, II.3, IV.6, V.1]
evidence: ADR-0023 (rejected, parked and built network ideas); docs/architecture/network-audit.md Appendix A (what packet capture would need); docs/services/llm-proxy.service (RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6, empty CapabilityBoundingSet); ADR-0030 (btmp with the utmp group). Read 2026-11-06 at origin/main 55466a3b. Nothing built or measured for this plan.
---

# The Network stack's privileged additions (blocked)

**Status:** proposed; **direction chosen 2026-11-07, nothing built.** These are the things ADR-0023 rejected or parked **because of a privilege** the
service's unit refuses. On 2026-11-07 the owner chose the path in §2a: a generic **core privileged helper**, opt-in, started by hand, never by the agent.
It is not a stack and not "a stack as its own process" (that stays parked, [`stacks-as-packages-and-network-stack.md`](stacks-as-packages-and-network-stack.md) §6),
and no stack may declare a privilege (Constitution IV.6, ADR-0033). Each item still needs the owner's per-item choice in §2a, its own Constitution item,
an ADR where there is a real alternative, and a measurement on vertex (§3, §5). Things rejected for low value stay rejected (§4).
**Related:** [ADR-0023](../../decisions/ADR-0023-network-ideas-rejected-or-already-built.md), [ADR-0032](../../decisions/ADR-0032-a-stack-is-a-package-with-a-manifest-kept-apart-by-tests.md),
[ADR-0033](../../decisions/ADR-0033-the-sandbox-is-core-and-stacks-gain-no-way-around-it.md).

## 1. Items

| # | Item | Privilege it needs | Value | Open questions |
|---|---|---|---|---|
| A | **Passive ARP, DHCP and mDNS listener.** Read link-layer headers of broadcast and multicast frames the host already receives on a switched LAN: real-time ARP-spoofing and gateway-MAC changes, a second DHCP server, a new device the moment it announces. Today the neighbour table is read on a timer. | `CAP_NET_RAW` and `AF_PACKET` (the unit allows only `AF_UNIX AF_INET AF_INET6` and has no capabilities) | Real-time detection without a mirror port, since the frames are broadcast | Headers only, no payload kept; a bound on frames per second; a Constitution item (I.7 says the program "never opens a raw socket", so this is a new rule); macOS uses BPF, unmeasured |
| B | **`nmap` UDP scans and OS detection** (parked in ADR-0023: need root). | `CAP_NET_RAW`; measure whether `nmap` needs more | Finds UDP services (DNS, SNMP, NTP) and device kinds | The scope stays private IPv4 only (II.3 item 3); a time and rate cap |
| C | **`auth.log` for SSH sources and usernames** (rejected in ADR-0023 for needing the `adm` group; ADR-0030 chose `btmp` with `utmp`). | The `adm` group, which also exposes every other system log | Where failed logins come from, not just when | Owner's decision on whether that width is acceptable in the one unit |
| D | **Readers for Zeek and Suricata logs.** Separate daemons the operator installs; the service reads their output directory through a group, as it reads `btmp`. | A read group on the log directory | Connection and DNS records, signature alerts | **No vantage point today:** ADR-0023 records no mirror port and no router login, so these tools would see only this computer. Suricata may use a JIT: check it against `MemoryDenyWriteExecute` first |

## 2a. Chosen direction (owner, 2026-11-07): a core privileged helper

**The owner's requirements:** the user decides whether to expose a level of privilege for an item; the privileged run is a **manual** trigger and never the
agent's; what it finds can reach the agent as data (through a store), without the agent ever holding the privilege; security is not weakened for anyone
who does not turn it on.

**Purpose (owner, 2026-11-07): a new process that runs certain networking commands outside the service's sandbox.** The point of the helper is to let a
fixed, reviewed set of networking actions (A to D in §1: raw capture, UDP scan and OS detection, log reading) run **outside the limits the service's unit puts on
the service and everything it starts** (an empty capability set, three address families, no log group), because those limits refuse the commands and must stay
as they are for everything else. Only the helper's process is outside them, and only for the catalogue's actions; the service, the agent's terminal and every
automation keep today's limits unchanged. **Meaning of "outside the sandbox", to be confirmed by the owner:** outside the *unit's privilege limits*; the
programs the helper starts are still file-jailed through `isolation` (item 2 below), which is the decision already made. If the owner means a program with no
file jail at all, that reopens item 2 and must be said explicitly. The purpose is part of the scope of N1 (the catalogue says, per action, which limit it
steps outside) and of N2, and it is why the helper has its own unit and gates (items 3 to 6 below) instead of a setting inside the service.

**Layers (why this is not a sandbox relaxation).** The sandbox (Landlock plus the Unix-socket filter) confines a child program inside the process tree. The
privileges in §1 are bounded by the service's **systemd unit** (`CapabilityBoundingSet=` empty, `NoNewPrivileges=true`,
`RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6`; `docs/services/llm-proxy.service`), so no relaxation inside the process can grant them. They have to
be held by something outside that unit.

**Rejected: file capabilities on `nmap`.** It works only if `NoNewPrivileges` is removed, which covers every process the service starts, so the agent's
terminal could run a privileged `nmap`. That contradicts "manual, not the agent".

**The design:**

1. **A generic helper, owned by the core.** A minimal binary beside the sandbox runner (privilege lives in the core, ADR-0033). It serves a **closed catalogue
   of actions in core code** (for example `udp_scan`, `os_detect`, `capture_window`, `read_auth_log`). Each action has a name, an argument schema, the privilege
   class (which unit runs it), a scope check, and time and rate caps. A new action is a core code change with its own Constitution item and ADR; a stack manifest
   cannot add one. Network is the first user; the mechanism is not Network-specific.
2. **The helper builds the arguments itself** and never forwards an argv it was given. It runs `nmap` under the same `isolation` jail, so a program gets the
   privilege and is still confined. It **re-checks scope itself** (private IPv4 and the declared ranges, II.3) and the caps; it does not trust the caller for safety.
3. **One unit per privilege class**, so the operator grants exactly what they choose: capture and scan (`CAP_NET_RAW`, `AF_PACKET`, no internet), and log reading
   (`adm` group, no network). Each unit is locked down on its own (almost no files, no internet for the capture unit, a system-call filter) and logs every request.
4. **On demand, never standing.** Socket activation starts the helper on the first connection and it exits when idle; a capture runs only for its window.
5. **Two gates, both off by default.** The unit must be installed (a root action by the operator; the service can never grant itself the privilege), and a
   per-action switch in settings is changed only with step-up.
6. **Manual only.** A stack asks through an SDK port, and the core honours it **only from a step-up request**: the route is declared `step_up` (built in P3b S1), and
   step-up already refuses admin tokens (`stepUpAllowed` returns 403 for a token principal), so automations and API tokens cannot trigger it. The core issues a
   **one-use grant bound to the exact action and target**. A fitness test (like `internal/archtest`) pins that only the handler and the composition root import the
   helper client, never `toolregistry`, `assistant` or automation code.
7. **Same-user reachability is a precondition, not an assumption.** The agent's children run as the service's user, so a check on who connects cannot tell them
   apart. Today they are kept out because the filter refuses `socket(AF_UNIX)` and Landlock hides the service's files; both go away if the operator enables
   `sandboxing.unix_sockets` or switches the terminal sandbox off. The helper client therefore refuses, and the UI says why, unless `Effective()` reports
   filesystem enforcement and Unix-socket denial. (The same-user finding is the advisor's reading; verify it in the first slice.)
8. **Untrusted data is parsed outside the privilege.** The helper returns raw, size-bounded bytes or fixed-size records; **all parsing and validation happen in the
   unprivileged service**, so a hostile packet or device name cannot reach privileged code. The core writes the typed result to a store the Network stack owns (a file
   declared in its manifest, P3b D10) and the agent reads it through the existing findings and read paths: the model sees typed fields, never packets (Appendix A).
9. **Item A is a time-boxed window, not always on:** capture runs for a fixed period (for example ten minutes) from a step-up trigger, because "always on" conflicts
   with "manual".

**Cost.** Nothing on an existing path goes through the helper (the device list, findings, the DNS check, every current scan), and the P3b baselines
([`network-stack-p3b-design.md`](network-stack-p3b-design.md) 4b) keep guarding those. A helper call is one local socket round trip plus, after idle, a process
start; **not measured**, expected to be negligible beside scans that take seconds to minutes. The real cost is operational: one more unit and an install step, paid
only by whoever turns an item on.

**Running it.**

| Where | How |
|---|---|
| Production (Ubuntu, systemd) | install a socket and service unit pair per privilege class beside the main unit (for example `llm-proxy-helper-net.socket`); the operator enables it as root; systemd starts it on the first connection. Extending `install.sh` is not designed yet (not read for this plan). |
| Local Linux, no unit | not installed, so the actions report unavailable and everything else works. To test the feature, start the helper by hand as root on a socket in a temporary folder. |
| macOS | no systemd and no Linux capabilities: reports unavailable, as the sandbox already does for its Linux-only guarantees. Linux first. |
| Tests | unit tests use a fake helper speaking the same plain-data protocol (no privilege, runs anywhere). The real privileged path is proved in a container with `--cap-add NET_RAW`, under the unit's own hardening (the `scripts/check-sandbox-under-unit.sh` approach), and on vertex. |

**Decisions the owner still makes** (the direction above is chosen, these are per item):

1. Which of A, B, C, D get a unit, and which are left out (C costs the `adm` group, which exposes every system log).
2. Accept the time-boxed capture window for A.
3. Accept a second unit per privilege class, installed by the operator.

**No-privilege alternatives to try first (unverified).** Item A's gateway and ARP changes may need no privilege: the kernel announces neighbour-table changes over
a netlink socket that an unprivileged process can, I believe, subscribe to; it would need only `AF_NETLINK` added to the allowed families, and it would not see
DHCP servers. The ledger notes no netlink socket was available on one deployment, so measure before relying on it.

## 2. The original ways forward (kept as the record)

1. **Widen the main unit on purpose** for one item, with its own ADR and Constitution item (for example one group for D). Each widening
   applies to the agent side too, which is the cost ADR-0029 to ADR-0031 already paid three times.
2. **Run a stack as its own process,** the parked idea, once an item justifies it (A and B are the cases that need a capability). Constitution
   IV.6 keeps it a contained piece of work; the cost list is in the parent plan §6.
3. **Leave the item out.**

## 3. Rule for every item

Off by default; a Constitution item merged by the owner before it runs; an ADR if there is a real alternative; measured on vertex before it
is called working; the model sees typed fields, never packets or payloads (Appendix A of the architecture doc). No item adds an external
program before the Go-only items (A, B, C) are done.

## 4. Stays rejected (value, not privilege)

The Feodo threat list (5 addresses when measured), a public-address change finding, version drift, new-destination alerts for this
computer's outbound connections, counters read from the host's own interfaces, router traffic counters (need UPnP, which the owner keeps off),
`dns-recursion`, `http-title`. nuclei is not reopened: the built-in web checks replaced it (ADR-0029). A proposal for any of these must first
say what changed (ADR-0023, Consequences).

## 5. What can be done now

**N0 - Measure on vertex (read-only, no code):** (1) what `nmap -sU` and `-O` need when run by hand with only `CAP_NET_RAW`, and whether they also need
`CAP_NET_ADMIN` or `--privileged`; (2) whether a raw `AF_PACKET` socket sees ARP and DHCP frames on the host's interface, and at what rate; (3) the permissions of
`auth.log`; (4) whether an unprivileged netlink neighbour subscription works once `AF_NETLINK` is allowed, and what it reports for a gateway MAC change; (5) the
`socket(AF_UNIX)` and Landlock behaviour of a child of the service, to confirm the same-user finding in §2a item 7. *Accept when:* the results are in
`docs/audits/` and the ledger. This gives the owner the facts for the per-item decisions. **Written 2026-11-07:** the runnable probes and the results template are [`docs/audits/2026-11-07-privileged-helper-n0.md`](../../audits/2026-11-07-privileged-helper-n0.md) (with its `2026-11-07-privileged-helper-n0/` folder); the owner runs them on vertex and pastes the masked results.

**N1 - Design the helper (docs only, after N0):** the action catalogue and its schema, the wire protocol (plain data), the grant, the unit files and the install step,
the `Effective()` precondition, the fitness test, and a proposed ADR ("the privileged helper is core, not a stack"). *Accept when:* the owner accepts the ADR; the
harness check passes.

**N2 - Build the helper with one action and its fake (after N1),** then each further item in its own change set, each with its Constitution item, an ADR where there is
a real alternative, and a measurement on vertex.

## Remaining Work

Direction chosen 2026-11-07 (§2a, purpose: run certain networking commands outside the service's sandbox through a core helper process, behind the gates); nothing built or measured. N0 probes written and run only in a Linux container (not vertex); next: the owner runs them on vertex and pastes the results, then N1, then N2. Independent
of P3b: the helper can be built in the core before slice S4 of [`network-stack-p3b-design.md`](network-stack-p3b-design.md), and must not be combined with S2.
