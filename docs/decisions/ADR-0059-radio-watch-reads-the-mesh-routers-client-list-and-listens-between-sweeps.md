---
id: ADR-0059
title: A radio watch listens again between sweeps on the owner's channels and reads the mesh router's list of joined devices
status: accepted
date: 2026-11-18
subsystems: [network-audit]
supersedes:
superseded_by:
---

# ADR-0059: A radio watch listens again between sweeps on the owner's channels and reads the mesh router's list of joined devices

## Context
The owner wants to see their phone move. Measured on vertex (`docs/audits/2026-11-18-radio-continuous-monitoring.md` §1, §2): the owner's
5 GHz channel gets about 7 s of listening per 5-minute cycle (the helper's 4,000-frame window cap), and the phone is heard in about 29% of
cycles while home, because a dozing phone sends very little. One adapter measures distance, not direction (`angleOf` is a tag hash). Two
levers need no new hardware: listen more on the owner's channels in the time a cycle leaves idle, and ask the router, which hears every
frame the phone sends. Measured on the owner's ASUS AiMesh (§8.1): `/tmp/clientlist.json` lists every joined device on both mesh boxes
with its signal, is rewritten about every 30 s, is under 1 KB, and a walk with the phone moved its signal 13 to 17 dB within one rewrite;
one SSH connection served a second session.

## Decision
We will (1) spend the idle time after each full sweep on **focus windows**: the channels the owner's devices were heard on in that sweep,
weighted by their frames, each the plan's own approved request; they move the live picture only and write nothing (`stacks/radio/focus.go`).
The owner answered on 2026-11-18 that Constitution I.16 covers them as written (still windows, within the helper's caps); a start runs them
only when its session was sized for them (`"focus": true`), and they have their own frame budget, so they can never end a watch.

We will (2) let a running radio watch cause reads of the router's list of joined devices, under an amendment of I.15: a second fixed
command, `cat /tmp/clientlist.json`, run by `platform/routerssh` and nothing else, over one connection kept open while reads continue,
at most once every 30 s however many callers ask, each read bounded in time and size, the connection closed after 2 minutes without a
read and whenever the setup changes. The core parses the list strictly, drops the addresses on this network at parse, and keeps per
hardware address only the box, the band, whether it is the guest network, and the signal, in memory. The Radio stack receives it through
its own port, `meshclients`, as tags under its vault purpose: no address crosses. It is shown on the Radio page only: it never feeds a
stored number, a class or the "on your Wi-Fi" evidence, and a guest-network client is never evidence that a device is the owner's.

## Consequences
- The owner's devices are seen within about 30 s of the router noticing, dozing or not, and "moving now" also comes from the router's
  readings. For a device joined to a mesh box in another place, the box's reading and vertex's adapter are two listeners.
- Nothing about the house's layout is stored: boxes are named from the network inventory (the gateway is the main router), and "nearer
  vertex or the box" is read from the signals each time.
- The router sees a login when a watch starts and after each 2-minute idle gap, not every 30 s. A watch keeps the router busy with a
  `cat` of a 1 KB file twice a minute.
- Two facts rest on one router: the backhaul filter (a client whose address shares its first five bytes with a box's is that box's own
  radio) and the file's shape. A router without the file answers a plain failure and the radio works as before.
- Accepted: the router's list is a record of which of the owner's devices were joined and where, kept in memory for the run.

## Alternatives considered
- **Listen only on the owner's channels (6 and 60)** — the owner refused it: it stops finding unknown devices.
- **Raise the sweep's frame budget** — buys more listening only on busy channels, and is still a passive listener that cannot hear a
  dozing phone.
- **`wl sta_info` per radio** — gives idle time, but only for the main box's own clients and with one command per client.
- **A login per read** — about 2,900 logins a day in the router's log and a key exchange on both machines each time.
- **A loop run on the router** — not one fixed command, and a hung loop on the router is harder to bound.
- **Smoothing the dot with a median of readings** — delayed a real move by two readings; dropped (audit §7).

## Links
- `docs/audits/2026-11-18-radio-continuous-monitoring.md` §7 (focus windows), §8 (the router read)
- Constitution I.15 (amended by this ADR), I.16, I.17
- ADR-0062 (proposed 2026-11-18): the same list also keeps Network presence online, read by the presence check's own code
- `docs/PLANS/cross-cutting/radio-stack.md` → Remaining work (the v0.99.0 handoff, harvested and deleted 2026-11-18)
