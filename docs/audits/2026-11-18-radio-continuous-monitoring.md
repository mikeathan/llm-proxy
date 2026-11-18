---
subsystem: network-audit
tags: [wifi, radio, presence, monitoring, duty-cycle, bluetooth, research]
result: reference
status: complete
date: 2026-11-18
evidence: Read-only GETs of vertex's radio API on v0.97.0 (commit 85bc643d), 2026-11-18 08:00–08:10 UTC, one cycle after deploy (listening state, the 24-hour stored signal track of three owner devices, device pages); code read at stacks/radio/listen.go, radio.go, live.go, backend/internal/platform/privhelper/catalog/radio.go, wire/radio.go. Nothing was changed on vertex. Analysis only; nothing built.
---

# Radio: what continuous monitoring would take (analysis, 2026-11-18)

The owner asked: my phone moves around the house, but a scan every 5 minutes will not pick that up in real time. What do we need for
continuous monitoring? This is an investigation, not a design. The decisions it needs are listed at the end.

## 1. What vertex does today (measured)

The plan is the default: channels 1, 6, 11, 36, 60, 149, a 20 s window each, a cycle every 300 s (`listen.go` `Plan`). One cycle on vertex
at 08:00:40 UTC:

| Channel | Frames | Owner's frames | Stop | Listened |
|---|---|---|---|---|
| 1 | 507 | 0 | window | 20 s |
| 6 (owner's 2.4 GHz) | 3,326 | 730 | window | 20 s |
| 11 | 1,749 | 0 | window | 20 s |
| 36 | 684 | 0 | window | 20 s |
| 60 (owner's 5 GHz, where the phone is) | 4,000 | 1,330 | **frame cap** | **7 s** |
| 149 | 2 | 0 | window | 20 s |

- Windows start about 24 s apart (20 s of listening plus about 4 s of switching the adapter to monitor mode and back, `catalog/radio.go`
  `Before`/`After`). A cycle listens for about 110 s of its 300 s.
- **The phone's channel gets about 7 s in every 5 minutes (about 2%).** Channel 60 fills the helper's per-window cap
  (`wire.RadioMaxFrames` = 4,000) in 7 s. A longer window buys nothing there.
- The picture already changes window by window: `LiveSnapshot` (`live.go`) adds the open cycle's reading to each device (corrected the
  same day; an earlier version of this note said once a cycle). The delay is how often a channel is visited, once a cycle, not the picture.

**How often the owner's devices are heard** (24-hour stored track, ADR-0055, `GET /admin/api/radio/air/track/{id}`; one point per cycle the
device was heard):

| Device | Points | Heard in cycles (hours with points) | Longest gap | Pattern |
|---|---|---|---|---|
| `4ded1379` "s24 plus - mike" (owner's phone) | 53 | about 29% | 10 h (09–18 UTC, *assumption:* out of the house) | 2 of 12 cycles an hour overnight; 11 of 12 at 06 UTC |
| `a4c371c6` "Lindiwes-iPhone" | 131 | about 46% | 3 h | 4–11 an hour, all day |
| `bd105ebf` "M4 MacBook air" | 133 | about 46% | 2 h | 7–12 an hour while in use |

**The headline:** the heard rate follows whether the phone is **in use**, not whether it is **present**. A phone dozing on a table sends a few
frames a minute (power save), and a 7 s window on its channel misses most of them. At 06 UTC, *assumption:* the alarm and the morning, it
was heard in 11 cycles of 12. So "every 5 minutes" is not the only limit: a dozing phone is close to silent to any passive Wi-Fi listener,
whatever the duty cycle.

## 2. Why "just turn the settings up" does not work

The settings allow a cycle down to 60 s and windows up to 30 s (`Plan`). Three caps stop that from becoming continuous:

1. **The per-window frame cap (helper, 4,000 frames).** Channel 60 hits it in 7 s, so the busy channel, the one that matters, is capped by
   frames, not seconds. *Assumption, not checked:* much of channel 60 is the ASUS mesh's own 5 GHz backhaul between its two boxes
   (`a0be4927`, `7726d4bd`). Raising the cap or adding a kernel frame filter is a change to the helper: a security review under I.11/I.16.
2. **The run's frame budget (stack, `framesPerHour` = 170,000, `listen.go`, checked at `listen.go:421`).** It is a total for the watch
   (hours × 170,000, `radio.go` `frameCapFor`). Today's rate is about 10.3k frames a cycle × 12 = **123k an hour, 72% of the budget.**
   Listening about 40% more on busy channels would end a 7-day watch before its time. At the budget, channel 60 alone gets about 42 windows
   × 7 s, about **5 minutes of listening an hour**.
3. **The hourly window caps.** The helper allows 240 `radio_listen` an hour (`radioListenMaxPerHour`), the stack plans for at most 200
   (`MaxWindowsPerHour`). 30 s windows back to back on two channels would be 120 an hour, so this cap is not the binding one.

**The law.** Constitution I.16: "Windows, not a standing sensor. A capture is duty-cycled … A long-lived sensor needs an amendment of its
own". I.17 says the same of sessions ("Not a standing sensor"). Near-continuous listening on the owner's channels is still windows within the
caps, but true real-time capture (always listening) is an owner decision and a Constitution amendment, not a setting.

## 3. The options, against what "real time" means

"Real time" can mean three different things. Each has a different cheapest answer:

- **A. Home or away** (is my phone here?), within a minute or two.
- **B. Which room** it is in.
- **C. Moving or still** (it is being carried around).

| Option | Answers | What it needs | Notes |
|---|---|---|---|
| **1. Listen to the owner's channels more** (built as focus windows between full sweeps, §7: every channel still swept every cycle) | A, better; C, weakly | Code in `stacks/radio`, no new privilege; still windows under I.16 | Capped by the frame budget (§2) and by the phone dozing. Raises the phone's heard rate when in use; does nothing for a sleeping phone. One sensor gives zones (in this room / in the house / at the edge), never a room (research §4.5). |
| **2. Ask the router who is connected** (each client's mesh node, signal and idle time, from the ASUS's own station list) | A, well; B, roughly (which mesh box it is on, so which end of the house) | A new read over the existing router SSH (I.15 allows only the connection table today); an I.15 amendment and a security review | The router hears every frame the phone sends, all the time, so it does not miss a dozing phone. *Assumption, not checked:* ASUS firmware offers `wl assoclist` / `wl sta_info` over SSH; nothing in the repo reads it (searched 2026-11-18). Only the owner's own devices are on it, so it is less sensitive than radio. |
| **3. Bluetooth through ESPHome Bluetooth proxies** (handoff item 5; research §3.8, §6) | A, B (with 2 or more proxies in different rooms), C | An ADR and a Constitution item (connections to LAN devices); one or more ESP32 boards | Phones advertise over Bluetooth far more often than they send Wi-Fi frames when idle (*assumption*, widely reported for iPhones; Android varies). This is what Home Assistant room presence (ESPresense, Bermuda) does. *Owner to confirm:* the "eveything presence lite" (`9714bc3d`, Espressif) is an ESPHome board that could also run a Bluetooth proxy, and nodeherder (`e36a93ec`) already reads ESPHome proxies. |
| **4. The network's presence check** (netaudit device inventory, every 5, 10, 15 or 30 minutes, `presencesettings.go`) | A, slowly | Nothing new | Built. 5 minutes at best, and a sleeping phone often does not answer. |
| ~~5. Wake the phone up~~ (send probes or frames to it) | — | — | **Ruled out:** I.16 says the radio "Sends nothing". |

**Recommendation (an assumption about what the owner wants, to confirm):**
- For **A**, option 2 is the strongest: the router already hears everything, and it needs no new radio privilege.
- For **B** and **C**, option 3 is the only one that gives rooms. It is the next architecture step already in the handoff.
- Option 1 is worth a small change on its own (extra listening on the owner's channels, built as focus windows, §7), but it will not make a
  dozing phone visible.

## 4. A measurement before any build (owner's step)

To know how much option 1 gains, run one 1-hour watch with channels `[60]` only, 30 s windows and a 120 s cycle (30 windows, at most
120k frames, under the budget), with the phone dozing and then carried around. Compare the phone's heard share with today's 29%. This needs
the owner: changing the plan and starting a watch both ask for the passphrase (I.17), and stopping today's 7-day watch is the owner's call.

## 4a. Seeing a device move across the radar (the owner's follow-up, same day)

**What the radar draws today.** A dot's distance comes from its signal (a near-to-far range, `DistanceModel.Range`, `distance.go`); its
**angle is not measured**: `angleOf` (`distance.go`) is a hash of the device's tag, "only a stable place on the ring". So with one adapter a
device can only move **in and out along its own spoke**, never across. The picture changes window by window (`LiveSnapshot`), and the page asks
every 15 s (`RadioRadar.vue` `REFRESH_MS`).

**What "across" needs: position in two dimensions, so two or more listeners in known places.**

| Listeners | What can be drawn honestly | How |
|---|---|---|
| 1 (today) | In this room / in the house / at the edge, and "getting nearer / farther" | Signal against a fitted path-loss model; a dozing phone gives few readings (§1) |
| 2 | "Nearer A or nearer B": a line between them | Compare the two signals for the same frame or advertisement |
| 3 or more, one per room | The room it is in (the nearest listener), and a trail from room to room | What Home Assistant's Bermuda and ESPresense do with ESP32 Bluetooth listeners: *nearest area*, not a point |
| 3+ with a walked map | A rough point (metres of error indoors) | Trilateration or fingerprints recorded room by room; walls make signal distance unreliable (research §3.6, §4.5) |

**Recommendation:** draw rooms, not coordinates: a plan of the house's listeners with each device beside the nearest one, its recent
moves as a short trail, and its uncertainty shown. A dot sliding smoothly across a map would be invented precision (research §4.5: "Report
zones, not metres").

**People rather than devices.** The Everything Presence Lite (`9714bc3d`) is a millimetre-wave presence sensor; nodeherder already draws
LD2450 targets moving across a room plan (nodeherder spec 040, "Spatial live tracking"; *assumption, not checked:* fed by this sensor). That
is real across-the-room movement of *people* in one room, without any radio privilege. It does not say whose phone it is.

**Which listeners.**
- **ESP32 boards running ESPHome as Bluetooth proxies** (research §3.8, §6; handoff item 5). They forward every Bluetooth advertisement
  they hear, with its signal, over the LAN, about once a second per device (*assumption:* typical, not measured here). Cheap, one per room.
  *Owner to confirm:* whether the Everything Presence Lite (`9714bc3d`) already runs ESPHome and can also be a proxy.
- **Trap: phones rotate their Bluetooth address about every 15 minutes** (*assumption*, widely reported). To follow your own phone you
  need its identity key (an iPhone's or Android's IRK, which Bermuda asks the owner to extract) or the phone advertising a fixed beacon from a
  companion app. Neighbours' phones stay anonymous rotating addresses, which suits I.16's tag rule.
- **The ASUS mesh** reports each client's signal at the node it is joined to, so two nodes give "which end of the house" (§3 option 2).
- More Wi-Fi monitor adapters on vertex do not help: they sit in the same place. A Wi-Fi listener in another room would be a remote
  sensor of its own (ESP32 promiscuous mode is possible but needs custom firmware, not stock ESPHome).

**What it would take to build** (each item needs the owner):
1. An ADR and a Constitution item: the core opening connections to listeners on the LAN (the ESPHome native API), what is kept of
   other people's Bluetooth devices (tags and day rows, as I.16), and that nothing is sent to them but the subscription. Two facts the ADR
   must settle first:
   - **Keys.** The native API normally uses an encryption key per device; nodeherder checks one before it connects
     (`ValidateEncryptionKey`, `backend/internal/deviceproviders/esphome/session.go` in nodeherder, read 2026-11-18). Each proxy's key
     would live in the secrets store, never answered back or logged.
   - **How many subscribers a proxy allows** for its advertisements: **unknown**, not stated in nodeherder's code or specs (searched
     2026-11-18). nodeherder already subscribes (`subscribeAdvertisements`, same file). If a proxy allows only one, this program would read
     the readings from nodeherder rather than from the proxies, which changes the design.
2. A sensor port in the stack: a listener is a named place; readings arrive as (tag, listener, signal, time), from the vertex adapter and
   from each proxy alike. ADR-0052 already reserves a Bluetooth driver.
3. A **per-reading, in-memory picture**, not per cycle: each device's latest signal per listener over the last minute, smoothed (a median
   with outliers dropped, research §3.6), its nearest listener, and a change of room only after it holds for a few readings.
4. A room layout the owner sets once: which listener is in which room, optionally where on a simple plan.
5. A pushed update to the page (server-sent events, which the core already uses for runs) instead of polling, so a move shows within a few
   seconds.

**Performance** (measured where stated, otherwise estimates):
- **The page is cheap today:** `GET /air/live` answered in 26–34 ms over five tries on vertex (38 KB, 62 devices), the device page in
  16–40 ms. One first call took 3.1 s, *assumption:* it met the cycle's write; worth watching, not fixing blind.
- **Bluetooth proxies:** *estimate:* about 1 reading a second per device per proxy; 40 devices × 3 proxies is about 120 readings a second.
  Folding that into an in-memory map is trivial for Go; the costs to design for are (a) **never one database write per reading**: keep
  readings in memory and write day rows and an optional track point at most once a minute, (b) push the picture to the page at most once
  a second, and only what changed, (c) the track table cap (`MaxTrackRows` = 300,000, `track.go`): one point a minute for 60 devices is
  86,400 a day, 172,800 in 48 hours, under the cap; one point per device **per listener** (about 518,000 in 48 hours with 3 listeners)
  or a point per Wi-Fi window would pass it, so keep one point per device a minute (its nearest listener and signal).
- **More Wi-Fi listening on vertex:** the frame budget (§2) is the limit, not the CPU: one adapter alternating between channels 6 and 60
  at today's rates (about 165 and 570 frames a second) reads about 1.3 million frames an hour, about 8 times the 170,000 budget, with up to
  about 2 MB of helper output for a full window (`RadioMaxOutput`; frames other than beacons are cut at 64 bytes, so usually less).
- **The frontend:** 60 to 100 dots moving with a short transition is light for SVG; the radar should animate only dots that changed.

## 5. Found on vertex during the same check (v0.97.0)

- **Works:** the Macs read "Computer" (from your name for it), "samsung - tv" a TV, the Nest Mini a speaker. The Roku you marked
  (`70f44fb0`) went from neighbour to "comes and goes" in the first cycle after the deploy.
- **Bug 1: the device page's chances disagree with the class for a device you marked.** `70f44fb0` reads "comes and goes, 100%" but its
  chances bars still show neighbour 56, comes and goes 37, passer-by 7. Cause: `explain` (`devicepage.go`) shows the raw `Model.Posterior`,
  which drops neighbour and passer-by only for a device on your Wi-Fi (`bOnOwn`, `learn.go` `Posterior`); `decideYours` drops them for a
  device you marked, as a share of the remaining two (37 / (0 + 37) = 100%). **Fixed in v0.98.0** (branch
  `radio-page-chances-and-ask-name`): one rule (`vector.houses`, `ownedVector` in `learn.go`) limits both, so the bars, the factors and the
  class agree; the owner's word is never set on a training vector.
- **Bug 2: "Mine" on the Help it learn card saves the guess as a permanent name.** `RadioAskCard.vue` sends `defaultLabel(m)`
  (`utils/radio/identity.ts`), which falls back to the kind word. The two Macs were guessed phones when they were answered, so their stored
  label is "Phone or tablet", and it now sits next to the kind "Computer". (The inventory name cannot be stored instead: I.16 says nothing
  of the inventory is written to the stack's file.) **Fixed in v0.98.0:** the offered name is the make and model, the maker or "My device",
  never the kind; and a device you marked can be renamed on its page (`PUT /admin/api/radio/air/mine/{id}`), which the two Macs need.
- Tapping a row to open the page cannot be checked through the API: the owner checks it.

## 7. Built the same day: focus windows between sweeps (v0.98.0, branch `radio-page-chances-and-ask-name`)

The owner rejected "listen only on channels 6 and 60" because it would stop finding unknown devices, and asked for an adaptive plan.

- **The sweep is unchanged.** Every channel of the plan is still heard every cycle, so unknown devices are found as before, and every stored
  number (day rows, shares, classes, the movement share, the track) still comes from sweeps only.
- **Focus windows use the idle time after each sweep** (about 150 s of a 300 s cycle), on the channels the owner's devices were heard on in
  that sweep, weighted by their frames (`newFocusPlan`, `focus.go`, from `WindowSummary.OwnFrames`); a channel with none gets no focus
  window. Each is the plan's own request (that channel, the plan's window length), so the session already approved it.
- **They write nothing.** `FocusIngest` parses, tags and sums per device; no `sight`, event, beacon, join or track write. The readings are
  a ring of 8 per device, at most 500 devices, kept 15 minutes, cleared with the run. They make the dot's newest reading and a "moving
  now" word (a 10 dB change within 3 minutes, `movingOf`). Test: `TestAFocusWindowMovesTheDotAndWritesNothing` compares the stored rows and
  track points of sweeps with focus windows against sweeps alone.
- **Budgets.** Focus frames have their own budget (`focusFramesPerHour` = 170,000, paced over the run, plus one window's worth), separate
  from the sweep's `framesPerHour`, so they can never end a watch. At most `focusPerHour` focus windows an hour (72 for the default plan),
  and the session's uses are sized from the stack's `windows_per_hour` (sweep 72 + focus 72 = 144, under the helper's 240), so focus
  never spends a session sized for sweeps. A start runs focus windows only when it says its session was sized for them
  (`"focus": true` in the start body, sent by the page): a session's uses are one pool, so focus windows on a session sized for sweeps
  would spend the sweeps' uses and end the watch early. A refused or failed focus window turns focusing off for the run; the sweep goes on.
- **Cost, measured** (`BenchmarkFocusIngest`, `BenchmarkSweepIngest`, on the development Mac, a full 4,000-frame window from 60 devices):
  focus 241 ns a frame (about 1 ms a window, 1.7 MB allocated and dropped); the sweep's path 2,079 ns a frame (about 8 ms, it writes).
  At the focus budget, about 41 ms of CPU an hour here; vertex is slower, *estimate* under 0.5 s an hour. Resident memory for focus:
  at most about 100 KB.
- **Expected gain (estimate from one measured cycle):** channel 60 from 12 visits an hour to about 40, channel 6 to about 25; the frame
  budget, not the time, is what limits it.
- **Tried and dropped:** placing a dot at the median of its newest three readings. It delayed a real move by two readings (10 minutes for a
  device on a channel with no focus window), and a window's reading is already the mean of every frame the device sent in 20 s. Three
  existing tests caught it.
- **Only on a new watch:** a deploy restarts the service, which ends every session; the next watch is sized for focus windows.

## 8. The router read (built 2026-11-18 on the same branch, ADR-0059, I.15 amended; the owner approved on 2026-11-18)

- **No stack-to-stack dependency.** The core owns the router login today (`backend/internal/platform/routerssh/routerssh.go`, one fixed
  command, `cat /proc/net/nf_conntrack`, at most once every 3 hours: `routerConnReadGap`, `backend/internal/app/netaudit_routerconn.go`).
  The Radio stack already reads the core's inventory through the `lan` port (`stacks/sdk/ports.go` `LAN`, `backend/internal/core/stackport/lan.go`),
  as tags, at most once a minute, and treats it as evidence, never a condition (`stacks/radio/lan.go`). The router read would add, in the
  core, a second fixed read-only command for the connected-clients list, and, through the same port, per tag: which mesh box (a number),
  its signal there, its idle time and when the list was read. No address crosses; with the network module off, the radio works as today.
- **One connection kept open while a watch runs (recommended):** connect at the watch's start with the pinned host key, open a short
  session for each read, close the connection with the watch, and reconnect with backoff if it drops. A login every 30 s would be a key
  exchange each time on both machines and about 2,900 logins a day in the router's log. Not a loop on the router: one fixed command per
  session keeps the read bounded. *Unverified:* that the router's SSH server serves several sessions on one connection (the handoff's
  command 4 checks it); if not, one login per read, at most once a minute.
- **Bounds:** the list kept in memory only, at most the inventory's 1,024 devices; one read at a time; a timeout per read; an output cap
  as `routerssh` already has (`capped`).
- **Needs:** an I.15 amendment (a second command, a read every 30 to 60 s while a radio watch runs, kept in memory only) and the
  owner's router output (handoff, "Commands for the router read").

### 8.1 Measured on the owner's router, 2026-11-18 (read-only, run by the owner; addresses masked, none recorded here)
- **Router:** firmware `3.0.0.4` / `388.12` / `2` (*assumption:* Asuswrt-Merlin 388.12_2), `sw_mode` 1, AiMesh main (`cfg_master` 1), radios
  `eth6` (2.4 GHz) and `eth7` (5 GHz). SSH on port 23 with its own key.
- **`/tmp/clientlist.json` covers both mesh boxes:** an object per box (keyed by the box's address), per band (`2G`, `5G`, guest `2G_1`),
  per client `{"ip", "rssi"}`, under 1 KB. The boxes come in either order and a `wired_mac` key appears in some reads and not others, so a
  parser must not rely on order or on optional keys. The satellite's own backhaul radios are listed as clients of the main box. No idle time.
- **It is rewritten about every 30 s** (six checks 20 s apart: rewrites 24, 7, 30 and 30 s apart, one 20 s gap with none): a read every
  30 s gets each new list; more often reads the same file.
- **`wl -i <radio> sta_info <address>`** on the main box gives idle seconds, seconds joined and a smoothed signal, for its own clients only.
- **One SSH connection served a second session** (OpenSSH connection sharing): the keep-one-connection-open design works on this router.
- **Walk test (3 minutes, one list every 15 s):** one 5 GHz client on the satellite with a private address (*assumption, owner to
  confirm:* the owner's phone) read -69/-70 for 90 s, -57 for 30 s, then -73/-74; a change of 13 to 17 dB, shown within one rewrite of the
  file. **No client changed box in the 3 minutes:** phones stay on the box they joined until the signal is poor, so "which box" changes
  slowly; the signal at that box is the faster sign of movement.
- **What this opens:** for a device on the satellite, the satellite's reading and vertex's own adapter are two listeners in different
  places, the "nearer A or nearer B" of §4a, with no new hardware.

### 8.2 Built (v0.98.0, branch `radio-page-chances-and-ask-name`, not run on vertex)
- `platform/routerssh`: `ClientsCommand` (`cat /tmp/clientlist.json`, at most 64 KiB), `Dial` and `Conn.ReadClients` (a new session a read on a
  kept-open connection); `ReadTable` shares the sign-in (`signIn`) and its pinned-key check.
- `core/netaudit/routerconn`: `ParseClients` (strict, bounded, fuzzed; drops the addresses on this network, `wired_mac`, and a box's own radios
  by their first five bytes, measured on one router) and `Manager.MeshClients` (at most once every 30 s, a wait doubling after each failure up
  to 10 minutes, its own lock so the operator's read is never "busy", the connection closed after 2 minutes idle and by `CloseMesh` on
  `SetTarget`, `SetKey`, `ClearKey`, `Pin`, the factory reset and shutdown).
- `core/stackport.Mesh` and the SDK port `meshclients` (only the Radio stack requires it; `TestOnlyTheRadioStackIsGivenTheRoutersListOfDevices`):
  tags under the stack's purpose, box names from the inventory, the gateway as the main box; no address crosses.
- `stacks/radio/mesh.go`: asked at most once every 30 s while a watch runs (after each window, and in 30 s steps through the idle wait);
  each dot carries `router` (box, name, main, band, guest, signal, read time, moving); cleared with the run; a test compares the stored rows
  with and without it.

