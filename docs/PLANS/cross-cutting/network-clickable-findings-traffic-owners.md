---
status: partial
date: 2026-11-11
last_reviewed: 2026-11-11
related_specs: [SPEC-003, SPEC-006]
constitution_references: [I.2, I.8, I.13]
evidence: docs/decisions/ADR-0043 (sources measured 2026-11-11: IPtoASN downloaded and parsed, 579,453 ranges); backend/internal/platform/privhelper/capture/filter_host.go and host_linux_test.go (a real kernel in a container with CAP_NET_RAW, 2026-11-11); backend/internal/core/netaudit/hosttraffic/ and ipowners/; frontend components named in the SPEC-003 entry 2.60.
---

# Network: problems link to where they are, and the Traffic page says who runs a destination

Branch `network-clickable-findings-traffic-owners` (from origin/main at v0.70.0). The owner asked on 2026-11-11 for (1) every place that lists an issue to link to it, (2) more about where traffic goes: destination, location and anything useful, shown well, and (3) the TLS server name that the traffic look did not read.

## Built (all checks run, nothing run on vertex)

- **Links.** `toFinding` and `toDeviceFindings`, `useFindingFocus` (scroll, mark for 3 s, "not in the latest report any more"), `useDeviceProblems` (provided by `NetworkView`). Linked from: attack-path steps, fixed and returned Timeline rows and the audit row, the Overview's Problems figure and the standing card, the device row (beside the row, not inside its button) and the Traffic detail of a device.
- **TLS server name.** The filter keeps 768 bytes of the first segment of a ClientHello and nothing more of any other frame; `hosttraffic/tls.go` reads the `server_name`; the conversation's own name beats the address's DNS name, and the view says which it was. Constitution I.13 amended.
- **Who runs a destination.** `core/netaudit/ipowners` (IPtoASN, public domain), downloaded on a step-up press, looked up in memory; owner line on each destination and conversation, "Where it went" lists by holder and by country, a download card on the Traffic tab. No map (ADR-0043 says why).

## Remaining Work

1. **Owner, on vertex:** deploy; reinstall the helper binary so the new filter runs (`sudo ./setup.sh --install --build --unattended --helper`); press "Download the table"; listen once; read the lists; check that a name marked "from a TLS handshake" is a real server name.
2. **Measure on a real host:** how often the name lies past 768 bytes; whether hellos push the sampling rate up needlessly (the doubling counts every kept frame).
3. **Decide after 1:** whether a destination should also link to the finding it caused (the phone-home watch's new-place finding is keyed on a device, not an address; no link was invented for it).
4. `npm run test:visual` was not run (see the PR).
