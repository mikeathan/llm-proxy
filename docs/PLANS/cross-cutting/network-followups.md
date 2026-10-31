---
status: partial
date: 2026-10-31
last_reviewed: 2026-10-31
related_specs: [SPEC-003, SPEC-006]
constitution_references: [I.7, II.3]
evidence: Recon of 2026-10-31 (§2), read from the code at v0.30.0; the phone-rejoin noise is the owner's report of 2026-10-30 (ledger). Phases 1 to 4 are built and tested (v0.31.0); the 30-minute hold and the port counts are not measured on a real network.
---

# Network follow-ups: port record, name hints, quieter private MACs, IPv6 scanning

**Status:** partial (2026-10-31): phases 1 to 4 are built in v0.31.0; phase 5 (IPv6 scanning) is **design only**, recorded as ADR-0017 (proposed). See As built.
**Related plans:** [`network-detection.md`](network-detection.md) (its Remaining Work lists these), [`ARCHIVE/cross-cutting/network-alerts-in-ui.md`](../ARCHIVE/cross-cutting/network-alerts-in-ui.md).
**Origin:** the owner (2026-10-31), after the live checks of v0.30.0: do the "Task C" candidates (per-scan port record, dismiss a wrong name suggestion), name the "Unknown device" entries so `device_missing` can work, noise tuning for `mac_changed` and rotating private MACs, IPv6 scanning, and adb/Kubelet service names; explain the botnet-C2 list idea.

## 1. Goal and non-goals

**Goal.** Help the operator tell their devices apart and name them; stop rotating phones from raising repeated alerts; report ports that opened or closed; and (as a proposal for security review) let the audit scan on-link IPv6-only devices.

**Not in this plan:** a vendor (OUI) table; `device_missing` for unnamed devices; adb and Kubelet service names (§5); a bundled botnet-C2 list (§7).

## 2. What exists (verified 2026-10-31, v0.30.0)

- `changeIsNotable` (`netaudit/inventory.go`) is the one predicate for both the `mac_changed` finding and the alert (`pendingOf` calls it). `pendingOf` alerts every unapproved, active, not-yet-alerted device at the next presence tick, private MAC or not.
- No per-scan port record. `ScanBaseline.Findings` holds `open_port` findings keyed by IP, and `Diff`/`restrictTo`/`scannedDevices` already treat an `Unscanned` host as "not looked at". `models.NetworkDay` holds finding counts only; the weekly summary therefore says nothing about ports (network-guard-module Phase 4 deviation).
- Name suggestions: `models.NameSuggestion` per MAC, re-asked after 7 days; no way to reject one. `device_missing` needs a non-empty name, a non-private MAC and an IPv4 address (`InventoryFindings`).
- IPv6: `NetworkDevice.IPv6` is display only ("never used to key anything"). The scan scope refuses IPv6 and link-local (`scanscope.checkScannable`, Constitution II.3 item 3, tested in `scanscope_test.go`).

## 3. Design

**3.1 Quieter private MACs.** (a) `changeIsNotable` ignores a swap where both the old and the new MAC are private (locally administered): a phone rotating its address at the same IP. One predicate, so the finding and the alert agree. (b) A new device with a private MAC is alerted only once it has been on the network `PrivateHold` (30 minutes, a named constant; the owner chose it, nothing in the repo measures it) and is still there; those that mature at the same tick go out as one message with a count. The `new_device` finding is unchanged. **Accepted residual risk:** an impostor that takes an approved phone's address with a random MAC raises no `mac_changed`; a non-private MAC, the router's MAC check and `duplicate_mac` stay exactly as loud.

**3.2 Per-scan port record.** A small store (`device-ports.json`, removed by a factory reset) keeps, per device, the ports open at its last completed scan: port number and protocol only (no banner, no product string, nothing a device controls), keyed by **MAC**, resolved through the inventory at scan time (an IP key would pin ports on the wrong device after a DHCP change). Only hosts the scan looked at (`Up` and not `Unscanned`) update their record, so a partial run never closes another device's ports; a device with no earlier record is a baseline, not "everything opened". `History` gains `PortsOpened` and `PortsClosed` per day, added up across the day's audits; the weekly summary gains one fixed-words line with the week's totals (counts only).

**3.3 Dismiss a wrong name suggestion.** `NameSuggestion.Dismissed` holds the name the operator rejected; `All()` does not offer a name equal to it, the 7-day re-ask keeps it, and a different name later is offered. `POST /admin/api/netaudit/devices/{mac}/dismiss-suggestion`; a "Not this" button beside "Use this name".

**3.4 Naming the unknown devices.** A prompt on the Devices card, "N devices have no name", opens a list of the unnamed ones, each with what helps tell it apart: last seen, the open ports from 3.2, and the name it gave if any. Naming uses the existing rename flow; a suggestion is still never an approval (one tap never approves a new device). Private-MAC and IPv6-only devices are listed but not counted in the prompt, because `device_missing` cannot work for them.

**3.5 IPv6 scanning (a proposal; in force only if the owner merges the PR that carries the amendment).** Scope: **on-link, IPv6-only devices** (no IPv4 address in the inventory), addresses taken only from the host's neighbour table on an interface that shares a segment with an in-scope IPv4 device. The operator cannot type an IPv6 target; no routed or global address is ever a target; dual-stack devices are scanned over IPv4 only, so nothing is reported twice. Off until switched on; switching on is step-up. **Finding key:** the device's MAC (`Finding.Device` is "an IPv4 address, or a MAC for an IPv6-only device"), because IPv6 privacy addresses rotate daily. Needs: the Constitution II.3 item 3 and I.7 rewrites, ADR-0017, the guarded-program checks (arguments built from a fixed table, zone id handling), tests that today pin "the scope never contains IPv6" changed deliberately.

## 4. Strain and safety

No new periodic sweep or listener. 3.1 and 3.3 add no network traffic. 3.2 reads results the audit already has; one small file write per completed scan. 3.5 adds scan targets only for devices already in the neighbour table, in the nightly audit, never from the presence check.

## 5. adb and Kubelet service names: not done

nmap 7.99 has no service match for either and the owner has no device running one (2026-10-31), so there is nothing to capture. A name written without a capture would be a guess in a security report; they stay on the port rule.

## 6. Decisions (owner, 2026-10-31)

- Unknown devices: hints and one naming list, no vendor table, no `device_missing` for unnamed devices.
- Private MACs: quiet private-to-private swaps and hold new private devices for 30 minutes, then one count.
- IPv6: on-link IPv6-only devices, keyed by MAC, as a proposal for security review.
- One branch, one PR.

## 7. The botnet / C2 list (idea, not designed)

See the report to the owner of 2026-10-31. Short form: a list can only be checked against **destinations**, and the app has none today (the router is the DNS server; packet capture is out of scope; the host's own connection table covers one machine). It becomes possible only with a source such as an AdGuard Home or Pi-hole query log. Licence and key terms of the candidate feeds are unchecked.

## 8. Phases

1. **Quieter private MACs (3.1).** Red: a private-to-private swap is not notable; a non-private swap still is; the alert and finding agree; a private device under 30 minutes is not pending, at 30 minutes it is, two maturing together make one message with a count. *Accept:* `go test ./...`, complexity.
2. **Port record (3.2).** Red: first scan is a baseline; an opened and a closed port count once; an `Unscanned` or partial run closes nothing; the MAC key survives an IP change; counts add up over two audits in a day; the digest line is counts only. *Accept:* `go test ./...`.
3. **Dismiss a suggestion (3.3).** Red: dismissed name hidden, survives the 7-day re-ask, a different name is offered. *Accept:* handler and suggester tests.
4. **Naming list (3.4).** Frontend: prompt, list, hints, 44 px targets, visual spec. *Accept:* `npm test`, `npm run build`, `npm run test:visual`.
5. **IPv6 scanning (3.5).** Amendment text, ADR-0017, scope, argv, parser, finding key, switch. *Accept:* the tests above, and the owner's review of the amendment.
6. **Docs.** SPEC-003 2.23, hub, ledger, plan, `network-detection.md` Remaining Work, version 0.31.0, built `index.html`, harness check.

## As built (2026-10-31, v0.31.0)

- **3.1** `changeIsNotable` ignores a private-to-private swap (one predicate for the finding and the alert); `worthTelling` holds a private-MAC newcomer until `LastSeen - FirstSeen >= PrivateHold` (30 minutes). Devices that have stayed are told in one message with a count, which is what `PresenceMessage` already does for several pending. Two older tests that expected a phone to be alerted at once were changed on purpose. **Mutation checked:** the new tests fail without the change.
- **3.2** `netaudit/portlog.go` (`PortLog`, `PortChange`), `persistence.DevicePortStore` (`device-ports.json`), `models.DevicePorts`; `RunReporter.WithPorts` records after a saved audit, resolving IP to MAC through the inventory the audit just read (`macByIP`); `History.RecordPorts` adds `PortsOpened`/`PortsClosed` to the day; the weekly summary says "N ports opened and M closed this week" from audited days only. **Limit:** a scan that probes a different port list than the previous one (the operator changes thoroughness) counts the difference once; the record does not remember which ports were probed.
- **3.3** `Suggester.Dismiss` / `NameSuggestion.Dismissed`; `POST /admin/api/netaudit/devices/{mac}/dismiss-suggestion`; "Not this" beside the suggestion (also on a new device, where the suggestion is information only).
- **3.4** The Devices card shows the open ports and scan time under each device (`DeviceView.OpenPorts`, `PortsScannedAt`) and a prompt "N devices have no name" with **Name them**, which lists only the nameable ones (no name, real MAC, IPv4 address).
- **3.5** **Not built.** ADR-0017 records the decision, the proposed Constitution text and why the code waits: `Finding.Device` is the key of acceptances, the diff, the report and the UI, and no `nmap -6` output from a real on-link device exists. `CONSTITUTION.md` is unchanged.
- **§5** adb and Kubelet service names: not done (nothing to capture).

## Remaining Work

- **IPv6 scanning (3.5):** the PR that carries the Constitution II.3(3)/I.7 change and the code. Needs first: one real `nmap -6` run against an on-link device (argument shape, zone id of a link-local target, XML address type), then widening `Finding.Device` to "IPv4 address or MAC" through `ack.go`, `diff.go`, the report, the fix guide and the UI.
- **Unverified:** the 30-minute hold on the owner's phones; that the port counts match what the owner sees; the first scan after upgrading records a baseline, so ports opened/closed start counting from the second audit.
- **Not done:** adb and Kubelet service names (§5); a vendor (OUI) table and `device_missing` for unnamed devices (§1).
