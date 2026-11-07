---
subsystem: network-audit
tags: [privileged-helper, network, measurement]
result: inconclusive
status: partial-results
date: 2026-11-07
---

# N0: what the privileged helper needs, measured on vertex (commands and results template)

**Status:** measured on vertex 2026-11-07 for sections 0 to 4, the secondary part of 5 and item 5's primary check with the takeover-route facts (results below); DHCP and the namespace simulation are still open. Plan: [`network-stack-privileged-additions.md`](../PLANS/cross-cutting/network-stack-privileged-additions.md) §5 N0 and §2a.
Purpose of the helper (owner, 2026-11-07): a new, core-owned process that runs certain networking commands **outside the service's sandbox**, behind the plan's
gates. These measurements decide which commands it can run and what each needs to step outside.

Files, all in [`2026-11-07-privileged-helper-n0/`](2026-11-07-privileged-helper-n0/): `run.sh` (the runner), `inforce.py` (prepended to every probe),
`capture_frames.py`, `neigh_listen.py`, `unix_probe.py`, and for item 5 by hand `standin_listener.py` and `agent_terminal_probe.py`.

## How the run works, and what it touches

`run.sh` starts each probe as the `llm-proxy` user in a **transient systemd unit** that copies the service's hardening from the installed unit and changes only
four things per variant: the capability set (bounding and ambient), `RestrictAddressFamilies`, `SupplementaryGroups` and, for one opt-in step, the network
namespace. Each of those is set by one `-p`, never merged with the unit's own (systemd merges list properties, so a second `-p` could silently test the wrong
thing). **Every probe first prints what is in force** (`CapEff`, `CapBnd`, `CapAmb`, `NoNewPrivs`, one `socket()` per family), so the pasted output proves which
variant ran.

| Touches | When |
|---|---|
| Read-only: facts, log permissions, a joined netlink group, a header-only capture (128 bytes per frame, no promiscuous mode, nothing sent, counts printed, no addresses) | always |
| Sends packets: UDP to one port and a few TCP probes, to **one private IPv4 device you name with `--target`** and own; rate and time limited (`--max-retries 1 --host-timeout 60s --max-rate 20`); refuses any other address | section 1, only with `--target` |
| Creates and deletes a throwaway network namespace `n0probe` (no host interface or route) | only with `--simulate-neigh-change` |
| Creates and removes `/run/n0-helper-standin` | only item 5's manual step, `standin_listener.py` |

Addresses (IPv4, IPv6, MAC) are masked before they reach the screen or the results file; no log line is read; the `adm` listing shows file names, modes and
owners only. Read the file before you paste it.

Copy the folder to vertex (for example `scp -r docs/audits/2026-11-07-privileged-helper-n0 vertex:`); vertex has the unit installed, so the repo-template fallback is not used there.

```bash
cd 2026-11-07-privileged-helper-n0   # on vertex, wherever you copied it
./run.sh --print --target 192.168.x.y            # show every command, run nothing (no root)
sudo ./run.sh --target 192.168.x.y --window 120  # everything but the simulation; add --iface wlp2s0 to name the interface
sudo ./run.sh --sections 4 --simulate-neigh-change   # only the netlink section, with the namespace simulation
```

Sections: 0 facts, 1 nmap, 2 `AF_PACKET`, 3 log files, 4 netlink, 5 Unix-socket secondary check. `--window` is the capture and listen time in seconds (5 to 600).

## What each section measures, and what N1 reads from it

| § | Question (plan §5 N0) | Variants run | N1 uses it to decide |
|---|---|---|---|
| 1 | What do `nmap -sU` and `-O` need? | six unit variants: no capability (today); `CAP_NET_RAW`; `CAP_NET_RAW` plus `AF_NETLINK`; plus `AF_PACKET`; both; `CAP_NET_RAW`+`CAP_NET_ADMIN` with both. Each runs `--iflist` first (the ledger shows an interface lookup needs netlink under the unit), then `-sU` default, `--privileged`, `--privileged --send-ip` (decides whether `AF_PACKET` is needed to send), and `-O` with and without `--privileged` | the privilege class and address families of the scan unit; whether `--privileged` is part of the action's fixed arguments |
| 2 | Does `AF_PACKET` see ARP and DHCP, and how often? | P1 today's unit (expect a refusal), P2 `AF_PACKET` without a capability, P3 `AF_PACKET` with `CAP_NET_RAW` for the window | whether a capture action is worth building, its frame-rate cap, its window length |
| 3 | Who can read `auth.log`, and what else comes with it? | modes and owners; what the `adm` group can read; `logrotate` create modes; `access()` as the service user with its own groups and with `adm` | whether the `adm` class is acceptable in its own unit |
| 4 | Does an unprivileged netlink neighbour subscription work, and what does a change show? | N1 today's unit (expect a refusal), N2 with `AF_NETLINK` and no capability, a passive window of normal change noise; optional simulation of a gateway change in a throwaway namespace | whether item A's ARP and gateway part needs the helper at all |
| 5 | Can an agent child reach a Unix socket the helper would listen on? | primary: the real agent terminal against a stand-in socket (below); secondary: `run.sh` section 5 | the `Effective()` precondition, and whether peer identity can be used at all |

## Item 5, the primary check (the one non-read-only step)

Section 5 of `run.sh` re-creates the jail's pieces in a transient unit. It tests the kernel, not this service's jail, so the **real** check is a real child of
the running service:

1. On vertex, as root, in one terminal: `sudo python3 standin_listener.py --seconds 900`. It creates `/run/n0-helper-standin/sock` (directory and socket
   `root:llm-proxy`, mode 0750 and 0660, the shape socket activation gives the helper), never reads what a client sends, records who connects, and removes
   the directory when it exits.
2. In the assistant's own terminal (an agent child of the service; **its allow-list may refuse a `python3 -` heredoc or ask you to approve it: record which happened**, or a refusal there looks like a pass), run the contents of `agent_terminal_probe.py` through
   `python3 - <<'PROBE' ... PROBE` (the terminal's file jail may not let it read this repository). It prints its uid, `NoNewPrivs`, `Seccomp`,
   capabilities, cgroup, every open descriptor (an inherited Unix socket would show here and would bypass a filter that only blocks `socket()`), whether the
   socket is visible, and what `socket(AF_UNIX)` and `connect` returned.
3. The listener prints, for each connection, the peer's uid, gid, pid, cgroup, `NoNewPrivs` and `Seccomp`. That shows whether uid or cgroup can tell an agent
   child from the service (`KillMode=control-group` suggests neither can; this is the measurement, not an assumption).
4. Repeat step 2 once from the service's own side if you can (any process of the service itself, for the baseline), and record the settings in force: the
   `sandboxing` block that `run.sh` section 0 prints from `settings.yml`, and the `sandboxing` and `effective` objects of `GET /admin/api/host`
   (`models/host_settings.go`: `unix_sockets` is part of `effective`). The call needs your admin token; the port and header scheme are the ones you already use.

## What was checked here (the tooling, not vertex)

Run 2026-11-07 on a Mac in the colima Linux container (kernel 6.8.0, aarch64, Landlock ABI 4, `--cap-add NET_RAW`, seccomp unconfined), as root. **This proves the
probes run and decode correctly; it says nothing about what vertex will report.**
- `capture_frames.py --selftest`: 7 synthetic frames (ARP request, reply and gratuitous; DHCP 68 to 67 broadcast and 67 to 68 not broadcast; mDNS; too short; other
  ethertype) classify correctly. A live 6-second capture counted frames by direction and label with no payload read.
- `neigh_listen.py`: the parser decodes real kernel events. In a dummy interface, an added neighbour and its address replacement each arrived as a single `new` event carrying the
  **new** hardware address and no earlier one; the delete arrived as `new` then `del` in state `failed` with no address. (Container, dummy device: the shape to check against vertex.)
- `unix_probe.py`: Landlock alone did not stop a connect to a socket outside its grant (matches ADR-0031 and pitfall 67); the filter alone refused `socket(AF_UNIX)`
  with `EACCES` and allowed `socketpair`; a connected descriptor made before the filter still sent after it.
- **`run.sh` end to end under real systemd 255** (the colima VM, Ubuntu 24.04, kernel 6.8, a temporary `llm-proxy` user, the repo template as the hardening source, an
  nmap 7.94 target in a container on the Docker bridge; the user and the container were removed afterwards). It proves the runner, not vertex: the in-force block matched
  every variant (`CapAmb` followed the requested set for a non-root user under `NoNewPrivileges`; a blocked family showed `EAFNOSUPPORT` and a missing capability
  `EPERM`, so the two refusals are distinguishable); `NetworkNamespacePath` works with `RestrictNamespaces=true` for the simulation; a missing log file did not stop the run; masking left the output readable.
  Measured there: two `-p RestrictAddressFamilies=` arguments **merged** (both `AF_INET` and `AF_NETLINK` were allowed), which is why each measured property is set by exactly one `-p`.
  nmap there needed `--privileged`, `CAP_NET_RAW`, `AF_NETLINK` and `AF_PACKET` together for `-sU` and `-O`, and without `--privileged` it quit with "requires root privileges"
  even holding the capability; vertex's LAN may differ, so it is not a result. `shellcheck` was not available.

## Results on vertex (2026-11-07, run by the owner; masked output, `run.sh` at commit b4b35069)

**Run:** host vertex, kernel 7.0.0-38-generic x86_64, systemd 255.4, Landlock ABI 8 (Landlock, Yama and AppArmor listed in `lsm`). **Item 5's primary check was run later the same day** (see the last table). **Still open:** DHCP (none seen, and what the operator did during the window was not recorded), the namespace simulation of a
gateway change (not run on vertex), nothing else from item 5 (the listener printed nothing and the terminal asked for no approval, both confirmed by the owner).

### 0. Facts
| Fact | Value |
|---|---|
| Unit as systemd sees it | `User=llm-proxy`, empty `CapabilityBoundingSet` and `AmbientCapabilities`, `SupplementaryGroups=utmp`, `NoNewPrivileges=yes`, `RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX`; the service user's groups in force: 982 (its own), 43 (`utmp`), 44, 992 |
| nmap | 7.94SVN |
| Interface | `wlp2s0`, wireless (a Wi-Fi link, not Ethernet); `ping_group_range` 0 to 2147483647 |
| `sandboxing` block | `enabled: true`, `unix_sockets: false`, `functional: false`, `maxstoragegb: 2`, `maxmemorymb: 256`, `egress_proxy: 0`. The API's `effective` object was not captured |
| `yama_ptrace_scope` | `1`, captured later the same day (last table) |

### 1. nmap (every refusal exited with status 1; cells are the message nmap gave)
| Variant (capabilities, families added to the unit's three) | `--iflist` | `-sU` | `-sU --privileged` | `-sU --privileged --send-ip` | `-O` | `-O --privileged` |
|---|---|---|---|---|---|---|
| 1 none (today) | worked, no routes | refused: requires root | refused: no `AF_NETLINK` socket (`route_dst_netlink`) | same | refused: requires root | refused: `route_dst_netlink` |
| 2 `CAP_NET_RAW` | worked, no routes | refused: requires root | refused: `route_dst_netlink` | same | refused: requires root | refused: `route_dst_netlink` |
| 3 `CAP_NET_RAW` + `AF_NETLINK` | worked, routes listed | refused: requires root | refused: `dnet: Failed to open device wlp2s0` | refused: `pcap_activate` failed, no `AF_PACKET` | refused: requires root | refused: `dnet: Failed to open device` |
| 4 `CAP_NET_RAW` + `AF_PACKET` | worked, no routes | refused: requires root | refused: `route_dst_netlink` | same | refused: requires root | refused: `route_dst_netlink` |
| **5 `CAP_NET_RAW` + `AF_NETLINK` + `AF_PACKET`** | worked | refused: requires root | **worked** (`53/udp closed`) | **worked** | refused: requires root | **worked** (`Linux 4.15 - 5.8`, SSH open on the target) |
| 6 variant 5 + `CAP_NET_ADMIN` | worked | refused: requires root | worked | worked | refused: requires root | worked |

Smallest variant on which `-sU` and `-O` work: **5**. `CAP_NET_ADMIN` adds nothing (6 matches 5). Without `--privileged` nmap refuses even while holding `CAP_NET_RAW`, so `--privileged` belongs in
the action's fixed arguments. `--send-ip` did not remove the need for `AF_PACKET` (capture still needs it). The in-force block matched the variant on every row (`Seccomp=2`, `NoNewPrivs=1`, `CapAmb` equal to `CapEff`).
No process was killed by a signal, so the unit's `SystemCallFilter` blocked nothing nmap needed.

### 2. Capture (`wlp2s0`, 60 s window, headers only)
**What the operator did during the window:** not recorded. P1 (today's unit): `open/bind AF_PACKET` failed `EAFNOSUPPORT` (family not allowed). P2 (`AF_PACKET`, no capability): `EPERM`. P3 (`CAP_NET_RAW` + `AF_PACKET`): worked; promiscuous before and after: no.

| Window figure | Value |
|---|---|
| Frames | 854 in 60 s: mean 14.2 per second, **maximum 139 in one second** |
| By direction | to this host 211, broadcast 134, multicast 413, sent by this host 96, other host 0 |
| ARP | 10 broadcast requests, **all gratuitous**, from 5 distinct sender hardware addresses; 1 request to this host and 1 reply from it |
| DHCP (68 to 67, 67 to 68) | **0 of either** |
| Other | mDNS 304 (multicast), IPv4 UDP broadcast 124 and multicast 180, IPv6 multicast 152, other ethertypes 66 |

On Wi-Fi without promiscuous mode a server's DHCP reply is usually unicast to the client and not seen; the window may also simply have held no join. **Not concluded.**

### 3. Log files
| Item | Value |
|---|---|
| `auth.log` | exists; owner `syslog:adm`, mode 640 (also `auth.log.1`, `syslog`); `/var/log` is `root:syslog` 755; `btmp` is `root:utmp` 660 |
| What `adm` reads | gid 4, 2 members, **67 files** at depth 2: `auth.log`, `syslog`, `kern.log`, `fail2ban.log`, `apport.log`, `cloud-init.log`, `dmesg`, apt's `term.log` and the unattended-upgrades logs with their rotations |
| `logrotate` create modes | `/etc/logrotate.d/btmp`: `create 0660 root utmp`; `/etc/logrotate.conf`: `su root adm` |
| `access()` as the service user | with the unit's groups: `auth.log` no, `syslog` no, `btmp` yes; with `adm` instead: `auth.log` yes, `syslog` yes, `btmp` no |

### 4. Netlink
Today's families: the subscription failed (`EAFNOSUPPORT`). With only `AF_NETLINK` added and **no capability** it worked: 2 events in 60 s (one `new` neighbour `reachable`, one `stale`, both with a destination and a hardware address). The simulation was not run on vertex; in the container a replacement arrived as one event with only the new address (tooling section above).

### 5. Same-user finding (secondary check only; the primary check is open)
Transient unit with the service's hardening on vertex, Landlock ABI 8: A, no confinement: connect ok. B, Landlock alone: the read outside the grant was refused (`EACCES`) and **the connect to a socket outside the grant still worked** (pitfall 67 holds on ABI 8). C, the seccomp filter alone: `socket(AF_UNIX)` refused (`EACCES`), `socketpair` ok. D, a connected descriptor made before the filter: **still sent after it**. The stand-in saw peer uid 995, gid 982 on every connection, the service's own, so uid cannot tell a child of the service from the service.

| Primary check (real agent terminal) | Result (owner, vertex, 2026-11-07; the output was relayed by the assistant, not a raw terminal capture) |
|---|---|
| `socket(AF_UNIX)`, connect | **`socket(AF_UNIX)` refused, `EACCES`; `connect` was never reached.** The refusal is the seccomp filter by elimination (capabilities are all zero and a Unix socket needs none; `Seccomp: 2`); the errno's origin was not proven |
| The child's identity | uid 995, gid 982, groups 43 44 982 992; `CapEff` and `CapBnd` all zero; `NoNewPrivs: 1`; `Seccomp: 2`; cgroup `0::/system.slice/llm-proxy.service`. Same uid, gid and cgroup as the service, so none of them tells child from service |
| Open descriptors | fd 0, 1, 2 only, all pipes: no inherited Unix socket |
| Stand-in socket visibility | `stat` ok, mode `0o140660`: the file jail does not hide it and the mode would let the group in; the filter is what stopped the connect |
| Listener's peer record (uid, gid, pid, cgroup) | **nothing printed** (owner confirmed): no connection reached the stand-in, consistent with the refused `socket()` |
| Whether the terminal's allow-list refused the heredoc or asked for approval | **no approval was asked** (owner); the probe ran as pasted |
| `yama_ptrace_scope` | **`1`**: a process may only inspect its own descendants, so a child cannot inspect the service |
| `MainPID` of `llm-proxy.service` | `555566` at the time of the run |
| `ptrace`, `pidfd_getfd`, `process_vm_readv`, `process_vm_writev` in `@system-service` | **none of the four listed** (`systemd-analyze syscall-filter @system-service`: this is the group's definition on systemd 255.4, not the live process's filter) |

**What this shows.** A real agent child of the service cannot create a Unix socket, so it cannot connect to a root helper's socket; the filter is the only barrier and is now measured on the real path, not a stand-in unit. The takeover routes (ptrace, `/proc/<MainPID>/mem`, `pidfd_getfd`, `process_vm_*`) are closed by Yama scope 1 together with a filter that does not list the calls; that the Yama rule also covers `/proc/<pid>/mem` and `pidfd_getfd` is the kernel's documented behaviour, not tested here. **What it does not show:** only the terminal tool's default path was tested, not the scanners, the program runner or any approval or override path (each must be checked before it can be said that every child is filtered); `connect` from a child that has a Unix socket by another route is untested (none is inherited); and the helper's own `MainPID` comparison is untested until N2a. `adm` (gid 4 on Ubuntu) is not among the child's groups 43 44 982 992 (gid 4 being `adm` on vertex was not checked).
