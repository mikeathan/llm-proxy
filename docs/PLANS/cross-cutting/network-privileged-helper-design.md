---
status: proposed
date: 2026-11-07
last_reviewed: 2026-11-07
related_specs: [SPEC-006]
constitution_references: [I.5, I.7, I.10, II.3, IV.6]
evidence: docs/audits/2026-11-07-privileged-helper-n0.md (measured on vertex 2026-11-07); ADR-0033; ADR-0034 (proposed); docs/services/llm-proxy.service; backend/internal/transport/http/authenticate.go (step-up); backend/internal/platform/{isolation,progexec,scanscope,sandbox}. Nothing built.
---

# The privileged helper: design (N1)

**Status:** proposed; design only, **no code, unit, setting or Constitution text is changed by this document.** It is N1 of
[`network-stack-privileged-additions.md`](network-stack-privileged-additions.md) (§2a is the decision, §5 the slices). **Purpose (owner, 2026-11-07):** run certain
networking commands **outside the service's sandbox** (outside the limits of its systemd unit) through a small process owned by the core, so the service, the agent's
terminal and every automation keep today's limits. Programs the helper starts stay file-jailed through `isolation`. Decision record: [ADR-0034](../../decisions/ADR-0034-the-privileged-helper-is-core-not-a-stack-and-decides-who-is-calling.md).

## 1. What N0 settled, and what it changed in the design

| Measured on vertex (audit) | Consequence |
|---|---|
| `nmap -sU` and `-O` work with `CAP_NET_RAW`, `AF_PACKET`, `AF_NETLINK` and `--privileged`; `CAP_NET_ADMIN` adds nothing | the scan class needs exactly that; `--privileged` is part of the fixed argument list |
| `AF_PACKET` capture on Wi-Fi saw ARP (10 gratuitous broadcasts from 5 senders in 60 s), no DHCP, mean 14 and **peak 139 frames per second** | the capture action needs a fixed header-only filter and a hard frame cap |
| An unprivileged netlink neighbour subscription works with `AF_NETLINK` and **no capability** | the ARP and gateway-change part of item A needs no helper (§8, owner decision) |
| `adm` reads 67 files, `auth.log` among them | the log class is its own unit and is the widest grant (§5) |
| Landlock ABI 8 still lets a child connect to a socket outside its grant; the filter alone refuses `socket(AF_UNIX)`; a descriptor connected before the filter still works; peer uid and gid are the service's | **the filter is the only barrier today, and uid cannot tell a child from the service** (§2, the reason for the design below) |

**The precondition does not protect the helper.** §2a item 7 (the client refuses unless `Effective()` reports file confinement and Unix-socket denial) runs *in the
service*. A child with the filter off never uses the client: it opens the socket itself. So the precondition is a safety interlock for the service's own calls, and
**the helper must decide who is calling.**

## 2. Who may call: the helper checks the caller, not a group

The socket is `root:llm-proxy`, mode 0660, so only the service's account can open it. That is necessary and not enough: a child runs as the same account. The helper
therefore accepts a request only when the **peer is the service's main process**:

1. On accept, take the peer's process handle from the kernel (`SO_PEERPIDFD`, Linux 6.5 and later, so present on vertex's 7.0 kernel; `SO_PEERCRED` pid as the fallback, with the pid-reuse race noted).
2. Read the unit's `MainPID` from systemd (a fixed, read-only query of `llm-proxy.service`; see decision D3 for how) and require peer pid equals `MainPID`.
3. Refuse anything else with no detail, and log the peer's pid and uid.

Not used: group or uid (measured identical), parent pid (a double-forked grandchild is re-parented to pid 1), cgroup (every child stays in the service's cgroup), and a
secret in a file or the environment (readable by the same account; Landlock's read list includes `/proc`).

**The rule depends on a child being unable to become or read the main process.** Same-account routes to check: `ptrace` and `/proc/<MainPID>/mem` (Yama's `ptrace_scope`,
and whether the unit's `SystemCallFilter` already refuses `ptrace`, `process_vm_readv`, `process_vm_writev` and `pidfd_getfd`, which would otherwise let a child steal the
service's connected descriptor, the same class as N0's stage D); a descriptor the service connected and a child inherited (Go opens descriptors close-on-exec, and a search of
non-test code under `internal/` finds no `ExtraFiles`, so the runner passes children no extra descriptors; the real terminal's descriptor list is the check). Options to evaluate, not
claims: the helper refuses when `ptrace_scope` is 0; the service marks itself non-dumpable. **Measured on vertex 2026-11-07:** `ptrace_scope` is 1 (a process may only inspect its descendants, and a child is not the service's ancestor), the service's `@system-service` filter lists none of `ptrace`, `pidfd_getfd`, `process_vm_readv`, `process_vm_writev`, and the real agent terminal had no inherited Unix socket (descriptors 0 to 2, pipes) and could not create one (`socket(AF_UNIX)` refused, `EACCES`): [audit](../../audits/2026-11-07-privileged-helper-n0.md). So D8's refusal is a cheap tripwire against a later change of that setting, not a fix for an open hole. **If the service process itself is compromised, the helper
cannot tell** (that process can mint grants): what remains is the catalogue, the helper's own scope and cap checks, the off-by-default switches and the unit gate. The helper is
built to survive a hostile *child*, not a hostile service.

**Where a child is not filtered (read from the code 2026-11-07, not measured on vertex).** Every non-test spawn site of a child in `backend/` is one of: the one-shot terminal (`internal/core/tools/terminal.go` `newCommand`, then `Wrap`), the persistent shell (`internal/shell/terminal.go:42`, then `Wrap`), `progexec` run (`internal/platform/progexec/progexec.go:164`, then `Wrap`), `progexec` version query (`progexec.go:87`, **no `Wrap`**, fixed arguments, empty environment), the GPU provider (`internal/platform/metrics/gpu_providers.go:170`, fixed commands, no `Wrap`) and `ps` on macOS. Two ways a `Wrap` call still leaves a child without the Unix-socket denial: (1) the Landlock provider returns without wrapping when the workspace root is empty (`landlock_provider_linux.go:163`, "non-workspace one-shot"), and the terminal passes an empty root when the run has no workspace id (`terminal.go:128-132`); whether a real run can reach that path was not checked; (2) the `noop` provider (filesystem confinement off, or Landlock unavailable) wraps nothing (`sandbox.go:145`) and reports it in `Effective()`. The vertex probe ran in a workspace with confinement on, so it covers neither. **Built (N2a, `internal/archtest/boundary_test.go`, `TestEveryPlaceTheServiceStartsAProgramIsPinned`):** the spawn sites are pinned exactly, each either names the function that calls `Wrap` (and the test fails if it stops) or carries a reason; a new site fails the build. **Whether a run with no workspace id reaches the terminal is not established:** the chat routes carry a workspace path parameter and `toolregistry` returns an empty root only for an empty id or no persistence, but no production caller without one was found or ruled out. The test cannot settle it (it checks `Wrap` is called, not what `Wrap` does with an empty root), so D9's fix stays its own change set. N2a's test must prove each is filtered or cannot be driven by the agent; closing (1) (always wrap, or refuse a terminal call with no workspace) changes the sandbox contract and is the owner's decision (D9).

## 3. Gate for N2

N2 does not start until: (a) item 5's primary check is in the audit and shows that a real agent child is refused by the filter and cannot be mistaken for `MainPID`
(listener prints `peer_is_main=False` for the child, descriptor list shows no Unix socket) **and** the takeover routes of §2 are measured on vertex (`yama_ptrace_scope`, and the
syscall-filter lines `run.sh` section 0 now prints). **Status 2026-11-07: met for the terminal tool's default path** (the child could not create a Unix socket, so there was no peer to compare; `ptrace_scope` 1; none of the four calls listed). **Not yet shown:** that every other child the service starts (scanners, the program runner, approval or override paths) runs under the same filter, which N2a's fitness test must prove, and the helper's own `MainPID` comparison; (b) the owner accepts ADR-0034; (c) the Constitution text in §9 is
merged for the first action; (d) the MainPID mechanism (D3) is chosen and the first slice proves it on vertex.

## 4. Action catalogue and schema

The catalogue is Go data in core code (proposed package `internal/platform/privhelper/catalog`), compiled into both the service and the helper. **An action is a row; a
stack cannot add one.**

| Field | Meaning |
|---|---|
| `ID` | stable name, for example `scan_udp` |
| `Class` | which unit runs it: `net` or `log` |
| `Params` | a typed struct; the wire carries it as JSON and the helper decodes it strictly (unknown fields refused) |
| `Build` | builds the whole argument list from the typed params; **never forwards an argv it was given** |
| `Scope` | the check the helper makes **from its own data, never the caller's**: fixed invariants in code (one private IPv4 host as `scanscope` defines it, II.3 item 3; ports from the fixed list; an interface that exists; log paths from a fixed list) and the operator's ranges from a root-owned copy in the unit's drop-in (`HELPER_SCOPE`, written by the install step), so a compromised service cannot widen what the helper will scan |
| `Caps` | timeout, maximum output bytes, a rate and a per-hour count |
| `Jail` | the `isolation.Profile` (root, read paths, whether the network is kept) |
| `Result` | `bytes` (raw, bounded) or `records` (fixed-size); never parsed text |
| `Switch` | the settings key that must be on |

First rows (the arguments are the measured ones; each action also needs its own Constitution item and an N2 measurement under the helper's unit):

| ID | Class | Params | Fixed behaviour | Result |
|---|---|---|---|---|
| `scan_udp` | net | `target` (one private IPv4), `ports` (a subset of a fixed list: 53, 67, 69, 123, 137, 161, 500, 1900, 5353) | `nmap --privileged -n -Pn --max-retries 1 --host-timeout 60s --max-rate 20 -sU -p <ports> <target>` | bytes (nmap's output, bounded) |
| `os_detect` | net | `target` | `nmap --privileged -n -Pn ... -O -p 22,80,443 <target>` | bytes |
| `capture_window` | net | `iface` (must exist), `seconds` (at most the cap) | the helper opens `AF_PACKET`, attaches a **fixed** classic BPF filter (ARP, UDP 67 and 68, 5353, neighbour discovery), copies at most 64 bytes of each frame, stops at the window, the frame cap or a disconnect; no promiscuous mode, nothing is sent | records: timestamp plus 64 raw bytes |
| `read_auth_log` | log | `since` (a time), `max_bytes` | opens only `/var/log/auth.log` and `auth.log.1` by fixed name and returns the newest bytes | bytes |

The privileged code never parses what it returns: ARP, DHCP, nmap and log text are parsed in the service (§7).

## 5. Units, user and install step

Drafts, proven in N2 with the `scripts/check-sandbox-under-unit.sh` approach. Socket activation with `Accept=yes`: **one short-lived process per connection**, no standing privilege.

```ini
# llm-proxy-helper-net.socket
[Socket]
ListenStream=/run/llm-proxy-helper-net.sock
SocketUser=root
SocketGroup=llm-proxy
SocketMode=0660
Accept=yes
MaxConnections=2

# llm-proxy-helper-net@.service
[Service]
ExecStart=/usr/local/bin/llm-proxy-helper --class net --actions ${HELPER_ACTIONS}
User=llm-proxy-helper   # a fixed account: DynamicUser cannot ask systemd for the main process (pitfall 69)
CapabilityBoundingSet=CAP_NET_RAW
AmbientCapabilities=CAP_NET_RAW
RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6 AF_NETLINK AF_PACKET
NoNewPrivileges=yes
SystemCallFilter=@system-service landlock_create_ruleset landlock_add_rule landlock_restrict_self
MemoryDenyWriteExecute=yes
RestrictNamespaces=yes
ProtectSystem=strict
ProtectHome=yes
PrivateTmp=yes
ProtectKernelTunables=yes
ProtectKernelModules=yes
ProtectKernelLogs=yes
IPAddressDeny=any
IPAddressAllow=10.0.0.0/8 172.16.0.0/12 192.168.0.0/16 fe80::/10 ff00::/8 224.0.0.0/4
RuntimeMaxSec=700
```
The `log` class is the same shape with `SupplementaryGroups=adm`, `PrivateNetwork=yes`, `RestrictAddressFamilies=AF_UNIX`, no capabilities. **Say plainly to the operator: the `adm`
group reads 67 files on vertex, `syslog`, `kern.log` and `fail2ban.log` among them; the action opens only the two `auth.log` names, but the unit holds the whole group.**
`IPAddressDeny` limits IP sockets, not `AF_PACKET`; whether it holds for nmap's raw sends is an N2 measurement. Unmeasured and listed as such: whether ambient `CAP_NET_RAW`
survives the sandbox runner's re-exec plus Landlock inside this unit (N2 acceptance test).

**The helper is a separate small binary** (proposed `backend/cmd/llm-proxy-helper`) so privileged code is not the whole service: an import allowlist pins it to `isolation`,
`progexec`, `scanscope`, the catalogue and the wire package. Its `main` calls `isolation.RunChildIfRequested()` first, or the jail cannot run in it.

**Two gates, both off by default.** (1) The operator installs the unit as root: `scripts/install-helper.sh` (`sudo scripts/install-helper.sh net --install ...`, built; the interactive `setup.sh` is not touched) renders the units from
`docs/services/` the way `render_unit` renders the main one, installs the binary, writes the drop-in that sets `HELPER_ACTIONS` and `HELPER_SCOPE` (the operator's scan ranges as of install), runs the under-unit check, then
`systemctl enable --now` the socket. The service can never grant itself this. (2) The per-action switch in settings, changed only by a step-up route. The drop-in's action list
is a third, root-owned check a compromised service cannot flip. Without a unit the actions report unavailable (to test, start the helper by hand as root on a temporary socket); macOS: unavailable.

## 6. Wire protocol and the grant

**Transport:** a Unix stream socket, one request per connection, length-prefixed JSON (4-byte big-endian length; the request is at most 4 KiB, the response at most the action's `Caps`). All
fields are plain data (IV.6). Unknown fields, a wrong version, a length over the cap or a second request are refused.

```json
{"v":1,"id":"<random>","action":"scan_udp","params":{"target":"192.168.50.10","ports":[53,161]}}
{"v":1,"id":"<same>","status":"ok","result":{"kind":"bytes","data":"<base64>","truncated":false}}
{"v":1,"id":"<same>","status":"refused","code":"scope","message":"target is outside the scan scope"}
```
Statuses are `ok`, `refused` (a rule said no: `scope`, `caps`, `disabled`, `caller`, `busy`) and `error` (the run failed). A refusal and an error carry a code and a message, never a Go error (IV.6).
The helper cancels the run when the peer disconnects.

**The grant** is an in-memory, one-use capability bound to the action and the canonical parameters. The minter must know both, and step-up runs in middleware before a stack
parses a body, so: **a route that can trigger an action is a core-checked privileged route.** Its manifest declares the catalogue ID it may request (a new `RouteSpec.Action`), it is
`step_up` (a token principal already gets 403; a session must have confirmed the passphrase shortly before), and **its request body is exactly that action's typed `Params`**, which the core
decodes strictly against the catalogue before the stack sees it. The core then mints a random 128-bit grant bound to the action and a hash of those params, with a short expiry, and
hands it to the stack in a new `Request.Grant` field (plain data). The stack passes grant, action and params back through the SDK port; the core checks they match, consumes the grant
and only then calls the helper client. These two SDK fields are N2a's work. The helper does **not** verify grants: it trusts only the caller identity of §2 and re-checks scope, caps and the unit's action list itself. So the grant answers "was this a
person's step-up click" inside the service, and §2 answers "was this the service" at the helper. The fitness test pins that nothing else can mint one.

## 7. How the data reaches the agent

The helper returns bytes or records. The **service** (the Network stack, unprivileged) parses and validates them into typed fields and writes them to the file the stack owns
(P3b D10); the agent reads them through the existing findings and read paths and sees typed fields, never packets or log lines (Appendix A of the architecture doc, ADR-0033 item 5).
Constitution I.10 says failed-login names are never read or kept; `read_auth_log` therefore needs its own Constitution item deciding what may be extracted.

## 8. Netlink is an owner decision, not part of the helper

Neighbour-table changes need only `AF_NETLINK`, with no capability. **A:** add `AF_NETLINK` to the main unit. Children could then read interfaces, routes and neighbours and
change nothing; it probably also fixes Nearby's `net.Interfaces()` failure in the ledger (2026-11-03). **B:** a no-capability helper class with a standing listener, which
conflicts with "on demand". Recommendation: A, as its own ADR, after the owner has weighed what it shows an agent child.

## 9. Constitution text (owner approved D7, 2026-11-07; written in `CONSTITUTION.md` on this branch, in force when the owner merges it)

- **New item I.11, "Privileged Helper":** the closed catalogue, the operator-installed units and three gates, the step-up-only grant, the caller check, helper-side scope, jailed programs and untrusted data parsed outside, the time-boxed header-only capture, the interlock, and the accepted residual risks. It states in its capture bullet that I.7 and I.9's "never opens a raw socket" stays true of the service and its children, so I.7 and I.9 are not rewritten (this replaces the earlier draft to amend I.7).
- **IV.6, last bullet:** a request to the helper is a request the core mediates through an SDK port, not a declaration; a manifest still cannot add an action.

## 10. Fitness tests (modelled on `internal/archtest/boundary_test.go`)

1. Only the step-up handler and the composition root import the helper client and the grant mint; `toolregistry`, `assistant`, automation and every stack do not.
2. The helper binary imports only its allowlist.
3. Every catalogue row has `Scope`, `Caps` and `Jail`, and a `Build` that takes only its typed params (a table test over `catalog.All()`).
4. A wire contract test (JSON round trip, unknown fields and oversize frames refused) that runs against the real helper and the fake.
5. A grant test: one use, bound to action and target, expires, cannot be built outside its package.
6. The helper's jail passes the `isolationtest` conformance suite.

## 11. Slices (each its own change set)

| Slice | Content | Needs |
|---|---|---|
| N2a | wire package, client, fake helper, grant, fitness tests 1, 3, 4, 5, and the spawn-site test of §2; no privilege, runs anywhere | this change set merged (ADR-0034 and I.11), §3 gate (a) (met) |
| N2b | helper binary and `net` unit, `scan_udp`; `setup.sh helper`; the under-unit check on vertex | Constitution item I.11 merged, D3 (decided) |
| N2c | `os_detect`; N2d `capture_window` with its frame cap | measurement of burst behaviour |
| N2e | `read_auth_log` and the `log` unit | **deferred (D1)**: a new owner decision on the `adm` width |

**Cost.** Nothing on an existing path goes through the helper, so the P3b baselines ([`network-stack-p3b-design.md`](network-stack-p3b-design.md) 4b) do not move; test 1 keeps hot paths from importing it. A call is a local socket round trip and a process start (unmeasured).

## 12. Decisions (the owner approved all of D1 to D9 with the recommendations below, 2026-11-07)

| # | Decision |
|---|---|
| D1 | Units for B (`scan_udp`, `os_detect`) first; A (`capture_window`) after, as its own slice; C (`read_auth_log`, the `adm` width) not yet, so it stays out of the first catalogue and N2e waits for a new decision; D (Zeek and Suricata logs) not through the helper, a plain read group is enough. |
| D2 | The caller rule of §2 (peer is `MainPID`) is the control, with the residual risk stated in I.11. |
| D3 | The helper learns `MainPID` from a fixed read-only `systemctl show -p MainPID --value llm-proxy.service`, run through `isolation` with `AllowUnixSockets` and `NoNetwork` (the precedent is `nmcli`, ADR-0031); an N2a test proves the child holds no capability (`CapEff` empty) although the unit gives the helper `CAP_NET_RAW`. No D-Bus dependency. |
| D4 | Netlink is not part of this work. If wanted, `AF_NETLINK` in the main unit is its own ADR after the owner weighs what a child could read (§8). |
| D5 | A capture window of about 60 s (the measured window) with a frame cap taken from the measured peak of 139 frames per second; the numbers are fixed in N2d from a burst measurement. 64 bytes per frame, nothing sent. |
| D6 | "Outside the sandbox" means outside the service unit's limits (capabilities, socket families); programs the helper starts stay file-jailed. |
| D7 | The Constitution items of §9, written in this branch. |
| D8 | The helper (and the install step) refuse when `ptrace_scope` is 0, and the service marks itself non-dumpable. Both are N2 code. |
| D9 | Always apply the filter: a terminal or program spawn with no workspace gets a deny-all profile instead of none (closes `landlock_provider_linux.go:163`). Open before N2a: whether a chat without a workspace is offered the terminal tool (`internal/core/commands/run.go:43` shows such chats exist; not traced further), which N2a's spawn-site test answers. The change is to the sandbox contract (SPEC-006), so it is its own change set with a SPEC update. |


## Remaining Work

Design written 2026-11-07 (N1); the owner approved D1 to D9 the same day; ADR-0034 and Constitution I.11 are in this branch and take effect when the owner merges it.

**N2a built (2026-11-07), no privilege, runs anywhere:** `internal/platform/privhelper/{wire,catalog,grant,client,port,helpertest}`, the SDK fields `RouteSpec.Action`, `Request.Grant` and the `Privileged` port, the route handler's decode-and-mint, and the architecture tests (spawn sites pinned, import rules).

**N2b built (2026-11-07):** the helper (`internal/platform/privhelper/helper`, binary `cmd/llm-proxy-helper`), the units (`docs/services/llm-proxy-helper-net.{socket,@service}`), `scripts/install-helper.sh` (instead of a `setup.sh` mode: the interactive `setup.sh` is not touched), the acceptance script `scripts/check-helper-under-unit.sh`, and the service side: the grant store, client and port are built once in `internal/app/privhelper.go`, with two step-up routes, `POST /admin/api/helper/scan_udp` and `/os_detect`, one per catalogue action, held to the step-up table by a test. The second gate is `sandboxing.helper_actions` (a list of action IDs in settings.yml, written only by the step-up `PUT /admin/api/host`, and validated against the catalogue); the third is the root-owned `HELPER_ACTIONS` drop-in. Fitness tests 2 (the helper's import allowlist) and 4 (the wire contract) exist.

**Measured, in the colima VM (Ubuntu 24.04, kernel 6.8, systemd 255, nmap 7.94; NOT vertex):** under the real units, a test "service" got a real nmap UDP scan result through the helper (raw packets, the ambient capability, the file jail all working together), a public target and a target outside `HELPER_SCOPE` were refused as `scope`, an action not in `HELPER_ACTIONS` was refused as `disabled`, and **a child of the service's main process, same account and same cgroup, was refused as `caller`** (the journal names the peer's pid and uid and the main pid). The Go client passed the same test against the real helper. A child started after `Clear` holds `CapEff` 0 and after `Raise` exactly `CAP_NET_RAW`. Findings that changed the design: a `DynamicUser` unit cannot ask systemd for the main process (pitfall 69), so the unit uses a fixed `llm-proxy-helper` account; `capget` and `capset` are already in `@system-service`.

**Not done, and not measured:** (1) the same checks on vertex (the owner runs `scripts/check-helper-under-unit.sh`, then the real install, then calls the route and runs `agent_terminal_probe.py` against `/run/llm-proxy-helper-net.sock`, expecting a refusal); (2) the service marking itself non-dumpable (D8's second half; it changes `/proc` ownership for the service's own monitoring, so it is measured on its own; **Constitution I.11 does not promise it**, it names `ptrace_scope` as the control); (3) `MaxPerHour` is a cap in the catalogue that nothing enforces yet (`Accept=yes` gives one process per connection and no shared state; `MaxConnections=2` and the action's timeout are the limits in force); (4) no UI, and the result is raw nmap text the service does not parse yet (the Network stack parses it when P3b moves it, and the route returns it as base64 bytes); (5) test 6 (the `isolationtest` suite on the helper's jail profile) and `os_detect` under the unit; (6) D9.

Next: the owner's vertex run; then `os_detect` and the capture (N2c, N2d); the `log` class (N2e) stays deferred by D1.
