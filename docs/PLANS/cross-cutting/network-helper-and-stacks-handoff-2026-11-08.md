---
status: partial
date: 2026-11-08
last_reviewed: 2026-11-08
related_specs: [SPEC-003, SPEC-006]
constitution_references: [I.7, I.9, I.11, II.3]
evidence: branch network-helper-attack-paths-and-timeline (v0.57.0); PR #207 merged as eb41b629; docs/PLANS/cross-cutting/network-nightly-udp-probes.md; docs/PLANS/cross-cutting/network-capture-window-plan.md; docs/PLANS/cross-cutting/network-stack-p3b-design.md; docs/learnings/network-audit.md (last line)
---

# Network helper and stacks: handoff for the next session (2026-11-08)

This replaces `network-helper-handoff-2026-11-08.md` (deleted). Read this, then the two new plans.

## State

- **#207 is merged and deployed to vertex.** The owner enabled the helper's UDP scan and OS detection and pressed them on 2026-11-08. **What the real runs returned was not reported to me:** ask whether the router's UDP scan and OS detection gave parsed results or a `parse_error` (the helper's real `-oX -` output was only tested with fixtures), and whether OS detection says "No guess" for devices with none of ports 22, 80, 443 open (`osDetectPorts`, `catalog/nmap.go`).
- **Built on this branch (v0.57.0):** a **Clear** button beside each kept result in an open device row (`DeviceProbe.vue`), backed by `Inventory.ClearProbe` and `clear_probe` on `PUT /admin/api/netaudit/devices/{mac}` (not step-up, like accepting a finding). The stale-result rule is decided by the owner: show the old result with its date, clear it on request, or replace it by a re-scan; clearing the UDP result removes its `udp_service` finding. Checks run: `go build`, `go test ./...`, complexity, `npm test` (2,727), `npm run build`, `npm run test:visual` (344), harness check.
- **Attack paths checked:** `udp_service` is not in `stepChecks` (`attackpaths.go:108`; an open port is not a step). Left out on purpose. **The Timeline was not checked.** Its summary is typed-fields-only (ADR-0025); keep it that way.
- **Two plans written, nothing built, both need the owner:**
  - `network-nightly-udp-probes.md`: SNMPv3 engine discovery and a unicast SSDP M-SEARCH through the guarded dial, a draft Constitution item 12, bounds, silence-is-unknown, duplicate handling. Left out with reasons: SNMP v1/v2c `public` (a default-password guess), DNS (ADR-0023), TFTP, NTP. **Correction to something I said earlier in the session:** I first told the owner "no security change", which was too quick: the SNMP `public` and TFTP probes would be new classes of traffic, so only the credential-free two are proposed, with a new item.
  - `network-capture-window-plan.md`: a manual 60 s headers-only snapshot, not real-time (the helper is step-up only), on a Wi-Fi link that sees only broadcast and multicast. The owner measures M1 to M3 on vertex first; no findings in the first slice.

## Do next (needs the owner)

1. Report the vertex results of the router UDP scan and OS detection.
2. Approve (or change) the nightly UDP probe list and Constitution item 12. Then build slices 1 to 3 of that plan.
3. Run the `capture_window` measurements M1 to M3 and decide go or no-go.
4. Delete the stray remote branch `network-privileged-helper-n1-design`.
5. Phase 3 (Router DNS check) data was due about 2026-11-08: bring back "Copy the results"; a finding only if there were no false alarms, else drop the phase.

## Left in the stacks work (not done in this branch, on purpose)

- **P3b slices S2 to S6** (`network-stack-p3b-design.md` section 4): each is its own branch and PR with the performance baseline re-measured (the owner's condition, section 4a), and the privileged-additions plan says the helper work must not be combined with S2. S2 is the DNS check vertical (a `Fetch` port, the `Config` adapter, module status, routes, lifecycle) and ships as one PR.
- **17 files still import `platform/sandbox` or `platform/progexec` directly** instead of `platform/isolation` (counted 2026-11-08 with a grep for non-test files). The switch is mechanical but touches the sandbox path, so it is its own change set.
- P4 harvest (architecture map, hubs, SPEC-006, pitfalls) after S6.
- The README rows for the stack and helper plans were refreshed this session.

## Still open from the earlier handoff

- OS detection port list: decide from real results (a code change that passes review).
- Nightly UDP without the helper: now the plan above.
- D9 (a spawn with no workspace gets a deny-all profile, `landlock_provider_linux.go:163`; SPEC-006 update), D8 second half (service non-dumpable), N2d/N2e: unchanged. nuclei through the helper: not proposed, needs the owner to accept relaxing the memory rule for the helper's own unit (ADR-0029).
- `go test ./...` failed once in `core/netaudit/routerlog` earlier and passed on rerun; not investigated.

## Traps

- Do not stash, reset or restore to compare files: AGENTS.md forbids it. (I tried `git stash` once this session; the permission check stopped it before it ran.)
- The new nmap text on `models.NetworkDevice` must never reach a prompt; re-check if any consumer serialises a whole device.
- Inventory findings are restricted to devices scanned that night; `ProbeFindings` deliberately is not.
- A UDP port with no reply is `open|filtered`, never "open"; only `open` makes a finding.
