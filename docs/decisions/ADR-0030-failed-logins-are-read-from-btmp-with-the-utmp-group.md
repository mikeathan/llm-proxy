---
id: ADR-0030
title: Failed logins to this computer are read from btmp, with the utmp group, and only their times
status: proposed
date: 2026-11-06
subsystems: [network-audit]
supersedes:
superseded_by:
---

# ADR-0030: Failed logins to this computer are read from btmp, with the utmp group, and only their times

Proposed: it is in force when the owner merges the pull request that carries Constitution I.10.

## Context
The Network module should tell the operator when someone keeps failing to log in to the computer it runs on (vertex accepts SSH passwords, the `host_ssh_password_login` finding). The service runs under a hardened systemd unit (`docs/services/llm-proxy.service`): `ProtectSystem=strict`, an empty capability set, no `AF_NETLINK`, and it is in no log-reading group (`id llm-proxy` on vertex, 2026-11-06: groups `llm-proxy`, `video`, `render`). Two sources hold the data. `/var/log/auth.log` is text, owned by `syslog` with group `adm` and mode 640 on Ubuntu. `/var/log/btmp` is a binary file of fixed 384-byte records that `sshd` and `login` append to on each failed password login; it is mode 660 and group `utmp`.

Measured on vertex (2026-11-06): under `systemd-run -p User=llm-proxy -p SupplementaryGroups=utmp -p ProtectSystem=strict -p NoNewPrivileges=yes -p PrivateTmp=yes -p CapabilityBoundingSet=` the service user read 384 bytes of `btmp`. A real two-record sample (user and address redacted) matched `utmp(5)`: type 6 at offset 0, the terminal `ssh:notty` at 8, the user at 44, the host at 76, the time as a 32-bit number at 340.

## Decision
We will read `/var/log/btmp` with the `utmp` group (`SupplementaryGroups=utmp` in the unit template, dropped by `setup.sh` on a host with no such group), through one package (`platform/loginlog`) that reads only each record's type and time, newest first and bounded to 4,096 records. A burst (ten failures within ten minutes) raises one fixed-words message and one nightly finding about this computer. No user name, terminal or address is read, so none can be kept, shown or sent. Constitution I.10 (proposed) states this.

## Consequences
- The service gains read access to `btmp`, `wtmp` and `utmp` and nothing else; under `ProtectSystem=strict` `/var/log` is read-only to it, so the group does not allow writing a login record.
- **The group is inherited by every program the service starts**, agent shells and the terminal tool's children included. Under the Landlock filesystem jail their read list (`platform/sandbox/landlock_linux.go`) has no `/var/log`, so they cannot open `btmp`; where the jail is not in force they can, and `wtmp` holds the user name and host of every successful login. Accepted: the operator already lets those programs run commands as this user; the alternative that avoids it (no group) means no watch.
- A forged record needs the privilege the file already requires; the router log, where any device can forge a line, is the weaker source and still raises only a message.
- The record holds password failures only: a guesser that tries keys and fails is not counted. This is stated in the Constitution item and the module's help.
- Until the owner applies the unit change (`setup.sh`, or a drop-in with `SupplementaryGroups=utmp`, then a restart), the module's readiness line says the group is missing and the monitor shows it stopped once it is switched on.
- The burst figures are a starting point with no attack data behind them (vertex's record held two failures, both the owner's own); tune them after a week.

## Alternatives considered
- **`adm` group and `auth.log`** — catches key failures too, but `adm` reads every system log, including `sudo` lines and anything a program printed, and the text needs parsing, which is where a forged user name does harm.
- **`journalctl` as a child program** — an external program under the unit's restrictions, a text parser, and the `systemd-journal` group, which is as broad as `adm`.
- **An ACL on `btmp` for the service user** — narrowest, but `logrotate` recreates the file with its own mode and the ACL is lost at the next rotation.
- **Not reading it, and pointing at the router log** — the router log says nothing about this computer's own logins.

## Links
- Plan: `docs/PLANS/cross-cutting/network-tls-dns-ssh-address-and-discovery.md`. Constitution I.10 (proposed). Ledger: `docs/learnings/network-audit.md` (2026-11-06).
