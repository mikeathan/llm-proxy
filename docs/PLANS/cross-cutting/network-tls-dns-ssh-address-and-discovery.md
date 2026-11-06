---
status: partial
date: 2026-11-06
last_reviewed: 2026-11-06
related_specs: [SPEC-003]
constitution_references: [I.8, I.9, I.10]
evidence: vertex (Ubuntu 24.04) 2026-11-06 — `id llm-proxy`, `ls -l /var/log/btmp /var/log/auth.log`, a `systemd-run` read of btmp under the unit's restrictions (384 bytes), a real two-record btmp sample, `iw dev`, `rfkill list`, and a `systemd-run` `iw scan dump` under the unit's address families ("Failed to connect to generic netlink"); vertex's findings of the last audit through the admin API. Owner decisions: design proposal answered "go ahead and do all the work in same branch" (UI not decided, to be reviewed afterwards).
---

# TLS checks, encrypted DNS, failed logins, address watch, and what each module says about itself

**Status:** partial (2026-11-06). Features 1 to 4 and the discoverability work are built in v0.54.0 on branch `network-tls-dns-ssh-address-and-discovery`; feature 5 (evil twin) was built afterwards in v0.55.0: see [`network-wifi-lookalikes-and-unix-socket-jail.md`](network-wifi-lookalikes-and-unix-socket-jail.md). Constitution I.8, I.9 (clauses) and I.10 are *proposed* text for the owner's review, in force only when this branch is merged. **Related:** [ADR-0030](../../decisions/ADR-0030-failed-logins-are-read-from-btmp-with-the-utmp-group.md), [ADR-0029](../../decisions/ADR-0029-the-service-keeps-memory-deny-write-execute-so-nuclei-is-removed.md), [`stack-modules-and-new-detection.md`](stack-modules-and-new-detection.md).

## What was built (one home for each job, no new layers)

| # | Feature | Home | Switch |
|---|---|---|---|
| 1 | Certificate checks | the **Web checks, nightly** module, now titled "Web and certificate checks, nightly" (`webprobe.ProbeTLS`, `tlsfindings.go`) | the same one |
| 2 | Public encrypted-DNS names | the **Phone-home watch** (`phonehome.EncryptedDNS`, `phonehome.Judge`) | the same one |
| 3 | Failed logins to this computer | a new Listen-layer module **Failed logins, this computer** (`platform/loginlog`, `LoginWatch`) | new, not step-up, off by default |
| 4 | Address watch | the **Presence check** (`changeIsNotable`, `Pending.Shared`) | the same one |
| – | Each module says what it does | `stack.About` and `Finds` on every module (`app/stacks_about.go`); the Modules tab fold, the coverage line on the Overview, the "found by" line on a problem card, the Help list | none |

**Decisions that differ from the first proposal (all recorded in the ledger):**
- **TLS reuses the nmap checks' ids and words** (`tls_cert_expired`, `tls_cert_expiring`, `tls_weak_protocol`, with the same "valid for 14 days or less" exemption) because nmap's `ssl-cert` and deep-check scripts already make them; one finding stands for both, the nmap one is kept, and a completed handshake replaces a *carried* deep-check `tls_weak_protocol` finding. New: `tls_self_signed` (low). **Name mismatch was not built**: a LAN device is reached by address, and a certificate for a name never matches an address, so the check would fire on nearly every device.
- **Encrypted DNS is a finding only, never a message**, and `use-application-dns.net` is left out of the list: Firefox asks for the canary and for `mozilla.cloudflare-dns.com` by default, and a phone's private-DNS setting asks for its server, so a message would be mostly noise.
- **Address watch**: the router's hardware-address check, the shared-address finding and the "address now used by a different device" alert already existed. New: an address held by a device marked **Important** that another device takes is notable even when that device is approved, and a hardware address answering for several addresses is now a message (once per set), not only a finding. **The DNS server is not a separate anchor**: it is almost always the router, and a LAN DNS server can be marked Important.
- **The failed-login source is `btmp` with the `utmp` group**, not `auth.log` with `adm` (ADR-0030).

## Remaining Work

- **Owner:** apply the unit change on vertex (`setup.sh` re-run or a drop-in with `SupplementaryGroups=utmp`, then restart), switch **Failed logins, this computer** on, and review Constitution I.10 and the I.8/I.9 clauses. After deploy: `GET /stacks/network` should show the module working; a deliberate failed SSH password login should show in the btmp count within a minute (the burst needs ten).
- **Tune from real data:** the burst figures (10 in 10 minutes) and whether `tls_self_signed` is too many findings (corrected 2026-11-06: none of vertex's four 8443 services is self-signed: the router and mesh node are issued by the router's own private CA, the Chromecasts by Google's, so it should report nothing on that network).
- **Feature 5, evil twin / rogue Wi-Fi — feasible, not built, waiting for the owner's go-ahead on the design below.** vertex has a Wi-Fi card (`wlp2s0`, managed, not associated). A scan through netlink is forbidden by the unit (`RestrictAddressFamilies` has no `AF_NETLINK`: "Failed to connect to generic netlink"; empty capability set). **The D-Bus route works under the unit's restrictions** (owner's run, 2026-11-06, `systemd-run -p User=llm-proxy -p RestrictAddressFamilies="AF_UNIX AF_INET AF_INET6" -p CapabilityBoundingSet= -p NoNewPrivileges=yes -p MemoryDenyWriteExecute=yes -p ProtectSystem=strict -p SystemCallFilter=@system-service -- nmcli -t -f SSID,BSSID,SIGNAL,SECURITY device wifi list --rescan no`): it listed 27 access points from NetworkManager's own scan, including the owner's `netsink`, `netsink-2G` and `netsink-Guest` from several hardware addresses (router and mesh node) and the neighbours'. Proposed build: a read-only listing through `progexec` (clean environment, fixed arguments, no connect and no rescan; Constitution II.3 amendment, proposed), a setting for the owner's network name(s), the owner confirming which hardware addresses are theirs the way the router's is confirmed, and a finding when a confirmed name is advertised by an unconfirmed hardware address or with weaker security. **Unverified and the main risk:** whether the Landlock jail `progexec` puts the child in lets it reach `/run/dbus/system_bus_socket` (the `Invocation.ReadPaths` extra read path exists; the first run through the real app on vertex decides, as with nuclei and `MemoryDenyWriteExecute`).
- Not built, by decision: automatic blocking through the router, default-password testing (intrusive; out of scope).
