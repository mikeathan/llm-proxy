---
status: partial
date: 2026-11-06
last_reviewed: 2026-11-06
related_specs: [SPEC-003]
constitution_references: [I.8, I.9, I.10]
evidence: vertex (Ubuntu 24.04) 2026-11-06 — `id llm-proxy`, `ls -l /var/log/btmp /var/log/auth.log`, a `systemd-run` read of btmp under the unit's restrictions (384 bytes), a real two-record btmp sample, `iw dev`, `rfkill list`, and a `systemd-run` `iw scan dump` under the unit's address families ("Failed to connect to generic netlink"); vertex's findings of the last audit through the admin API. Owner decisions: design proposal answered "go ahead and do all the work in same branch" (UI not decided, to be reviewed afterwards).
---

# TLS checks, encrypted DNS, failed logins, address watch, and what each module says about itself

**Status:** partial (2026-11-06). Features 1 to 4 and the discoverability work are built in v0.54.0 on branch `network-tls-dns-ssh-address-and-discovery`; feature 5 (evil twin) is **not built**: see Remaining Work. Constitution I.8, I.9 (clauses) and I.10 are *proposed* text for the owner's review, in force only when this branch is merged. **Related:** [ADR-0030](../../decisions/ADR-0030-failed-logins-are-read-from-btmp-with-the-utmp-group.md), [ADR-0029](../../decisions/ADR-0029-the-service-keeps-memory-deny-write-execute-so-nuclei-is-removed.md), [`stack-modules-and-new-detection.md`](stack-modules-and-new-detection.md).

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
- **Tune from real data:** the burst figures (10 in 10 minutes) and whether `tls_self_signed` is too many findings (vertex has four self-signed 8443 services: the router, the mesh node and two Chromecasts).
- **Feature 5, evil twin / rogue Wi-Fi — not built, not infeasible.** vertex has a Wi-Fi card (`wlp2s0`, managed, not associated) and the unit forbids what a scan needs: `RestrictAddressFamilies` has no `AF_NETLINK` (an `iw` scan under the unit's families fails with "Failed to connect to generic netlink") and the capability set is empty (a scan needs `CAP_NET_ADMIN`). **`NetworkManager` and `wpa_supplicant` are both active on vertex**, so the `AF_UNIX` D-Bus route is open in principle: `nmcli -t -f SSID,BSSID,SIGNAL,SECURITY device wifi list --rescan no` reads the access points NetworkManager already knows. Not tried under the unit's restrictions (needs the owner to run it: the command is in the final report of this work); if it works it needs, in order: a guarded-program entry for `nmcli` (Constitution II.3 amendment), a setting for the owner's network name, a parser for the terse output, and a finding for a copy of that name from a BSSID not seen before. If it does not work (polkit, or `MemoryDenyWriteExecute` for glib), record it here and drop the idea.
- Not built, by decision: automatic blocking through the router, default-password testing (intrusive; out of scope).
