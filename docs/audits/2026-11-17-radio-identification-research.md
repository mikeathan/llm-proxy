---
subsystem: network-audit
tags: [wifi, radio, identification, ownership, fingerprinting, wips, ui, research]
result: reference
status: complete
date: 2026-11-17
---

# Radio: telling my devices from the neighbours', and what the page should show (research, 2026-11-17)

The owner's verdict on the Radio page (2026-11-17, verbatim): *"the new work around radio scanning and monitoring is useless … i see SKY UK 2a6e3b0d,
that could be my router or neighbors there is no way to determine that … my devices has some bars with colours … what does that mean … are we doing any
training to learn and assign labels/categories?"* They asked for research into similar monitoring tools and fingerprinting papers, and for better
positioning, tracking, filtering, isolation and per-device views.

This builds on [`2026-11-13-wifi-sensing-research.md`](2026-11-13-wifi-sensing-research.md) and does not repeat it. **Evidence labels:** *code* means
read in this repo at `f4ec7e53`. *vertex* means one read-only GET of `radio/air/devices`, `radio/networks` and `radio/devices/mine` on 2026-11-17, with
network names and labels removed before reading. *source* means a web search summary, not a full paper read: check the paper before relying on a number.
*assumption* is flagged. Nothing was built.

## 1. The owner's questions, answered

**"SKY UK 2a6e3b0d: mine or the neighbour's?"** The row has two parts. "SKY UK" is the company the address block was registered to (ADR-0057, the
embedded IEEE table). `2a6e3b0d` is the first 8 hex digits of the keyed one-way tag (ADR-0053), which nobody can read back. *vertex:* this device is
a **Wi-Fi 6 access point** (`wifi_generation: 6`) with a fixed address, on channel 6, at medium signal. It was first seen today and heard in **11 of
353** listened cycles. It joined none of your networks, and it is not one of your 4 confirmed access points. The rules call it `transient`
(`brief_appearance`). Most likely it is a neighbour's Sky hub, or a Sky Q box at the edge of range: Sky Q boxes act as access points of their own
(source: Sky forum threads). The 2026-11-13 capture also saw a neighbour's SKY router (`docs/learnings/network-audit.md`). **The page knew all of this
and printed none of it.** That is the first fault.

**"The coloured bars on My devices: is green good?"** No. In `frontend/src/components/radio/RadioPresenceStrip.vue` one square is one hour:
- **filled green** (`bg-state-success`): the device was *heard* that hour;
- **outlined**: the sensor was listening and did not hear it;
- **pale**: the sensor was not listening, so nothing is known.

The bars underneath are the device's estimated distance range for each hour, on a log scale, with closer at the top. None of this has a legend. Green
is the app's "healthy" colour, so it reads as signal quality. **Design fault:** use a neutral fill, add a three-item legend and label the distance strip's
axis.

**"Are we doing any training?"** **No.** *code:* `stacks/radio/classes.go` is a hand-written rule set (version 2, marked provisional). Its thresholds are
guesses: always here means heard in 80% of cycles; steady means a spread of 5 dB or less; strong is -60 dBm and weak is -75 dBm. "This is mine" sets
an owner flag and a label on one row (`owner_said`). It never tunes a threshold and never carries over to similar devices. The plan says the numbers
should be fitted to a week of data and the owner's marks. That has not been done.

## 2. Why the page feels useless: measured faults

1. **"On your network" never fires.** *vertex:* 4 access points are confirmed as yours, yet **0 of 56 devices** have `joined_own` after 353 cycles.
   *code:* `recorder.go:97` marks a device as yours only when an *association request* to your access point is heard. That frame is sent once, when a
   device connects, and almost never inside a 20-second window. Every data frame carries the BSSID it belongs to (`frame.go:299`, `dataBSSID`), but
   the recorder throws it away. This one fault explains most of "there is no way to tell mine".
2. **No join with the Network stack.** ADR-0052 says the core "joins a radio device to Network's inventory by address". *code:* nothing does this. The
   Network stack already knows every device on your LAN by address, with its name and your labels. Your own phone on your own WPA2 network uses the
   same private address over the air as on the LAN (see §3.4), so a "random address" device of yours *can* be named.
3. **One physical router shows as several dots.** A mesh node or hub sends 2 to 4 BSSIDs: 2.4 GHz, 5 GHz, guest, backhaul. *vertex:* 15 access points
   among 56 devices, with 3 ASUS for your own mesh. Nothing groups the BSSIDs of one box.
4. **Rows show attributes, not conclusions.** A maker, a tag, a class colour and a distance ring are given without the evidence behind them or the
   "so what". 32 of 56 rows have no maker (26 random addresses, 6 not in the registry), so those rows show only a tag.
5. **No learning loop** (§1), and the class sorting is provisional with **17 of 56 still `unknown`**.
6. **One sensor in one place gives a distance band, not a position.** The radar cannot be made truthful about *where* without a second sensor in
   another place (§4.5).

## 3. What the research adds (beyond 2026-11-13)

### 3.1 How professional tools decide "mine vs neighbour": wired-side correlation
Enterprise wireless intrusion systems (WIPS) sort every access point they hear into *authorised*, *interfering* (a neighbour's, heard but not on your
wired network) or *rogue* (on your wired network without approval). The test is **wired-side correlation**: compare the BSSID with MAC addresses
learnt from the wired side. Aruba uses an exact match ("Equal") and the last bit one less ("Minus One"). Extreme AirDefense derives a BSSID range from
each wired AP's MAC. Ruckus warns that such offset matches give false positives ([Aruba](https://arubanetworking.hpe.com/techdocs/aos/wifi-design-deploy/security/wids-wips/classification/),
[Extreme AirDefense](https://documentation.extremenetworks.com/Essentials/airdefense/UserGuide/GUID-6F2B5C8A-6E47-49C8-B5FE-D00470447D2F.shtml),
[Ruckus](https://support.ruckuswireless.com/answers/000001261)). An unknown BSSID on its own is weak evidence: combine the maker, the signal and the
wired side ([Purple](https://www.purple.ai/en-us/blogs/rogue-access-point-detection)). Classification only moves up, never silently down (Aruba).
**For us:** the Network stack is our "wired side". The router's LAN MAC and every LAN device's MAC can be matched against BSSIDs and client
addresses heard over the air. This is the strongest ownership signal we can have.

### 3.2 Client-to-access-point graph (Kismet, airodump-ng)
Kismet keeps, for each access point, the list of clients associated with it (`clients-of`). It calls two devices *related* when traffic shows they share
one physical network, are several BSSIDs of one physical AP, or are BSSIDs of one roaming SSID ([Kismet phy80211 API](https://kismetwireless.net/docs/api/wifi_dot11)).
Tools built on it store an AP→client "Connected" table ([wifi_db](https://awesome.ecosyste.ms/projects/github.com%2Fr4ulcl%2Fwifi_db)). **For us:** this
graph is what isolates devices. A client that exchanges data with your BSSID is on your network. A client on another BSSID belongs with that network,
which is probably a neighbour's. A client with no network only probes. Grouping by network turns 56 dots into a handful of households.

### 3.3 Passive model identification from management frames
- **Client taxonomy** (Google; a hostapd patch): the information elements of a device's probe and association requests form a signature that "in
  many cases" names the device model (e.g. a Nexus 5X). It tells a vendor-A phone from a vendor-B phone, not one person from another
  ([Google Research](https://research.google/pubs/passive-taxonomy-of-wifi-clients-using-mlme-frame-contents/), [arXiv 1608.01725](https://arxiv.org/abs/1608.01725v1),
  [hostap patch](https://lists.infradead.org/pipermail/hostap/2016-August/036096.html)). Not confirmed to be in current upstream hostapd.
- **WPS element in beacons:** many routers announce Manufacturer, Model Name and Model Number in a vendor element, and these can be read passively
  (the [Wi-Fi Alliance WPS fingerprinting post](https://blog-on-wifi.blogspot.com/2010/10/wap-fingerprinting-wi-fi-alliance-way.html), and the
  [Microsoft field map](https://learn.microsoft.com/en-us/windows-hardware/design/device-experiences/wireless-projection-implementing-additional-details),
  where Model Number can hold firmware). The same element's **Device Name** can carry a person's name (2026-11-13 capture: a WPS name "Streaming Stick"),
  so only the manufacturer and model are fit to show. *code:* `frame.go readIE` does not parse it.
- **Co-located BSSIDs:** Wi-Fi 6 beacons can list the other BSSIDs of the same box in the Multiple BSSID and Reduced Neighbor Report elements
  ([IEEE 802.11 TGax CID resolution](https://bibliotheek.ehb.be:3499/802.11/dcn/20/11-20-0818-03-00ax-resolution-for-cid-24114.docx)). This is direct
  evidence of "same physical box", stronger than guessing from address adjacency. *code:* not parsed.

### 3.4 MAC randomisation as it works today
iOS 14+ uses a private address per network. Since iOS 18 each network is set to Off, Fixed or Rotating ([Apple security guide](https://support.apple.com/guide/security/secb9cb3140c)).
A vendor blog says Fixed is the default on WPA2 and above (not confirmed by Apple's text). Android 10 to 15 keeps one random address per saved network,
until the network is forgotten or network settings are reset ([IETF MADINAS OS practices](https://github.com/ietf-wg-madinas/draft-ietf-madinas-mac-address-randomization/blob/main/OS-current-practices.md),
[IronWiFi](https://www.ironwifi.com/blogs/mac-randomization-wifi/)). **Consequence:** while a phone is *connected* to your network, its address is stable
and is the one your router's ARP table holds. It changes in its probe requests and on other networks. So your devices can be joined to the Network
inventory by address. Neighbours' phones stay anonymous, which is also the privacy protection.

### 3.5 Grouping random addresses (newer papers)
Since 2024 the state of the art combines the probe-request IE set, the timing pattern of probe bursts, and signal strength from several sniffers in
two-stage clustering ([arXiv 2408.01578](https://arxiv.org/pdf/2408.01578)). There are compact fingerprints at under 1% of the memory
([arXiv 2412.10548](https://web3.arxiv.org/pdf/2412.10548)), and management-frame state machines are claimed to reach 97% ([StateFi, arXiv 2507.02478](https://arxiv.org/pdf/2507.02478v2)).
Sequence numbers no longer increment predictably across bursts on recent phones; this was reported by arXiv 2606.25788, **whose date I could not
verify**. Self-supervised use of IEs, sequence numbers and signal strength together comes from Cappuccino ([TMC 2023](https://home.cse.ust.hk/~gchan/papers/TMC23_cappuccino.pdf)).
**For us:** useful for "these 3 brief random devices are probably one phone", which cuts the count of fake passers-by. It is a later step and needs a
stored fingerprint (an ADR).

### 3.6 Presence and movement from signal strength
The best direct evidence for a passive receiver is the **standard deviation of RSSI over about 20 s**, against a per-room threshold. Several receivers
voting help ([Jukić et al., arXiv 2308.06773](https://arxiv.org/pdf/2308.06773)). Kalman, median and Savitzky-Golay smoothing have been compared
for passive work ([Deak, Curran, Condell 2010](https://pure.ulster.ac.uk/en/publications/filters-for-rssi-based-measurements-in-a-device-free-passive-loca-3/)),
with no clear winner in the abstract. Hampel outlier removal is used too (a slide deck, preliminary). **For us:** use a robust per-cycle median with
outliers dropped, then the spread for still or moving. Report *zones*, not metres.

### 3.7 Learning with few labels
Cisco-style patents label a *cluster*: they ask an expert about the clusters the classifier is least sure of, then carry one answer to every member
([US 11528231](https://image-ppubs.uspto.gov/dirsearch-public/print/downloadPdf/11528231)). AutoIoT updates a semi-supervised identifier as new device
types appear ([AutoIoT, IEEE TMC 2023](https://aminer.org/pub/64f25ed73fda6d7f066063f1/italic-autoiot-italic-automatically-updated-iot-device-identification-with-semi-supervised-learning)).
Active selection plus label propagation is the standard few-label recipe ([AS3L](https://arxiv.org/html/2203.04560v3), [SEI via active learning](https://arxiv.org/html/2601.04502v1)).
**For us:** the owner is the expert. Ask about a few devices at a time, the least certain first, and carry one answer to devices that look the same.

### 3.8 Bluetooth as a second radio
Home Assistant room presence (ESPresense, Bermuda) follows Apple devices through address rotation by fingerprinting BLE Continuity messages rather
than addresses. It uses two or more cheap ESP32 nodes in different rooms to get the room ([HA guide](https://home-assistant-guide.com/2021/09/09/is-espresense-the-successor-to-esp32-mqtt-room-weve-been-waiting-for/),
[Seeed wiki](https://wiki.seeedstudio.com/xiao-esp32c3-espresense/), [Bermuda](https://community.home-assistant.io/t/bermuda-bluetooth-ble-room-presence-and-tracking-custom-integration/625780?page=38)).
*assumption:* vertex's built-in card has a Bluetooth half (`hci0`, from the 2026-11-13 learnings). ADR-0052 already reserves a Bluetooth driver. A
second *place* matters more than a second radio type.

## 4. What we can do, ranked

Constraint tags: **[now]** fits accepted ADRs and Constitution I.16 as written. **[ADR]** needs an owner decision and an ADR or I.16 amendment.
**[port]** needs a new core-to-stack port (an SDK change, so ask first).

### 4.1 Ownership evidence: "is this mine?" (highest value)
1. **Mark from data frames [now].** When a data frame (or, see the note, a control frame with both addresses) passes between a device and one of your
   confirmed access points, set `joined_own`. Check both directions: to the AP and from the AP, where the client is the receiver. The column exists.
   Also make the rules read `own_aps`, so that a confirmed mesh node that drops out is never sorted as a neighbour's (the v0.95.0 open item).
   *Note:* on 5 GHz this adapter decodes no HE data frames, but RTS and Block Ack frames carry both addresses (2026-11-13: 85k each in 240 s).
   `frame.go:284` already keeps both addresses of every control frame except CTS and ACK, so RTS and Block Ack pairs can be used now.
2. **Join the Network inventory without moving an address [port, approved in principle by ADR-0052].** The core tags each LAN MAC with the same vault
   subkey and hands the stack a set of tags with inventory ids. The stack matches tags it already computes. No address crosses the port and nothing new
   is stored, because the inventory's name and your label are shown at read time. Result: "Kitchen iPad (from Network)" instead of `0733101c`.
3. **Router adjacency, like a WIPS [port].** The core also tags the gateway's LAN MAC ±8 and its locally-administered variants. A BSSID that matches
   is "your router's radio" (Aruba "Equal / Minus One"). Show it as *likely*, because offsets give false positives.
4. **One box, several BSSIDs [now in memory; ADR to store].** Parse the Multiple BSSID and RNR elements in beacons, and fall back to address
   adjacency inside the recorder (the only code that sees addresses). Draw one access point with "4 networks". Storing a group key needs an ADR.
5. **Network name match [now].** A BSSID whose name equals one of yours is already *proposed* and shown in memory. Say on the row in plain words:
   "broadcasts a network that is not one of yours" or "broadcasts your network name: confirm?".
6. **WPS manufacturer and model for access points [ADR].** Parse at frame time and show in memory only, as with the network name in ADR-0057. Never
   the WPS Device Name.

### 4.2 Rows that say what a device probably is, and why
Replace "maker + tag + class colour" with **one conclusion sentence, one confidence word, and the evidence list**. For the SKY row, from vertex's own numbers:
> **Probably a neighbour's Sky router** (likely). Wi-Fi 6 access point · broadcasts a network that is not yours · none of your devices use it ·
> first heard today, in 11 of 353 listens · medium signal on channel 6.
> Actions: *This is mine* · *Neighbour's* · *Don't know*

The conclusion comes from code-written rules over typed facts, never from frame strings. Device-chosen text stays fenced (I.8). Use an icon only when
the evidence names a kind (an access point, a smart-home module). Otherwise show a plain dot, not a random symbol.

### 4.3 Filtering and isolation
- **A Networks view (topology):** each access point (grouped per box), with its clients beneath it. *Your networks* first, then each other network as
  "Neighbour network A, B, …" (in memory, per run), then "not connected (only probing)". This one view answers "what is mine" at a glance. **[now]** in
  memory; storing the client→network link for neighbours is **[ADR]**.
- **Filters:** On my network · Likely mine (evidence, not confirmed) · Confirmed mine · Neighbours' networks · Not connected · Access points only ·
  Fixed vs random address · New today · Hide brief (under N listens) · Nearer than a zone. All are derived from stored or in-memory facts. **[now]**
- **De-noising [ADR to store]:** group brief random-address devices that share a probe-IE set and burst timing into "probably one phone" (§3.5).
  At first this would be in memory only and shown as a count.

### 4.4 Learning: real training, explainable
1. **Evidence ledger per device [now].** Weighted facts are kept with their source. The class and the "mine" verdict come from them, and the row shows them.
2. **"Ask me" queue [now].** "We think these 3 are yours. Confirm?" Pick the least certain devices first (active learning, §3.7), at most 5 at a time,
   on My devices.
3. **Spread one answer [now].** After a mark, offer "2 others look the same (same maker, same network, always here): mark them too?". This is cluster
   labelling, and nothing is applied silently.
4. **Fit the thresholds [now, after enough marks].** Fit rules v3 to your marks (for example one threshold per feature, chosen for the best agreement,
   kept as a versioned rule document). Show agreement as "the rules match 18 of your 20 marks". Clear `provisional` only when the agreement is measured.
   Choose explainable rules over a black-box model. `class_log` already records every change.

### 4.5 Position and tracking: what is honest
- **With one sensor:** distance *zones* (in this room / in the house / at the edge / outside), never a bearing. Your fixed mesh nodes are free
  calibration anchors: you say how far each node is from vertex, and the path-loss model is fitted to your own walls, with no walk-around. Compute a
  per-cycle median with outliers dropped (Hampel), and get still or moving from the spread (§3.6). **[now]**
- **For real position:** a second sensor *in another place* (a small Wi-Fi or BLE node, §3.8) gives "nearer A or nearer B", and three give a rough
  room. This needs a remote-sensor design and probably a Constitution item. **[ADR]** The alternative, recording signal patterns per room while
  walking the house, is accurate but costs the owner time.
- **Tracking over time:** the presence strip from §1, fixed, and a day-level strip for neighbours (ADR-0053 allows day rows).

### 4.6 Visual fixes (small, do first)
Presence strip: neutral fill, a legend ("Heard · Listened, not heard · Not listening") and a labelled distance axis. The radar keeps a centre plus
range rings only. List-first on mobile, with the conclusion sentence as the row's main line.

## 5. Suggested order
1. **Data-frame ownership marks + rules read `own_aps` + presence-strip legend** [now]. Small, and it fixes the "0 of 56" fault.
2. **Conclusion rows with evidence + the Networks (topology) view + filters** [now, memory only].
3. **Inventory and router-adjacency join through a vault-tag port** [port]: owner approval of the SDK change.
4. **Ask-me queue, spread one answer, fit rules v3 to the marks** [now].
5. **Distance zones calibrated on your own mesh nodes, still/moving** [now].
6. Owner decisions: WPS model, RNR group storage, storing neighbour client→network links, a probe-IE fingerprint, a second sensor or Bluetooth [ADR each].

## Sources
The links are inline above. Most were read as search summaries, not as full papers. The vertex numbers come from one read-only GET on 2026-11-17, with
names removed.

## 6. Addendum (same day): what was built, and what nodeherder's wireless work added

**Built** on branch `radio-identify-classify-render` (v0.96.0, ADR-0058): every §4.1 to §4.6 item marked [now] or [port], plus the owner-approved [ADR] items (WPS make, model and type; the box; the neighbours' client-to-network link; the request signature). Not built: Bluetooth capture and a second sensor (§4.5).

**nodeherder** (`/Users/mikeathan/dev/nodeherder`, branch `feat/wireless-observations`, specs 064 and 065, read 2026-11-17; BLE advertisements from ESPHome Bluetooth proxies, classified from daily summaries). Borrowed, each with a test:
- **Per-day evidence with recency weighting** (half-life 2 days): `recencyWeight`, `weightSQL` in `stacks/radio/features.go`.
- **Night presence** (01:00 to 05:00) as the strongest sign of always-on equipment: one bit a day, `daily.night`.
- **Movement** from distance-band crossings: here, cycles whose mean jumped 10 dB or more, as a share of heard cycles (`daily.crossings`, `jumped` in `live.go`); a per-day count grew with the cycle rate and read an access point's 8 dB wobble as movement, so the threshold and the share were changed before shipping.
- **A signal reference** ("10 dB below the strongest device in the room"): here, signals shifted so the owner's own access points read -45 dBm (`signalOffset`, `learn.go`).
- **Provisional first-look classes** (`mobile-unidentifiable`, `likely-neighbour`): `firstLook` in `ident.go`, shown as "First look: ...".
- **Radar framing on the median, not the farthest** (×1.5, a tail past ×2 sits on the rim and is counted), **label collision avoidance**, and **"N hidden" wherever a filter hides rows** (passers-by hidden by default): `utils/radio/radar.ts`, `useRadioAround.ts`.
- **An evidence line in numbers the owner can check** on the device page.
Not borrowed, and why: its rule scores without learning (the owner asked for learning); its write floor (2 hours, 20 packets: would drop the passers-by the transient class needs); its "two evaluations" hysteresis note does not apply here because a classification runs once a cycle (5 minutes), not per snapshot. **Architecture to take next:** BLE from ESPHome Bluetooth proxies (remote ESP32 boards on the LAN) as remote sensors: it answers both Bluetooth and "a second sensor in another room" (§4.5) without a privileged helper action, but it opens connections to LAN devices, so it needs an ADR and a Constitution item before it is built.
