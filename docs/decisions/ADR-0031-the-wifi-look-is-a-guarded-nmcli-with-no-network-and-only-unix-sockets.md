---
id: ADR-0031
title: The Wi-Fi look-alike check runs nmcli as a guarded program with no network and only Unix sockets, after the jail's Unix-socket gap is closed
status: proposed
date: 2026-11-06
subsystems: [network-audit, guardrails]
supersedes:
superseded_by:
---

# ADR-0031: The Wi-Fi look-alike check runs nmcli as a guarded program with no network and only Unix sockets, after the jail's Unix-socket gap is closed

Proposed: it is in force when the owner merges the pull request that carries Constitution II.3's new bullets.

## Context
The owner asked for an evil-twin check: tell me when an access point advertises my network's name from an address that is not mine. vertex has a Wi-Fi card (`wlp2s0`, not associated). A scan through netlink is impossible under the service unit (`RestrictAddressFamilies` has no `AF_NETLINK`, the capability set is empty; an `iw` scan under the unit's families failed with "Failed to connect to generic netlink"). NetworkManager and `wpa_supplicant` are active on vertex, and `nmcli -t -f SSID,BSSID,SIGNAL,SECURITY device wifi list --rescan no` ran under the unit's restrictions as `llm-proxy` and listed 27 access points (owner's run, 2026-11-06). `nmcli general permissions` for the same user answered `no` or `auth` for every state-changing action, a rescan included.

The jail that guards children does not stop a connection to a path-based Unix socket (pitfall 67, measured on kernel 6.8, Landlock ABI 4), so any child can already reach the system D-Bus. That made the question "does this widen the sandbox" the wrong one and "what does the sandbox let a child do over a socket" the right one.

## Decision
We will (1) close the gap: the runner installs a seccomp filter that refuses `socket(AF_UNIX)` and `io_uring_setup` for every child unless the spawn site allows Unix sockets or the operator sets `sandboxing.unix_sockets: true`; and (2) run `nmcli` as a guarded program (`core/netaudit/wifilook`) that is the one program allowed Unix sockets, with fixed arguments, a clean environment, and the Landlock TCP denial (`progexec.Invocation.NoNetwork`), so it can talk to NetworkManager and nothing else. The look reads NetworkManager's list; it never asks for a scan. The names are judged only against names the operator typed, and only hardware addresses the operator confirmed from a look are trusted.

## Consequences
- Jailed children lose Unix sockets by default. A program that makes one itself (tmux, an ssh ControlMaster, syslog, a D-Bus or docker client) gets "permission denied" until the operator sets `sandboxing.unix_sockets: true`. `socketpair` and TCP are unaffected.
- `nmcli` is the only child with a path to the system bus. Polkit, not the jail, decides what it may do there; if a later NetworkManager or polkit policy gave the service user a state-changing action, a compromised `nmcli` could use it. The check of that permission list belongs in the readiness notes (`nmcli general permissions`).
- The filter is written for linux/amd64 and linux/arm64 and tested on arm64 only (kernel 6.8); an amd64 mistake would kill every wrapped child at its first call, so the first check after deploying is that an agent terminal command runs.
- The module needs `nmcli` (the `network-manager` package), a confined sandbox (or the operator's opt-in), and a network name the operator has typed.
- The look reads what NetworkManager scanned; NetworkManager may not be scanning (an associated card scans rarely). A quiet look is not "no twin": the card says when the last look was made.

## Alternatives considered
- **Netlink scan in the service** — needs `AF_NETLINK` and `CAP_NET_ADMIN`, which would let the service reconfigure networking.
- **A D-Bus client inside the service** — a new dependency that parses air-controlled data inside the process that holds the secrets, with no jail.
- **`nmcli` with the old jail and no Unix-socket denial** — it works, but leaves every other child able to do the same.
- **Allowing `nmcli` a Landlock read path to `/run/dbus`** — unnecessary: Landlock does not control the connection, and adding the path would only suggest it did.
- **Alerting on every unknown address of any name** — the neighbours' networks are not the operator's business and would be kept and shown.

## Links
- Plan: `docs/PLANS/cross-cutting/network-wifi-lookalikes-and-unix-socket-jail.md`. Pitfall 67. Constitution II.3 (proposed bullets). SPEC-006 §II.7.6. Ledger: `docs/learnings/network-audit.md` (2026-11-06).
