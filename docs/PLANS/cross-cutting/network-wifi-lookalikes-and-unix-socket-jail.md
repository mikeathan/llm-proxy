---
status: partial
date: 2026-11-06
last_reviewed: 2026-11-06
related_specs: [SPEC-003, SPEC-006]
constitution_references: [II.3]
evidence: vertex (Ubuntu 24.04, kernel 6.8) 2026-11-06 - the owner's runs of `nmcli device wifi list --rescan no` and `nmcli general permissions` under the unit's real restrictions (27 access points listed; every state-changing NetworkManager action `no` or `auth`), and a Landlock probe in a container on the same kernel (a read outside the grant denied, a path-based Unix socket connection allowed). Owner decision: build the evil-twin check, and fix the sandbox gap first.
---

# Wi-Fi look-alikes, and Unix sockets refused to every jailed child

**Status:** partial (2026-11-06): built in v0.55.0 on branch `network-wifi-lookalikes-and-unix-socket-jail`; neither part has run on vertex. Constitution II.3's two new bullets are *proposed* text in force when the branch is merged. **Related:** [ADR-0031](../../decisions/ADR-0031-the-wifi-look-is-a-guarded-nmcli-with-no-network-and-only-unix-sockets.md), pitfall 67, [`network-tls-dns-ssh-address-and-discovery.md`](network-tls-dns-ssh-address-and-discovery.md) (feature 5 of that plan).

## What was built

1. **The jail's Unix-socket gap is closed.** Landlock (ABI up to 8) does not control the connection to a path-based Unix socket, so a jailed child could reach the system D-Bus. The runner installs a seccomp filter (`platform/sandbox/seccomp_linux.go`, linux/amd64 and arm64) that refuses `socket(AF_UNIX)` (EACCES) and `io_uring_setup` (EPERM) for every child, unless the spawn site allows Unix sockets (`WorkspaceView.AllowUnixSockets`, `progexec.Invocation.AllowUnixSockets`) or the operator writes `sandboxing.unix_sockets: true`. `Effective().unix_sockets` reports it. A runner asked for the filter that cannot apply it fails closed. `progexec.Invocation.NoNetwork` gives a program with no use for TCP the Landlock TCP denial.
2. **The Wi-Fi look-alike check** (module *Wi-Fi look-alikes*, Discover layer, off by default, not step-up): `core/netaudit/wifilook` runs `nmcli -t -f SSID,BSSID,SIGNAL,SECURITY device wifi list --rescan no` as the one guarded program that is allowed Unix sockets and has no network; `WifiWatch` looks every ten minutes and tells, in fixed words, of an access point that advertises the operator's own network name from an address the operator has not confirmed; one nightly finding (`wifi_lookalike`, high when a copy has no real security). The operator types the network names and confirms, from a look, which access points are theirs (a step-up action); a name with nothing confirmed is not judged. The Settings card is *Wi-Fi look-alikes*.

## Remaining Work

- **Owner, on vertex after deploying:** (a) check that an agent terminal command **and a network scan or deep check** still run (nmap is a guarded program and now runs under the filter; the filter was tested on arm64 only here, and CI's Ubuntu sandbox job is the amd64 evidence). If they do not, set `sandboxing.unix_sockets: true` (Security page, or `/var/lib/llm-proxy/settings.yml`) and `sudo systemctl restart llm-proxy`; (b) install is already there (`nmcli` ran); switch the module on, type the network name(s) (`netsink`, `netsink-2G`, ...), press Look now, confirm the router's and the mesh node's addresses per name; (c) review Constitution II.3's new bullets.
- **First run through the real app is the test of the jail:** `GET /stacks/network` should show the module working; a look that fails shows as stopped with the program's whole error. If the Landlock jail stops `nmcli` reaching NetworkManager (only the real unit shows that), record it here; do not loosen the jail without a decision.
- **Possible agent breakage, to watch for:** programs that make a Unix socket themselves now get "permission denied": tmux, an ssh ControlMaster, syslog, a D-Bus or docker client, Playwright or Chromium (its profile singleton), Python 3.14's default multiprocessing start method (forkserver), and `.local` lookups through nss-mdns/avahi. The Security page has a switch, "Let programs use Unix sockets", with the effective state beside it.
- Not built: a check that a confirmed access point has *dropped* its security (a router change); an amd64 run of the filter test; a CI expectation (`LLMPROXY_EXPECT_UNIX_DENY`) in the conformance matrix (the workflow was not changed: CI changes are the owner's call; the conformance test itself now asserts the surface on Linux).
