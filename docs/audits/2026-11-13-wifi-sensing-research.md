---
subsystem: network-audit
tags: [wifi, monitor-mode, sensing, fingerprinting, localisation, research]
result: reference
status: complete
date: 2026-11-13
---

# Wi-Fi monitor-mode sensing: what the literature says we can do with the signal

Evidence for the (not yet written) Wi-Fi radio sensor plan. Web search on 2026-11-13; **most items below were read as search
summaries, not as full papers** — check the paper before relying on a number. Our own measurements are in
`docs/learnings/network-audit.md` (2026-11-13 entry) and the owner-run capture scripts (removed from the tree, kept in git history; see `docs/PLANS/cross-cutting/radio-stack-handoff-2026-11-13.md`). Not instructions.

**Design rule these findings support:** a *sensor* is any monitor-mode adapter (one, two or more). The software must not assume a
count, a band or a chip. A sensor declares what it can decode (bands, HT/VHT/HE), and everything downstream works with whatever
sensors exist and says plainly what it could not see.

## 1. Telling devices apart when the MAC address is random

| Technique | Source | Finding | Fit for us |
|---|---|---|---|
| Information-element (IE) fingerprint of probe requests | [Vanhoef et al., ASIACCS 2016](https://hal.archives-ouvertes.fr/hal-01282900) | Randomising the address is not enough; the IE set and order in a probe request identifies the device model/driver | Free: we capture probe requests. Gives *model class*, not an individual |
| Sequence-number continuity | [PETS 2021](https://www.petsymposium.org/popets/2021/popets-2021-0042.pdf), [ASIACCS 2016](https://lirias.kuleuven.be/bitstream/123456789/543617/1//asiaccs2016.pdf) | Sequence number is not reset when the address changes, so N then N+1 under a new address links the two | Links address changes of one device. Partly defeated by per-burst random start numbers ([Hamburg](https://svs.informatik.uni-hamburg.de/publications/2022/2022-06-08_Probing_for_Passwords.pdf)) |
| Combine sequence number, IEs and RSSI from several sniffers, solved as minimum-cost flow | Espresso, [Infocom 2021 (HKUST)](https://cse.hkust.edu.hk/~gchan/papers/info21_vMac.pdf) | Linking randomised addresses to physical devices; the closest match to "group and re-group as we learn" | The shape to copy: weighted evidence, global assignment, not a hash |
| State-machine behaviour of a device's probing | [StateFi 2025](https://arxiv.org/html/2507.02478v2) | Up to 97% re-identification on public datasets (authors' claim, unreplicated) | Later, needs training data |
| Counter-evidence | [Compact fingerprinting](https://web3.arxiv.org/pdf/2412.10548) and a 2023 study | Similar devices overlap; individual identification stays limited | Expect *classes* and *likely same device*, never certainty |
| Why it also matters for privacy | [Martin et al., randomization study](https://arxiv.org/pdf/1703.02874) | Flaws in most deployed schemes; Android global MAC often recoverable | Our own phone changes MAC per SSID: we measured it |

**Take:** group by a score from several weak signals (IE fingerprint, sequence continuity, timing, SSID list, RSSI together
across sensors), keep a *claim* ("these 3 addresses are one device, 0.8") that later evidence can raise, lower or split.

## 2. Distance and position

- **Path-loss model** (log-distance) turns RSSI into distance; raw error is large (one ESP32 preprint: 47.7% raw, 7.1% after
  Kalman filtering, small room, BLE) — [PMC10301307](https://www.ncbi.nlm.nih.gov/pmc/articles/PMC10301307/) and
  [Preprints 202504.2304](https://www.preprints.org/manuscript/202504.2304). Different hardware and rooms; indicative only.
- **Fingerprinting** (map signal vectors to places) is the most used RSSI method because of multipath
  ([Bern thesis](https://boristheses.unibe.ch/893/8/16li_z.pdf)); **particle filters** beat EKF/trilateration there and handle
  multimodal answers. Cost: someone must walk the house to label places.
- **Passive multi-sniffer** localisation from probe requests: [Mishra et al., "Do WiFi probe requests reveal your trajectory"](https://miishra.github.io//papers/Do_WiFi_Probe-Requests_Reveal_Your_Trajectory.pdf);
  a TUM thesis reports about 1.4 m median with a floor-plan constraint ([TUM](https://mediatum.ub.tum.de/doc/1579221/document.pdf), figure not verified here).
- **Time-synchronised sensors** (NTP) matter when sensors' readings are fused ([ESP32 preprint](https://www.preprints.org/manuscript/202504.2304)); our adapters share one host clock and the radiotap `tsft`, which helps.
- **Ranging by round-trip time (FTM, 802.11mc)**: about 1 to 2 m in good conditions, better at 5/6 GHz and wide channels, needs a responder
  and multipath-aware calibration ([Bullmann 2020](https://pmc.ncbi.nlm.nih.gov/articles/PMC7472118), [arXiv 2303.03766](https://ar5iv.labs.arxiv.org/html/2303.03766)).
  Not available to a passive sniffer; mention only as a later add-on.

**Take:** with one sensor, report *distance bands and trend*, never a point. With N≥3 sensors with known positions, trilaterate or use
a particle filter; with a floor plan, constrain it. Always print the uncertainty.

## 3. Movement and presence

- **Stationary vs moving** from the spread of a device's RSSI: stable to about 2 dB over 20 s when static; moving changes all multipath
  components, nearby human activity changes a few ([Buffalo CoNEXT'14](https://cse.buffalo.edu/faculty/dimitrio/publications/conext14.pdf), [arXiv 1301.6644](https://arxiv.org/pdf/1301.6644); also patent literature using RSS deviation thresholds).
  Limit: a person walking next to a still device looks like movement; several sniffers and extra features reduce false alarms.
- **Device-free sensing** (people, not carried devices): CSI beats RSSI, but RSSI is available everywhere ([MDPI survey](https://mdpi.com/1424-8220/19/15/3329)); IEEE 802.11bf is standardising sensing ([overview](https://arxiv.org/pdf/2310.17661)). A monitor-mode adapter gives RSSI, not CSI, so this is a stretch goal.
- **Presence and counting from probe requests**: usable, degraded by randomisation and by detection falling with walking speed ([arXiv 1606.03713](https://www.arxiv.org/pdf/1606.03713), [SMARTCOMP 2020](https://conferences.computer.org/smartcomp/pdfs/SMARTCOMP2020-4Hjgt7Ifv9GdaywXmXXUSh/699700a106/699700a106.pdf)); privacy-preserving counting uses Bloom filters ([arXiv 2010.10370](https://ar5iv.labs.arxiv.org/html/2010.10370)).

## 4. Classifying what a device is, from encrypted traffic

- [Sivanathan et al.](https://forms1.ieee.org/rs/682-UPB-550/images/Classifying%20IoT%20Devices%20in%20Smart%20Environments%20Using%20Network%20Traffic%20Characteristics.pdf): rate, packet size, activity cycle and
  other flow features identify IoT device types (over 99% reported on their set); the unsupervised version ([LCN 2019](https://www2.ee.unsw.edu.au/~hhabibi/pubs/conf/19lcn-iot.pdf)) uses average packet size and rate at several time scales (over 94%, ten devices).
- Packet size and inter-arrival time of the first packets alone classify well ([arXiv 2009.07672](https://arxiv.org/pdf/2009.07672)); padding and shaping do not fully hide it ([arXiv 2110.11188](https://www.arxiv.org/pdf/2110.11188)).
- Over the air, with encryption on, the same side channel works: timing, sizes and RSSI reveal device presence, activity, routines and rough layout to a neighbour ([Noisy Networks, Nosy Neighbors](https://arxiv.org/pdf/2510.13822), [2026 follow-up](https://arxiv.org/pdf/2605.02553)).
  This is both our opportunity and the reason for the privacy rules below.

**Take:** per-device features we can compute from headers alone: frame-size histogram, rate, duty cycle, up/down ratio, retry rate,
periodicity, probe behaviour, RSSI spread.

## 5. Learning, reclassifying, adjusting

- Open-set / incremental: BIRCH-style incremental clustering supports cheap updates (0.13 s) and flags a held-out novel device, at some cost in
  known-device accuracy ([Alderman and Hastings 2026](https://arxiv.org/pdf/2602.24047)); IoTTFID adds device types over stages (80.4% after five, supervised) ([IEEE Access 2023](https://doaj.org/article/cdf3d41dd07e4670af0817858eaeb787)).
  The literature has the pieces (novelty detection, incremental update, drift) but no single recipe.
- **Pattern for us:** every group is a stored *hypothesis with evidence and a confidence*; new evidence adds, merges or splits it; an owner label
  ("this is the Zigbee hub") is the strongest evidence and survives re-clustering. Nothing is reclassified silently: the change is
  recorded and shown.

## 6. Threat and posture detection (management frames)

- Rogue access points, evil twins and deauth floods: counts per source, SSID/BSSID correlation, RSSI anomalies, vendor-prefix checks
  ([taxonomy](https://sindex.sdl.edu.sa/esploro/outputs/journalArticle/Rogue-Access-Point-Detection-Taxonomy-Challenges/9933880308331), [wids.py](https://en.kali.tools/?p=83)).
- **Clock-skew fingerprint of an access point** from beacon timestamps (TSF): stable per AP, differs between APs, 50 to 100 beacons suffice; defeats MAC spoofing ([Jana and Kasera, IEEE TMC 2010](https://doi.org/10.1145/1409944.1409958), [Dartmouth](https://cs.dartmouth.edu/dfk/research/arackaparambil-clock-skew/arackaparambil-clock-skew.pdf)). A strong, cheap way to tell a cloned BSSID from the real one.
- **Prior art for the capture and device-tracking layer:** [Kismet](https://kismetwireless.net/docs/dev/datatypes/) models every adapter as a *datasource*, builds device records from decoded frames, keeps per-device
  signal and a location history, and can keep per-datasource signal for multi-sensor comparison. Its shape (generic datasources feeding one device tracker) is the pattern for "any number of adapters".

## 7. Open question the search did not settle: UK law

Capturing frames not addressed to you in the UK touches [Wireless Telegraphy Act 2006 s.48](https://www.legislation.gov.uk/ukpga/2006/36/section/48/data.htm)
(obtaining information about a message's contents, sender or addressee without lawful authority) and, through its carve-out, the
[Investigatory Powers Act 2016](https://www.legislation.gov.uk/ukpga/2016/25/section/259/data.htm). The search found **no** guidance on passive
monitor-mode capture of neighbours' frames and I am not a lawyer. Before the Constitution amendment: record the owner's decision, keep neighbours' data
to the minimum (counts, salted hashes, short retention, no payload, headers only), and consider specialist advice.

## What this means for the plan (not yet a plan)

1. **Sensor-generic core:** capability record per adapter (bands, HT/VHT/HE decode, channel list, driver quirks, verified-channel read-back); N sensors in,
   one device tracker out; every output carries which sensors saw it and what was invisible.
2. **Evidence-based device tracker** with hypotheses (identity groups, class, position) and owner labels, per section 1 and 5.
3. **Position as an estimate with uncertainty**, graded by sensor count and calibration (section 2).
4. **Management-frame detections first** (they need no Wi-Fi 6 decode): rogue/twin, deauth, posture, association events, probe behaviour, clock skew.
5. **Privacy by design** (section 4 and 7): owner's devices stored; others counted, hashed, expired.

## 8. Privacy position and where this lives (owner discussion 2026-11-13, not yet decided)

**Neighbours' devices.** A persistent identifier that lets us recognise a device again is personal data even when hashed: the ICO says a MAC
address is personal data when it is used to track a device in order to single out or treat an individual differently, and that pseudonymisation does
not change the status ([ICO on indirect identification](https://ico.org.uk/for-organisations/uk-gdpr-guidance-and-resources/personal-information-what-is-it/what-is-personal-data/can-we-identify-an-individual-indirectly/),
[2016 ICO Wi-Fi location analytics guidance](https://www.twobirds.com/en/insights/2016/uk/ico-publishes-new-guidance-on-wifi-location-analytics); the ICO text is under review).
So the aim is **minimise and expire, not "no personal data"**: keep the owner's devices; for others keep a *keyed* hash (HMAC, key in the
secrets store; a plain hash of a 48-bit address is a dictionary lookup), a device class, signal band and counts; no probe-request network names, no
device-chosen names (a WPS "Device Name" can carry a person's name), no raw addresses, per-day aggregates not tracks, expiry (default proposed 30 days
since last seen), a "forget neighbours" action, nothing raw to the model. Whether the household exemption covers this is **unresolved** (not found by search).

**Placement.** Findings so far: a stack is its own Go module with a manifest (layers, modules, tools, routes, storage, requires, observes), plain data
across ports, no import of `internal/` or of another stack (Constitution IV.6, ADR-0032); the Network stack owns sensors and the core keeps the
judgement (ADR-0038); the privileged helper is core and a stack only passes back a grant (ADR-0034, `PortPrivileged`); Network and Agents are the two stacks
and the stacks overview page waits for a third (ADR-0028). Options: a **Radio layer inside Network** (reuses Devices, Findings, Timeline, alerts, the
agent tool) or a **new Radio stack** (own storage and retention for neighbour data, room for BLE, 802.15.4 and SDR later; joins the Network inventory by address through the core).
