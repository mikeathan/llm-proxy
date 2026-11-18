---
status: partial
date: 2026-11-16
last_reviewed: 2026-11-16
related_specs: [SPEC-006, SPEC-012]
constitution_references: [I.16, I.17, II.3]
evidence: Session of 2026-11-16 (PR #259 merged and deployed as v0.94.0). The owner's paste of the first real radio_digest report (reports/radio-watch-report/20261116T200724Z-31c6c3c8d60bb59b) and read-only GETs of vertex's radio/air/devices, air/airwaves and air/live (45 devices, 95 listened cycles, first seen 14 to 16 November). Delete this file when the work merges (harvest first, documentation-stewardship).
---

# Radio: report wording and device patterns (2026-11-16)

## What vertex showed (read-only GETs; counts, kinds and one-way codes, no addresses)
45 other devices over 95 listened cycles: 20 unknown, 17 personal, 3 infrastructure, 3 transient, 2 neighbour. **13 announce a Wi-Fi generation, so they are beacon senders (access points)**; the tag of a beacon sender is the same keyed tag as its device row (`recorder.go writeNetworks`).
- **Access points sorted wrongly.** The owner's own ASUS mesh (three radios, always here) landed as 1 infrastructure and 2 unknown (`no_rule_fits`), because the infrastructure rule demands a steady signal (spread of 5 dB or less) and an access point's signal wobbles. A random-address access point that was heard in 100% of cycles (a virtual network address) is unknown too. Four access points that come and go (two Arcadyan, one a neighbour's EE router) were sorted **personal**: a strong signal that comes and goes is how the personal rule sees any passing router.
- **7 of the 10 `no_rule_fits` devices were heard in 76% to 100% of cycles.** No rule covers "always here, signal not steady".
- **17 personal is too many for a house**: 4 are access points, 2 are Espressif (a smart-home module maker, 2.4 GHz only), the rest are strong and intermittent.
- **Channel 149 is not empty** (3 devices, heard in 11% to 15% of cycles): the two empty cycles seen on the Listening tab were chance.
- The first report read well but said "One of your…" beside counts of 3, 6 and 2; put the "no management-frame protection" finding first although it is common on home routers; printed the template's "Status: PASS" line; and gave no count of other people's devices.

## Constraints that bind (do not break)
ADR-0053: for other people's devices only day-level rows are kept (`daily`: cycles, frames, signal sums and maximum, channel mask, days present) plus `devices.random`, `joined_own`, `maker`, `wifi_gen`. Any new per-device column for others needs the owner. Rules are a versioned document: new rule code is `classifier_v2`, a new `class_ver`, and a `class_log` row with a reason for every change. The sorting stays **provisional**. Phones re-randomise their address, so one phone can look like several brief devices: a brief random-address device is not a passer-by with confidence.

## Built in this change (v0.95.0, branch `radio-report-wording-and-device-patterns`; not run on vertex)
1. **Report text.** Count-neutral claims and a fixed, hedged `means` line per finding (stack `claimWords`/`meaningWords`, core `claims`, held word for word equal by `TestRadioDigestTool_TheCoreHasWordsForEveryFindingTheStackOffers`; the Findings panel shows the meaning line too); the report profile may relay a finding's claim and meaning line and nothing more, and ranks by them; the header gives the total of other people's devices and the counts by kind in plain words ("first-guess (provisional) rule set"); the report is told to print no status line (the executor does not read one: checked); the report card shows the **saved report file** (`last_report_path`) as formatted text (`MarkdownViewer`), because the server's `last_output` is the run's log with the executor's own lines around the report (read from vertex).
2. **Rules version 2** (provisional, `classes.go`): an access point (a device whose beacon or probe response announced a Wi-Fi generation; a phone's probe request does not, but a phone hotspot does) is a neighbour's when it was never strong, the house's own infrastructure when always here, a neighbour's again when it comes and goes (never personal); a device heard in 80% or more of cycles is infrastructure even when its signal moves; a device whose maker is on the rules' small smart-home list (Espressif, Tuya, Shelly; each checked against the embedded registry by a test) is infrastructure unless its address is random. A store on version 1 is moved to version 2 by `CurrentRules`: the next cycle re-sorts every device and `class_log` records each change with version 2 and its reason. **Retired:** the finding `radio_unknown_strong_device` (see `radio-stack.md`): with these rules no strong device is left unplaced, and it had been raised by the owner's own access points.
3. **UI**: the reasons in plain words (`RADIO_REASONS`, held complete by `TestEveryReasonTheRulesCanGiveHasItsWordsOnThePage`, which also found `owner_said` and `first_look` had none); "Access point" in a device's facts; the finding's meaning line.
**Open:** the neighbour rule for an access point that comes and goes does not exclude the owner's *confirmed* access points (a mesh node that drops out sometimes would be sorted a neighbour's); `own_aps` is not read by the rules. **Not built:** the sanity read of the rules over the owner's own marked devices (needs the owner's marks; the live class log after deploy shows the movement); anything per hour for neighbours (ADR-0053).

## Left (needs data or the owner)
- Fit thresholds (share, spread, strong) to a week of data and the owner's marks; the numbers are still guesses.
- Presence strip for neighbours and a time scrubber: mockups first (the owner picks).
- A real run of the changed report prompt on vertex (press Analyse and paste).
