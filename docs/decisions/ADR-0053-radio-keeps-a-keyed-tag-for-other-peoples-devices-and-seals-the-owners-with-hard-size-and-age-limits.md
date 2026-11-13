---
id: ADR-0053
title: Radio keeps a keyed tag for other people's devices and seals the owner's, with hard size and age limits
status: proposed
date: 2026-11-13
subsystems: [network-audit]
supersedes:
superseded_by:
---

# ADR-0053: Radio keeps a keyed tag for other people's devices and seals the owner's, with hard size and age limits

Proposed: in force when the owner accepts it. Part of [ADR-0052](ADR-0052-radio-is-its-own-stack-with-generic-sensors-and-the-core-keeps-capture-and-judgement.md). Constitution III.2 (atomic persistence), III.6 (secrets are encrypted), IV.6.

## Context
A monitor-mode radio hears every device in range, including neighbours' and passers-by's. The owner wants to recognise them again (after a restart, or when one leaves and returns) without storing personal information and without filling
the disk. Facts: a plain hash of a 48-bit address is reversible by trying every address; a salt matters. The ICO treats a MAC address used to single a device out as personal data, and pseudonymisation does not change that
([ICO](https://ico.org.uk/for-organisations/uk-gdpr-guidance-and-resources/personal-information-what-is-it/what-is-personal-data/can-we-identify-an-individual-indirectly/); the page is under review). Whether the household
exemption covers this was **not found** by search. The service already has SQLite (`platform/db`) and AES-GCM sealing (`platform/secretcrypto`, key in `master.key`). Owner decisions 2026-11-13: neighbours 14 days, everything else 30 days, small bounded data, SQLite allowed.

## Decision
We will treat "no personal data" as unachievable and aim to **minimise and expire**:

- **Other people's devices: a keyed tag only.** HMAC-SHA-256 from a per-purpose subkey the core derives from the master key, exposed to the stack through a `vault` port (`Tag`, `Seal`, `Open`); the stack never holds a key. Stored with a class, a signal band, a
  visit count, first and last day. **Never** stored: the address, probe-request network names, device-chosen names, per-frame traces, anything finer than a day. The tag is stable for a key, so a returning device matches its row; "forget neighbours" rotates the key and every tag stops matching.
- **The owner's devices: sealed.** The address is encrypted (AES-GCM) so the owner's own list can be shown; labels are the owner's.
- **Age limits:** 14 days since last seen for other people's devices and their beacons; 30 days for the owner's hourly and daily rollups and events (the daily setting may be raised to a bounded maximum). A daily job prunes.
- **Size limit:** one SQLite file, default hard cap 20 MB (`PRAGMA max_page_count`, plus our own eviction of the lowest-value rows before it is reached), incremental vacuum; no frame is ever written to disk.
- **Nothing raw to the model:** the agent sees the typed digest; other people's devices as counts by class.
- **Open and recorded, not assumed away:** a tag is a pseudonymous identifier; the legal position of keeping it is for the owner to accept as a risk or to take advice on before the capture is enabled by default.

## Consequences
- Re-recognition works for devices with a fixed address and for classes; it mostly will not link a phone's rotating random address across days, which is also the protection.
- One more core port (`vault`) and one subkey scheme to keep stable: changing the derivation forgets every tag.
- A test must read the database file and find no address of another person.
- Storage is bounded by construction; the data-management page (plan `data-management-page`) shows and purges it.

## Alternatives considered
- **Plain or salted hash of the address** — reversible or linkable by anyone with the salt; keyed with a secret the stack never sees is stronger and no harder.
- **Store nothing about others** — no re-recognition, no neighbour class, no "unknown device close to the house".
- **JSON files like the inventory** — a whole-file rewrite for rows that change every window; no age-based delete; SQLite already ships.
- **A key-value store (bbolt, badger)** — a new dependency with no queries by time.

## Links
Plan [`radio-stack.md`](../PLANS/cross-cutting/radio-stack.md) §3 · research `docs/audits/2026-11-13-wifi-sensing-research.md` §7 and §8.
