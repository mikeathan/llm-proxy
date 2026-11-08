---
status: partial
date: 2026-11-08
last_reviewed: 2026-11-08
related_specs: [SPEC-003, SPEC-006]
constitution_references: [I.11, II.3]
evidence: PR #207 (branch network-helper-in-the-audit, v0.56.0); docs/PLANS/cross-cutting/network-privileged-helper-design.md (last two paragraphs); docs/learnings/network-audit.md (last three lines)
---

# Privileged helper: handoff for the next session (2026-11-08)

Read this first, then the last two paragraphs of `network-privileged-helper-design.md`. This file says where the work stands and what is left; the design says why.

## State

- The helper is installed and ready on vertex with `scan_udp` and `os_detect` switched on (`GET /admin/api/helper`, checked 2026-11-08). The owner enabled it with `--helper`.
- **PR #207** (branch `network-helper-in-the-audit`, v0.56.0) holds everything below. When this file was written it was open and **not run on vertex**.
  1. The helper asks nmap for XML (`-oX -` in `catalog/nmap.go`); the service parses it (`core/netaudit/nmap/probe.go`, `ParseProbe`).
  2. `HelperHandler(action, HelperDeps{Grants, Run, On, Parse, Keep})` answers `parsed` or `parse_error` beside the raw bytes; a cut output is never parsed.
  3. The inventory keeps the last UDP scan and OS detection on the device (`RecordUDPScan`, `RecordOSGuess`; looked up by target IP, newest `LastSeen` wins).
  4. `DeviceProbe.vue` in an open device row: a button per ready action, passphrase asked by the existing step-up prompt, last result shown with its time.
  5. `ProbeFindings` (`core/netaudit/probefindings.go`): a `udp_service` finding for a kept port in state `open` only (SNMP 161 and TFTP 69 medium, UPnP 1900 low), fixed wording, not restricted to devices scanned that night. The Devices tab shows "UDP scan kept for N of M devices, the oldest X".
- **Decided by the owner (2026-11-08):** the helper stays step-up only; **Constitution I.11 is not to be loosened** ("we don't want to open any security holes"). A scheduled audit never calls the helper, by design.

## Do first (needs the owner or vertex)

1. After #207 is merged and deployed: open Network → Devices, expand the router (192.168.50.1), press **UDP scan** (enter the passphrase). Expect `53/udp` or `open|filtered` lines and a "UDP scan, just now" block that survives a page reload. Press **OS detection**; expect a guess or "No guess". If either fails, read the `parse_error` and the raw bytes first: the real `-oX -` output through the helper's jail has only been tested with fixtures. Then run the nightly audit and look for a `udp_service` finding if a scanned device answered on 161, 69 or 1900.
2. OS detection probes only ports 22, 80 and 443 (`osDetectPorts` in `catalog/nmap.go`). nmap needs one open and one closed port to fingerprint, so devices with none open will often say "No guess". Decide from real results whether to widen the list (the list is fixed on purpose; changing it is a code change that passes review).
3. Delete the stray remote branch `network-privileged-helper-n1-design` on GitHub (an earlier push re-created it after #205's merge deleted it).

## Remaining work, in the order I would do it

| # | Item | Notes |
|---|---|---|
| 1 | Stale-result rule | A kept result reports a finding for as long as it exists, however old. Decide (owner): after N days show "old result" in the finding evidence, or stop reporting. Do not drop it silently: a missing finding must not read as fixed. |
| 2 | Attack paths and the Timeline from a kept result | Not built. `attackpaths.go` reads findings by `Check`; a `udp_service` finding may already appear there as a step with the generic title. Check before adding. Timeline summary is typed-fields-only (ADR-0025): keep it that way. |
| 3 | Nightly UDP coverage **without the helper** | Proposal only, unbuilt. SNMP, NTP, SSDP or DNS can be probed with an ordinary unicast UDP socket and no raw-packet permission. First read what `names.Prober` already sends and the wording of II.3, I.7 and I.9; a new kind of probe probably needs its own Constitution item (nearby discovery needed I.9 for two multicast datagrams). Write it up for the owner; do not build before approval. |
| 4 | D9 (sandbox contract) | A terminal or program spawn with no workspace gets a deny-all profile instead of none (`landlock_provider_linux.go:163`). Its own change set with a SPEC-006 update; see the design, section 12. |
| 5 | N2c remainder and N2d | `capture_window` with its frame cap (needs a burst measurement first), then the `isolationtest` suite on the helper's jail profile (test 6). `read_auth_log` (N2e) stays deferred (D1). |
| 6 | D8 second half | The service marking itself non-dumpable; measure on its own (changes `/proc` ownership for the service's own monitoring). |

## Traps and facts (verified this session)

- **The new nmap text on `models.NetworkDevice` must never reach a prompt.** Checked 2026-11-08: no consumer of the inventory (digest, activity, overview, attack paths, run reporter, nearby view) serialises a whole device, and no agent tool reads it; the fields go to the admin UI only. Re-check if any new consumer serialises `NetworkDevice`.
- Inventory findings are restricted to devices scanned that night (`restrictTo`); `ProbeFindings` deliberately is not, or an offline device's kept finding would read as resolved.
- `encoding/xml` rejects a raw ESC in an attribute, so a hostile name is an error, not a cleaned string; tests use `U+202E` and `U+200B`.
- The step-up prompt is automatic: `apiFetch` sees `403 reauth_required`, asks, and resends. The service never keeps a session open for a scan.
- An admin script token (`LLM_PROXY_TOKEN`) can read `GET /admin/api/helper` and `/netaudit/devices` but can never call the helper routes (token principals are refused).
- `go test ./...` failed once in `core/netaudit/routerlog` and passed on rerun; not investigated.
- Comments are never deleted in this repo unless wrong; keep the doc comments on the helper types.

## Checks run for #207 (all passed)

`go build ./...`, `go test ./...`, `go run ./tools/check-complexity/`, `npm test` (2,725), `npm run build`, `npm run test:visual` (344), `./scripts/check-agent-harness.sh`. Nothing was run on vertex.
