---
subsystem: network-audit
tags: [privileged-helper, network, measurement]
result: inconclusive
status: awaiting-results
date: 2026-11-07
---

# N0: what the privileged helper needs, measured on vertex (commands and results template)

**Status:** the probes are written and have been run in a Linux container (see "What was checked here"); **nothing has been measured on vertex yet and no
result below is filled in.** Plan: [`network-stack-privileged-additions.md`](../PLANS/cross-cutting/network-stack-privileged-additions.md) §5 N0 and §2a.
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

## Results template (fill in on vertex; paste the masked output under each heading)

**Run:** date ____ host ____ commit ____ results file ____ operator ____

### 0. Facts
| Fact | Value |
|---|---|
| Kernel, systemd, Landlock listed in `lsm` | |
| Unit as systemd sees it (`CapabilityBoundingSet`, `RestrictAddressFamilies`, `SupplementaryGroups`) | |
| nmap version | |
| Interface, wireless yes or no, promiscuous before | |
| `sandboxing` block of `settings.yml`; `effective` from the API | |

### 1. nmap (fill one cell per variant and test with the exit status and one of: `worked`, `refused: <message>`, `unreliable: <message>`, or `killed by signal` (a SIGSYS from the copied `SystemCallFilter` is a third outcome, not a capability refusal, and sizes the helper unit's syscall filter))
| Variant (caps, families) | `--iflist` | `-sU` | `-sU --privileged` | `-sU --privileged --send-ip` | `-O` | `-O --privileged` |
|---|---|---|---|---|---|---|
| 1 none (today) | | | | | | |
| 2 `CAP_NET_RAW` | | | | | | |
| 3 `CAP_NET_RAW` + `AF_NETLINK` | | | | | | |
| 4 `CAP_NET_RAW` + `AF_PACKET` | | | | | | |
| 5 `CAP_NET_RAW` + `AF_NETLINK` + `AF_PACKET` | | | | | | |
| 6 plus `CAP_NET_ADMIN` | | | | | | |

Smallest variant on which each test works: `-sU` ____ ; `-O` ____ . Did the in-force block match the variant for every row? ____

### 2. Capture
**What you did on the network during the window (for example reconnected a phone at minute 1):** ____
| P1 (today) result | P2 (`AF_PACKET`, no capability) result | P3 frames total, mean and max per second |
|---|---|---|
| | | |

| Counts in the window | to this host | broadcast | multicast | other host | sent by this host |
|---|---|---|---|---|---|
| ARP request / reply / gratuitous | | | | | |
| DHCP 68 to 67 (broadcast / not) | | | | | |
| DHCP 67 to 68 (broadcast / not) | | | | | |
| mDNS, DHCPv6, neighbour discovery | | | | | |

Promiscuous before and after: ____ . Caveat to read these by: on Wi-Fi (vertex uses `wlp2s0`) a server's DHCP reply is usually unicast to the client and is **not**
seen, and other hosts' unicast is not seen without promiscuous mode, which this probe never enables; a quiet window is not proof the link is quiet.

### 3. Log files
| Item | Value |
|---|---|
| `auth.log` owner, group, mode; does it exist | |
| What the `adm` group can read (count and names) | |
| `logrotate` create modes for the logs | |
| `access(auth.log)` with the unit's groups / with `adm` | |

### 4. Netlink
| N1 (today's families) | N2 (`AF_NETLINK`, no capability) | events in the window by kind | simulation (`--simulate-neigh-change`) events seen |
|---|---|---|---|
| | | | |

Did a real gateway or neighbour address change happen during the passive window? ____ . Does a change event carry only the new address (simulation, vertex)? ____

### 5. Same-user finding
| Check | Result |
|---|---|
| Real agent terminal: `socket(AF_UNIX)` | |
| Real agent terminal: connect to the stand-in | |
| Listener: peer uid, gid, cgroup of the agent child vs the service | |
| Open descriptors in the agent terminal (any Unix socket?) | |
| `sandboxing.unix_sockets` and `effective.unix_sockets` | |
| `run.sh` section 5, stages A to D | |

**Conclusion for N1 (write after the table is filled):** can uid or cgroup tell an agent child from the service? ____ ; what keeps the child out today? ____
