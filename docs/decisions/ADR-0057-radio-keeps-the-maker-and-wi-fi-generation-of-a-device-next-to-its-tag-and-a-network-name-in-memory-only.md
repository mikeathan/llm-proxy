---
id: ADR-0057
title: Radio keeps the maker and Wi-Fi generation of a device next to its tag, and a network name in memory only
status: accepted
date: 2026-11-15
subsystems: [network-audit]
supersedes:
superseded_by:
---

# ADR-0057: Radio keeps the maker and Wi-Fi generation of a device next to its tag, and a network name in memory only

**Accepted 2026-11-16:** the pull request that carries it (#258) was merged by the owner, which this ADR named as its acceptance.

Proposed 2026-11-15; becomes accepted when the owner merges the pull request that carries the amended Constitution I.16. Amends [ADR-0053](ADR-0053-radio-keeps-a-keyed-tag-for-other-peoples-devices-and-seals-the-owners-with-hard-size-and-age-limits.md) (what is kept beside a neighbour's tag).

## Context
The Around you page showed a neighbour as eight hex digits and a class. The owner could not tell which dot was a router, a TV, a plug or a phone, so could not say what was theirs: "we display what we can extract but we store the hash" (owner, 2026-11-15). ADR-0053 keeps, for another person's device, a keyed tag, a class, a signal band, counts and days, and lists as never stored the address, probe-request names, device-chosen names, per-frame traces and anything finer than a day. Two things a frame says about a device are neither stored nor forbidden: who the address block was assigned to, and the Wi-Fi generation the device announces. A network name an access point broadcasts is data the same ADR keeps only as a length and a hash.

## Decision
- **The maker is derived when the frame is parsed and only its name is kept.** The first bytes of the address (never a locally administered, random address, and never a group address) are looked up in the IEEE registry embedded in the program (`stacks/radio/oui_data.tsv.gz`, 54,000 entries, built by `tools/ouigen` from the MA-L, MA-M and MA-S registries; no lookup leaves the computer). The name, such as "Espressif", is stored in `devices.maker` beside the tag. The address is not.
- **The Wi-Fi generation** (4, 5 or 6) a device's frames announce is kept in `devices.wifi_gen`; it only grows.
- **A network name stays in memory.** The name an access point broadcasts is shown while the device is heard in this run and is forgotten when the run ends (like the live picture); it is never written to the disk, so ADR-0053's "another network's name is kept only as a length and a hash" still holds.
- **Marking a device as the owner's from the radar** (`POST radio/air/mine/{id}`, by the short id the radar shows) sets the owner flag and the owner's name on the existing row. Only the tag of a neighbour is known, so there is no address to seal: the mark holds while the tag matches (for a phone with a random address, until it changes address) and ends when the neighbours are forgotten.
- **Everything else in ADR-0053 stands:** no address, no probe-request name, no frame, nothing finer than a day on the disk, the 14-day age and 20 MB size limits. A test reads the database file and finds neither an address nor a network name.

## Consequences
- A device with a registered address (an access point, a TV, a bulb, a plug, a camera) can be named by its maker; a phone or laptop with a random address cannot, and the page says "random address".
- A maker is a coarse attribute shared by many devices; with the tag it narrows who a device is. The legal position of ADR-0053 (a tag is a pseudonymous identifier; UK law is an open risk) is unchanged and stays with the owner.
- The embedded table is fixed until the generator is run again and the file committed.
- Storage grows by a short text per device row: a few kilobytes a day at most, inside the 20 MB cap.

## Alternatives considered
- **Keep the 3-byte prefix itself** — also a coarse attribute, but a raw part of the address; the name carries the same information for the owner without keeping any bytes of it.
- **Store the broadcast network name** — the most useful label for an access point, but it is often a person's name ("Smiths Home"); ADR-0053 keeps others' names out of the disk.
- **Say nothing about makers** — leaves the page unreadable, the reason for this change.
