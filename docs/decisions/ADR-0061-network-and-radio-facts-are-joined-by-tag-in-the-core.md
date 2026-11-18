---
id: ADR-0061
title: Network and Radio facts about one device are joined by tag in the core
status: proposed
date: 2026-11-18
subsystems: [network-audit]
supersedes:
superseded_by:
---

# ADR-0061: Network and Radio facts about one device are joined by tag in the core

## Context
The owner asked (2026-11-18) whether the network data and the radio data can be used together without making the stacks depend on each other,
and then for the whole of `docs/PLANS/cross-cutting/network-radio-joined-by-tag.md`, with no more noise on screen. The two sides each know
half of a device: the core's presence check knows that a phone is on the network now; Radio knows where in the house it was heard and that it
tried a café's network. On vertex the phone `4ded1379` was heard over the air only every 5 to 30 minutes while the mesh router listed it
20 seconds earlier, so Radio's My devices said "Not heard lately" for a phone that was at home.

Constitution IV.6 forbids a stack to import or call another; I.16 lets the core hand Radio the inventory as tags under Radio's own vault
purpose (the `lan` port, ADR-0058) and says nothing of what may come back. ADR-0036 lets a stack offer a finding only about an address.

## Decision
1. **One owner of the join.** `backend/internal/core/devicejoin` is the only code that sees both sides. It keeps no store and no key: it reads
   the inventory, pulls Radio's air report through the stack host, makes the vault's tag of each inventory hardware address under the purposes
   Radio itself asked the `lan` port for (remembered in memory by `stackport.AskedPurposes`, the newest two, so a key rotation keeps working),
   and keeps only the facts whose tag is an inventory device's. Its answer is cached for a minute and read with a 3 s timeout. It is wired in
   `internal/app/devicejoin.go`.
2. **Ports, plain data.** `LANDevice` gains `Presence` (online, away, or empty: unknown while the presence check is off or has not looked
   within an hour, never read as away). A stack that declares `Manifest.ReportsAir` (which needs the `lan` port) implements
   `AirReporter.Air`, an `AirReport` of fixed values per inventory device (`AirFact`: last heard, band, ring, moving, on your Wi-Fi, kind,
   each from the SDK's word lists). `StackFinding` gains `Device`, a tag; the stack host maps it to the inventory device's address or drops
   the finding (this amends ADR-0036).
3. **What each side shows, and nothing more.** Radio's My devices: a device the network sees now reads "Home · on the network" in the one
   status tag. Network's device row: the existing "First seen" line gains "heard over the air 2 minutes ago, in the house" (Advanced adds the
   band, movement, your Wi-Fi and the kind); the closed row is unchanged. The new-device alert gains one fixed phrase with counts. The nightly
   report words `radio_join_foreign_network` on the device (low severity).
4. **Either side off is a state.** No Radio stack, Radio switched off, not listening in the last day, or a report that could not be read are
   the states `off`, `not_listening` and `unreadable`; then the Network page shows nothing of the air and the alert has no phrase.

## Consequences
- No stack imports another and no address crosses; a neighbour's device matches no inventory tag, so it is never joined to anything.
- The new-device alert now says, in fixed words, whether Radio heard the device inside the house. That is a derived fact about an inventory
  device leaving through the connectors; it carries no name or address. **The owner's reading of I.16 for this is pending** (the amendment
  text is in the Constitution, in force when the pull request is merged).
- A Radio finding about a device the nightly scan did not see that night is left out with the rest of the findings about unscanned devices
  (`restrictTo`); it comes back on a night the device is up.
- Not done, with reasons (plan): a deauthentication burst joined with devices dropping off (the presence check keeps no departure times),
  and one look-alike check instead of two (Radio's `radio_rogue_twin` raises no alert and its subject is an access point).

## Alternatives considered
- **Radio returns the purpose its tags are made under.** Rejected: a text field back from the stack, and a stack could name another
  stack's purpose. Remembering the purposes the stack asked for keeps the choice in the core.
- **A second stack-to-stack port.** Rejected by IV.6.
- **New rows or a "Radio" column on the Network page.** Rejected by the owner's "no more noise": one phrase on a line that exists.
