---
id: ADR-0062
title: Presence reads the router's list of joined Wi-Fi devices, so a dozing phone stays online
status: proposed
date: 2026-11-18
subsystems: [network-audit]
supersedes:
superseded_by:
---

# ADR-0062: Presence reads the router's list of joined Wi-Fi devices, so a dozing phone stays online

## Context
The presence check marks a device online when the sweep finds it in the neighbour table within 30 minutes (`DeviceOnlineWindow`). A phone with
its screen off often does not answer, so it shows Offline while it is at home. ADR-0059 already reads the ASUS AiMesh's `/tmp/clientlist.json`
(every joined device on both mesh boxes, rewritten about every 30 s) for a running Radio watch only, and kept it out of anything stored. The
owner asked on 2026-11-18 for the same list to keep Network presence honest, with one rule: Network must not depend on Radio, and Radio must not
depend on Network.

## Decision
A Network module, **Joined devices, from the router** (`router_clients`, step-up, off by default, needs `presence` and a finished router
setup), lets each presence look call the router manager's `MeshClients` (`core/netaudit/routerjoined.go`, `RouterJoined.Look`, wired in
`internal/app/netaudit_routerconn.go`). The look runs after the alert. For each device the list names on a non-guest band and the inventory
already has, `Inventory.MarkJoined` writes `NetworkDevice.RouterSeen` = the read's time. Every "is it here lately" judgment uses the later of `LastSeen` and `RouterSeen` (`models.NetworkDevice.SeenAt`): `PresenceOf`, the
missing-device finding, the Overview's active count, the weekly digest and the DNS-silent note; so does the Network page
(`frontend/src/utils/netaudit/devices.ts` `lastJoined`).
- **Online only.** The mark never makes a device away, never adds a device (new-device alerts still come from the neighbour table) and never
  moves `LastSeen`, which decides which device holds an address (`lastOwners`, `newestDeviceAt`).
- **Quiet when it cannot read.** `ErrNotSet` and `ErrMeshWaiting` are nothing to do; any other failure is logged once per message and marks
  nothing. It is never a presence problem.
- **No dependency either way.** It reads through `routerconn.Manager`, which the core always builds; the Radio stack is not involved. Radio's
  `meshclients` port is unchanged and answers "unavailable" when the list cannot be had, and the run goes on as before
  (`TestTheRadioWorksAsBeforeWhenTheRoutersListIsUnavailable`, `TestRouterJoined_KeepsADeviceOnlineWithNoRadioStack`).

Constitution I.15 is amended to allow this second use of the same fixed command.

## Consequences
- A phone at home reads Online within one presence look of the router seeing it, dozing or not.
- Strain: one `cat` of a file under 1 KB per presence look, and never two reads within 5 minutes (`routerJoinedGap`: a look woken early
  by the router log does not read again). The connection closes after 2 minutes idle (ADR-0059), so each read is one SSH sign-in: at most 288
  a day on the router's log, fewer at longer intervals. While a Radio watch
  runs, the two share the 30 s cache and the open connection. On the service: one parse bounded to 64 KiB and one inventory write when a mark
  changed.
- Stored: one timestamp per device the owner already has. Nothing about the house's layout (box, band, signal) is stored.
- Only an ASUS router with `/tmp/clientlist.json` gives a list; any other router answers a failure and presence works as before.

## Alternatives considered
- **Bump `LastSeen`** — rejected: `LastSeen` decides who holds an address, and a sighting without an address would corrupt that.
- **Keep the connection open between presence looks** — rejected: an idle SSH session held for hours to save a sign-in every 5 minutes.
- **Feed presence from Radio's air report** — rejected: it makes Network depend on Radio, and a dozing phone is just as quiet over the air.
- **Let the list add new devices** — rejected: the list has no address, and a new device must keep the neighbour table's alert.

## Links
- ADR-0059 (the list, read for a Radio watch), ADR-0061 (Network and Radio joined by tag)
- Constitution I.15 (amended by this ADR)
- `docs/audits/2026-11-18-radio-continuous-monitoring.md` §3 option 2, §8
