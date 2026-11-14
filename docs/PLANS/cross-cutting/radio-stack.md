---
status: in-progress
date: 2026-11-13
last_reviewed: 2026-11-13
related_specs: [SPEC-006]
constitution_references: [I.5, I.8, I.11, II.3, III.6, IV.6]
evidence: Owner-run measurement on vertex 2026-11-13 (owner-run capture scripts, since removed from the tree and kept in git history, see the handoff; ledger entry in docs/learnings/network-audit.md); literature digest docs/audits/2026-11-13-wifi-sensing-research.md; stacks/sdk/manifest.go and ports.go, ADR-0028, ADR-0032, ADR-0034, ADR-0036, ADR-0038, ADR-0049 and ADR-0051 read 2026-11-13; backend/internal/platform/{db,secretcrypto,sizewatch} read 2026-11-13. Nothing built.
---

# The Radio stack: passive wireless sensing with any number of adapters

Owner direction (2026-11-13), in order: a **new Radio stack** (not a layer of Network); retention **14 days for neighbours' devices and 30 days for everything
else**, with small, bounded storage that cannot fill the disk; **SQLite is fine** (`modernc.org/sqlite`, already in the repo); what is stored for other people is
hashed or encrypted with the least information, and can still be matched to a later reading (after a restart, or when a device leaves and returns); four classes,
adjustable as the system learns; **generic for any adapter, one or many, not hard-coded to two**; extensible, pluggable, switchable as a stack; **does nothing when no
adapter is connected**; a connected radio's capabilities are discovered and **those we do not use are listed in the UI for later**; and a page for managing all
stored data across the service ([`data-management-page.md`](data-management-page.md)).
Decisions: [ADR-0052](../../decisions/ADR-0052-radio-is-its-own-stack-with-generic-sensors-and-the-core-keeps-capture-and-judgement.md),
[ADR-0053](../../decisions/ADR-0053-radio-keeps-a-keyed-tag-for-other-peoples-devices-and-seals-the-owners-with-hard-size-and-age-limits.md).

## 1. What we measured (so the design rests on evidence)

A USB 802.11ac adapter (Realtek, in-kernel `rtw88_8822bu`) and a 2.4 GHz-only MT7601U both enter monitor mode on vertex's kernel 7.0 with `iw`. Passive capture gave
management frames, device joins and leaves with reason codes, access-point beacons with their security fields, probe requests, per-frame signal per antenna and
time. It did **not** decode Wi-Fi 6 data from the router (0 HE frames of 308,712 on 5 GHz), so per-device volume there is an undercount while management and
control frames are complete. A phone uses a different random address per network name. Full numbers and tool traps: the ledger entry. Not measured: distance accuracy,
Zigbee (a Wi-Fi radio cannot hear it), a Wi-Fi 6 adapter, long runs.

## 2. Shape

`stacks/radio/` is its own Go module (Constitution IV.6): standard library, `modernc.org/sqlite` (third-party, allowed; **ask-first item, see §10**) and `llm-proxy-stacks/sdk`.
Plain data across every port. Layers, bottom first (manifest `Layers`):

| Layer | Holds | Plug-in point (an interface inside the stack, plain data in and out) |
|---|---|---|
| `sensors` | discovery of radios, their capabilities, their health | `SensorDriver`: `Kind()`, `Discover()`, `Capabilities()`, `Open(plan)`. First driver: Wi-Fi monitor. Later: Bluetooth LE (vertex already has `hci0`), 802.15.4, SDR |
| `observations` | frames to typed facts (a join, a leave, a beacon, a probe, a signal sample) | `Analyzer`: one per concern (association events, beacon posture, deauth rate, probe fingerprint, access-point clock skew, airtime) |
| `tracker` | who is who: identity claims, class, presence, movement, position | `Classifier` (rules as data, §4) and `Positioner` (method by sensor count, §5) |
| `insight` | findings by id, the digest, trends | ADR-0036 (ids, the core words them) and the digest tool (§7) |

Plug-ins are compile-time interfaces with runtime switches, not downloadable code. Modules (ADR-0028, one switch each, with `needs`): `radio_sensors` (the stack's
master), `radio_presence`, `radio_neighbourhood`, `radio_posture`, `radio_airtime`, `radio_position`. Off means no goroutine, no capture, no write.

**Idle when empty.** With no adapter the stack runs a cheap discovery read of `/sys/class/net` at a low rate and on a Rescan press, and nothing else. The service has no
netlink (the unit refuses it), so detail (bands, widths, standards, modes) comes from one privileged one-shot probe action, parsed in the unprivileged stack
(Constitution I.11: untrusted text is parsed outside the privileged process). **macOS:** no capture, so the stack is idle and says so.

**Capabilities and the "not used yet" list.** Each sensor has a plain `Capabilities` record: bands and channels, widths, Wi-Fi generations it can decode (HT, VHT, HE, 6 GHz),
streams, supported modes (monitor, AP, injection), ranging support, bus, driver, regulatory domain. Code maps each capability to a use. The Sensors page shows every one as
**used**, **unused by design** (injection is refused by policy), **unused, not yet built** (HE decode, ranging, a 5 GHz channel the survey did not include) or **found, no driver** (a
Bluetooth or 802.15.4 radio). That list is the future-work list the owner asked for, and it needs no new code per capability.

## 3. What is stored (ADR-0053)

One file, `radio.db` (SQLite, WAL), declared in the manifest's `Storage`. No frame is ever written; frames are parsed in memory and dropped.

| Data | Form | Kept |
|---|---|---|
| Owner's devices | address **sealed** (AES-GCM by the core's key, III.6), label, class | until the owner deletes |
| Owner's devices, history | hourly rollups (time present, signal min, mean, max, joins and leaves) | 30 days |
| Owner's devices, daily rollups | one row a day | 30 days (a setting, upper bound 365) |
| Other people's devices | a **keyed tag** (HMAC from the core's key, never the address), class, signal band, visit count, first and last day | **14 days** since last seen |
| Beacons (access points) | tag, name length and a hash of the name, security fields, channel | 14 days; the owner's own access points are named |
| Events (joins, leaves, deauth bursts) | typed, per owner's device; counts only for others | 30 days and 14 days |

Never stored for others: raw addresses, probe-request network names, device-chosen names (a WPS device name can carry a person's name), per-frame traces, anything below a
day's resolution. A hard cap (default 20 MB, `PRAGMA max_page_count` plus our eviction before it is reached; lowest-value rows go first: oldest, fewest visits) means it cannot fill the disk.
A daily job prunes and runs an incremental vacuum. **Forget neighbours** rotates the tag key: every stored tag stops matching at once. A tag is a pseudonymous identifier
and the ICO treats such identifiers as personal data; the design is *minimise and expire*, and the UK-law question is recorded as open in ADR-0053, not assumed away.

**Matching without decrypting.** The tag is deterministic for a key, so a returning device finds its old row after a restart or a re-entry. The owner's sealed addresses are only
opened to show the owner's own list. The stack reaches this through one new SDK port, **`vault`**: `Tag(purpose, bytes)`, `Seal(bytes)`, `Open(bytes)`. The core derives a
subkey per purpose from the master key; the stack never sees a key.

## 4. Classes (the owner's table; rules are data and adjustable)

| Class | Signature | Means |
|---|---|---|
| Infrastructure | present 24/7, low signal variance, static address | TV, bulbs, plugs: ignore |
| Personal | intermittent, high variance, comes and goes | phones, watches, tags |
| Neighbour | consistently weak, never strong | outside your four walls |
| Transient | brief appearances | passers-by, cars |
| Unknown *(added here)* | too little evidence | new devices, shown with what is missing |

A class is a rule over **features**: share of time present, session count and length, signal mean, spread and maximum, hours-of-day pattern, days seen, band, whether it joins one of the owner's networks (the strongest
single piece of evidence), and the model class from its probe fingerprint. Rules and thresholds live in the database as versioned data (`classifier_v<N>`), **not in code**, and **the numbers come from vertex's measured captures, not from this plan.**
Adjusting: (1) an owner's label is ground truth and nudges the thresholds toward it; (2) the system proposes a new group ("12 devices, strong, never join") for the owner to accept; (3) every change is a new
version, logged, shown in the Timeline with its reason and confidence, and **nothing is reclassified silently**. Identity claims between changing addresses use weighted evidence (IE fingerprint, sequence-number continuity,
timing, signal across sensors; the shape of Espresso in the research digest) and are stored as claims with a score that later evidence can raise, lower or split.

## 5. Position, distance, movement (always with its uncertainty)

One sensor: a **distance band** and a trend from signal strength (log-distance model, filtered), never a point. Two: bands that agree or disagree. Three or more at positions the owner places: trilateration
or a particle filter, constrained by an optional floor plan. Movement: spread of signal over time against the device's own baseline, with several sensors to tell a person walking past from a device moving. Calibration
is by the owner standing a labelled device at a known place, and a known anchor such as vertex's own card (seen near -18 dBm). The method is chosen from the sensor count at run time, so one, two or ten adapters all work.

## 6. Capture, privilege and the proposed Constitution item (draft, not yet in the Constitution)

Monitor mode needs `CAP_NET_ADMIN` and nl80211, which the current helper does not hold (it has `CAP_NET_RAW` only). Proposed **I.16 Radio Sensor** (in force only when the owner merges the pull request that carries the
text): a helper action class `radio` in the closed catalogue (I.11), installed by the operator, off by default, three gates and step-up as I.11; actions `radio_probe` (read capabilities, one-shot) and `radio_listen`
(put one named USB adapter in monitor mode for a bounded window on listed channels, return truncated headers); the adapter must be USB and never the one the host is connected through; **sends nothing, no injection, no
deauth**; capture windows are duty-cycled (initial: 20 s a channel, a cycle every 5 min, both numbers settings with caps) and stop on a time or frame cap; the helper returns headers only (radiotap and the 802.11 header, up to 512 bytes for a beacon and 64 for
anything else), parsed in the unprivileged stack with strict bounds and fuzzing (anyone in range controls these bytes); after each window the helper returns the adapter to managed mode; the helper reads back the channel it set.
**Open design question:** duty-cycled windows (fits I.11 as written) against a long-lived sensor unit (needs a larger amendment). This plan assumes windows; R2 measures whether they are enough.

## 7. Findings, the digest and the agent

The stack offers **findings by id** and the core words them (ADR-0036): `radio_rogue_twin` (same name, different or cloned address), `radio_deauth_burst`, `radio_open_or_weak_ap`, `radio_pmf_off` (management-frame
protection off on the owner's network), `radio_hidden_ap_near`, `radio_unknown_strong_device`, `radio_join_foreign_network` (an owner device repeatedly trying another network), `radio_sensor_blind` (an adapter that cannot decode what the air carries).
For the agent: a **typed, bounded, read-only digest** (the precedent is `network_findings` and the audit window digest, ADR-0049): per owner's device presence, arrivals and departures, class changes, movement, anomaly counts, airtime shares, new
classes near the house, each claim with a **confidence tier set by code** (seen, inferred, estimated), the sensors that saw it and a "could not see" list; every device-chosen string inside the untrusted-data fence (I.8); other people's devices
only as counts by class. Two ways to use it: an automation template (nightly or weekly trends) and an **Analyse** button on the Radio page that runs the same tool and saves a report like the audit reports. The model words and interprets; code computes classes, positions and counts.

## 8. Screens (Radio, in the Stacks group beside Network and Agents; the stacks overview page ADR-0028 promised arrives with this third stack)

Air overview (sensors and what they cannot see, who is present, anomalies) · Devices (the owner's, with class, signal band, movement, label; joined to Network's device list by address through the core) · Map (distance rings, a floor plan with three or more sensors, uncertainty drawn) ·
Neighbourhood (counts by class, "unknown device close to the house") · Timeline and patterns (joins, leaves, presence by hour and day) · Airwaves (channel use, retries, airtime by sender, group-traffic cost, a guided channel fix) · Sensors (capabilities with the used / unused list) · Modules · Settings (retention, caps, classes). Simple and Advanced views, mobile first (owner rule).

## 9. Phases (each ends with a check that fails before and passes after)

- **R0 Decisions.** Owner accepts the ADRs and the I.16 text; owner says whether the open helper question (windows or long-lived) is settled. *Check:* ADRs `accepted`.
- **R1 Skeleton.** `stacks/radio` with manifest, `vault` and storage-report ports in the SDK, discovery from `/sys`, capability records, the Sensors page, idle behaviour. *Check:* with no adapter the stack starts no goroutine and writes nothing (test); with a fake adapter tree it lists capabilities, including an unused one; manifest and boundary tests pass.
- **R2 Capture.** `radio_probe` and `radio_listen`, the parser (fuzzed), the store with caps and retention. *Check:* a fuzz run clean; a replayed pcap from vertex gives the same typed facts as the owner's report; the size cap holds when the test floods rows; channel read-back test.
- **R3 Tracker.** Tags, sealed owner list, classes v1 from measured data, owner labels. *Check:* a device re-enters after a restart and matches its row; forget-neighbours makes every old tag stop matching; no raw address of another person in the file (test greps the DB).
- **R4 Findings and digest.** The finding ids, the digest tool, the template, the Analyse button. *Check:* digest has no device-chosen string outside the fence; byte-identical wording tests (ADR-0038 rule 1).
- **R5 Screens.** The pages in §8. *Check:* Vitest, `npm run build`, visual tests, viewed at 390 px.
- **R6 Position.** Distance bands, then multi-sensor. *Check:* measured on vertex with a walked device; the stated uncertainty contains the truth in the owner's test.
- **R7 Learning loop.** Proposed groups, versioned classifier, label-driven thresholds. *Check:* a label changes a class and the change is in the Timeline with its reason.
- **R8 Bluetooth driver (optional).** Proves the plug-in point with a second radio kind.

## 10. Risks and open items

Ask first (AGENTS.md): the `modernc.org/sqlite` dependency in the stack's `go.mod`; the Constitution I.16 item; any SDK port added. Windows against a long-lived sensor (R0). The Realtek heard 0 frames on one channel in one run, so channel read-back is mandatory. DFS: the router can leave channel 60. A Wi-Fi 6 adapter on 5 GHz is untested (R2 or a later measurement). UK law (ADR-0053). Adapter power and heat over hours (unmeasured). Linux only (macOS idle).

## Progress (2026-11-13)

- **R0 done.** The owner accepted ADR-0052 and ADR-0053 (now `accepted`), the SDK ports, the `modernc.org/sqlite` dependency, **duty-cycled windows** for capture, and the UK-law question as an **open risk** (no advice taken; capture must not default on before the owner takes it). The Constitution I.16 text (§6) is **not** in `CONSTITUTION.md` yet: it rides in the PR that builds R2 (`radio_probe`, `radio_listen`), the first code that needs a helper action.
- **R1 built and merged** (PR #245): `stacks/radio` (manifest, one module `radio_sensors`, one read-only route `GET /admin/api/radio/sensors`, discovery from `/sys` with capability records and the used / unused list); SDK `PortVault` + `Vault` interface and the optional `StorageReporter`; core wiring (`models.RadioConfig`, storage `Radio()` view, registry stack `radio`, hosted stack, off until switched on); the Radio page (Sensors, Modules) in the Stacks group. Idle by construction: **no goroutine ever** in R1 (discovery runs when the page asks), nothing written, a test checks both.
- **Not in R1, on purpose:** the vault *adapter* in the core (no stack uses it until R3; the port and its types exist and are tested as plain data); the storage-report consumer (the data page, D1); SQLite (R2, the first thing that stores).
- **Still to do for R1's check:** "with a fake adapter tree it lists capabilities, including an unused one" is covered by `stacks/radio` tests; the privileged probe (bands, widths, modes) is R2.
- **R2 started (2026-11-13, branch `radio-r2-capture`):** Constitution I.16 added (owner approved); the frame parser `stacks/radio/frame.go` is built test-first with fuzzing, real-frame fixtures and a replay test that matches tcpdump on the owner's captures. Also built (uncommitted at the time of writing): the core `Vault` adapter, SDK `FilePather`, the SQLite store (cap, eviction, 14/30-day retention, forget), the `Recorder` and the stack's `StorageReporter`; manifest now requires `datadir` and `vault` and lists `radio.db`. Not built: the owner-device UI, hourly rollups, the pipeline that feeds the Recorder (R3).
- **R2 helper actions built (2026-11-13, branch `ci-faster-backend-test-and-coverage`, with the CI test-speed fixes, PR #247; not run on a real host):** `radio_probe` (`iw list`, no parameters) and `radio_listen` (`{iface, channel, seconds}`, one channel a call, the stack loops over channels) in a new `radio` class: catalogue rows with `Before`/`After` steps (monitor mode, channel set and read back, then managed mode on every path, run by the helper's jailed `ip` and `iw`), `helper.Config.Capabilities` (a list: `CAP_NET_ADMIN` + `CAP_NET_RAW`) and `Config.Interface` (USB only, never the default route or an SSH session, read from `/sys` and `/proc`), the `radio-listen` program (`capture/radio.go`, `wire/radio.go`: 512 bytes of a beacon, 64 of anything else), a per-class socket in the client, step-up entries, `docs/services/llm-proxy-helper-radio*`, `install-helper.sh radio`. Bounded sessions (Constitution I.17, ADR-0054) let the scheduler run without a step-up per window. Not done: see the handoff.
- **R3 part 1 built (2026-11-13, branch `radio-r3-unit-pipeline-sessions`, v0.86.0; not run on vertex):** window reader and pipeline into the Recorder, the session-gated run, Radio page control, owner devices, forget-neighbours as tag-epoch rotation, vault key cache. Left for R3: the tracker and classes (thresholds from the owner's captures). See the handoff.

- **R3 part 1 merged (#251, v0.86.0) and listening on vertex** (the owner saw 9,571 frames heard in the first run). **R3 part 2 built (2026-11-14, branch `radio-r3-sensors-redo-probe-classes`, v0.87.0; not run on vertex):** the probe parsed and kept; a truthful Sensors page and a Radio page of four tabs (Listening with Around you, Sensors, My devices with Your networks, Modules); the database migration; day-level aggregates; `classifier_v1` (provisional, unmeasured); the class log; own networks proposed by name and confirmed by the owner. Still open in R3: thresholds fitted to measured data (needs a day of vertex's own listening, then R7's fitting), owner hourly rollups.

- **R3 part 2 merged (#252, v0.87.0); 2026-11-14 owner verdict on the Listening tab: the UI is temporary and lacks useful, openable data.** Next, in one branch: R5's useful core (readable windows, openable Around you, presence, hourly rollups), then R4. Order and detail: the handoff file.

## Remaining work

R2 onward as §9 (I.16 and the parser are done): the helper actions `radio_probe` / `radio_listen`, the parser with fuzzing, the SQLite store with caps and retention. R2 needs **a pcap from vertex** (capture with tcpdump on a monitor interface; keep it out of the repo, derive a small synthetic fixture).
