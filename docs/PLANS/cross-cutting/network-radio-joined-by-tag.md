---
status: proposed
date: 2026-11-18
last_reviewed: 2026-11-18
related_specs: [SPEC-006]
constitution_references: [I.16, IV.6]
evidence: Constitution IV.6 and I.16 as of 2026-11-18; `stacks/sdk/ports.go` (`LAN`, `MeshClients`, `StackFinding`); `backend/internal/core/stackport/lan.go`; ADR-0036, ADR-0058 §6, ADR-0059; vertex read-only GETs of 2026-11-18 (the phone `4ded1379` heard over the air every 5–30 min while the mesh router listed it 20 s earlier).
---

# Network and Radio, joined by tag in the core (options, 2026-11-18)

The owner asked (2026-11-18) whether the network data and the radio data can be used together **without creating dependencies** between the
stacks. They can, and the repo already does it in one direction.

## How it works (the pattern that exists)
- **Neither stack sees the other.** IV.6: a stack imports only the SDK; "stacks never call each other directly; a call between stacks goes
  through an SDK port". Most network facts (inventory, presence, router) live in the core, not in `stacks/network`.
- **The core is the only party that sees both, and it joins by tag.** The core holds the vault key. When Radio asks `LAN.Devices(purpose)`,
  the core tags each inventory address under Radio's purpose; that tag equals the tag Radio makes of the same address from a frame. So the two
  meet on a tag and no address crosses (ADR-0058 §6). `MeshClients` does the same for the router's client list (ADR-0059).
- **The core can join back.** It minted the tags, so it can tag each inventory address again and match a tag that Radio returns to the device
  on the Network page. That needs no new key and no stored map.
- **Either side can be off.** A join is a column, not a dependency: with Radio off, the Network page loses the over-the-air line and says
  so; with nothing listening, "not heard" is never read as "not there" (the rule the radio digest already follows).
- **One place.** The join is core code in one package (for example `internal/core/devicejoin`), wired in `internal/app/stacks.go`
  (the only file that names a stack), never spread across handlers.

## What the join could give, in order of effort

**1. Network facts on the Radio page (SDK field, no rule change).** Add to `LANDevice` what the core already knows and owns: online now /
last seen by the presence check, first seen. Radio then says "Home: on the network now, last heard over the air 12 min ago" for a sleeping
phone instead of "Not heard lately" (vertex: `4ded1379` is heard over the air every 5–30 min). A device heard strong over the air but not
on the network is a visitor's or a neighbour's. *Needs:* an SDK change (plain fields; ask first).

**2. Air facts on the Network page (new pull port, ADR, I.16 reading).** A Radio method the core pulls, like `Findings`: for tags of
inventory devices, fixed values only: heard now, last heard, band, ring (room / house / nearby / far), moving, on your Wi-Fi. No name and
no address leaves the stack; only devices already in the owner's inventory are asked about, so no neighbour's device is joined to anything.
The Network device row says "Over the air: near this computer, 5 GHz, heard 2 min ago" or "Never heard over the air while listening on its
channel: probably wired". *Needs:* an ADR, and the owner's reading of I.16 (it governs what leaves the Radio stack; derived values about
inventory devices are not named there).

**3. Joined findings, by a pure function in the core (each finding owner-approved, as P3 "one device, one story").**
- A Radio deauthentication burst and devices dropping off the network in the same minutes: "devices were knocked off Wi-Fi", inferred.
- A new device on the network (inventory alert) gains one fixed phrase: heard strong inside the house, or not heard over the air.
- An owner's device that tried a network not yours (`radio_join_foreign_network`) while it was off the home network.
- **One path per job:** the core's Wi-Fi look-alike check (`core/netaudit/wifilook`, nmcli) and Radio's `radio_rogue_twin` look for
  the same thing. When Radio is listening, keep one of them (Radio hears more).

**4. Radio findings in the nightly network report (SDK change).** `StackFinding.Subject` is an address, so Radio cannot offer a finding
today. A tag subject that the core maps back to an inventory device (as in 2) would let the nightly report carry a Wi-Fi section without a
second report. *Needs:* SDK change and ADR-0036 amended.

## Constraints that bind
- No stack imports another (IV.6); every value crosses as plain data through an SDK port.
- Radio facts reach the core only as fixed values, never a name or an address from a frame (I.16); the operator's names stay on screen,
  never in a finding, report, alert or prompt (ADR-0058 §6).
- A stack never pushes into another; the core pulls, with a timeout, and shows "could not read" on failure.

## Recommendation
1 first (small, rule-safe, fixes the "phone not heard" confusion the owner saw), then 2 with its ADR; 3 and 4 after 2 exists.

## Remaining work
- Owner: choose the steps; approve the SDK change for 1; read I.16 for 2.
