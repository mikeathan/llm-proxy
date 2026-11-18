---
id: ADR-0063
title: One list of the owner's Wi-Fi networks, owned by the core, and one look-alike check over both sensors
status: proposed
date: 2026-11-18
subsystems: [network-audit]
supersedes:
superseded_by:
---

# ADR-0063: One list of the owner's Wi-Fi networks, owned by the core, and one look-alike check over both sensors

## Context
Two parts looked for a copy of the owner's Wi-Fi, and each kept its own list:
- The core's `wifi_lookalikes` module (ADR-0031) reads what this computer's NetworkManager hears. It keeps `NetauditConfig.WifiNetworks`
  (names plus confirmed BSSIDs) and alerts.
- The Radio stack (ADR-0058) hears every channel in monitor mode. It kept its own names (`RadioConfig.OwnNetworks`) and its own
  confirmations (`own_aps.confirmed`). Its `radio_rogue_twin` only went into the digest and raised no alert.

On vertex on 2026-11-18, read-only, Radio watched `netsink` and `netsink-2G` with two confirmed access points each, and the core watched only
`netsink` with two. The owner asked for one production-ready check (network-radio-join plan, 3d). The rule from 6a still applies: neither
side may depend on the other.

## Decision
- **The core owns the one list.** `WifiNetworks` is the only list of names and confirmed access points, and it changes only on the Network
  page (Wi-Fi look-alikes card, step-up as before).
- **Radio reads it through a new port, `ownnetworks`** (`stacksdk.OwnNetworks`, `core/stackport/networks.go`). The port gives names, plus
  the confirmed BSSIDs as tags under the purpose Radio asked for. Only Radio is given it
  (`TestOnlyTheRadioStackIsGivenTheOwnersWifiNetworks`).
  - Radio's names editor and its confirm and "not mine" routes are removed. Its networks view is read-only and links to the card.
  - Radio keeps only the guest flag, which is its own evidence rule.
  - Radio re-reads the list at each cycle, after an epoch rotation and whenever its views are read
    (`stacks/radio/ownlist.go`, `SyncOwnNetworks`).
- **Radio reports what it heard, never what is trusted.** The air report gains `APs`: for each access point heard lately under one of the
  owner's names, its tag and its BSSID sealed, both under the purpose asked of the port.
  - The core's `Networks.Sightings` opens the sealed value only under a purpose it handed out, and requires 6 bytes, unicast, and that its
    tag equals the reported tag. The name must be the owner's, byte for byte, and the list is bounded to 64. Anything else is dropped.
  - Constitution I.16 is amended for this one crossing.
- **One check over both sensors.** `WifiWatch` merges this computer's access points with Radio's sightings by BSSID. The result has one
  alert path, one throttle and one finding (`wifi_lookalike`).
  - An access point only Radio heard can be confirmed on the card, because `Confirm` still accepts only addresses the last look saw.
  - The look fails only when neither sensor answered.
  - The module is Ready when either sensor can look.
  - `radio_rogue_twin`, weak and PMF stay in Radio's digest, computed from the core's confirmed tags. They raise no separate alert.
- **No alert storm on upgrade.** Access points confirmed on the Radio page before (`own_aps.confirmed`, now read as `ConfirmedBefore`) are
  reported as such.
  - The card marks them "You confirmed it on the Radio page before" and offers one press to confirm them.
  - They raise no alert or finding until then, and are never treated as confirmed.
- **Names move once.** At start-up the core moves `RadioConfig.OwnNetworks` into `WifiNetworks` and clears it (`moveRadioNetworkNames`).
  The field is no longer sent to the stack. Access points are not moved: a stack never writes the trusted list.

## Consequences
- One place to confirm, one alert, and a look-alike only Radio can hear (another channel, out of this computer's reach) now reaches the owner.
- Network works without Radio: the Air source answers false and the check runs on NetworkManager alone. Radio works without Network: the port
  is the core's config, not a module, and the look-alike switch does not gate it.
- After the upgrade the owner confirms the Radio-confirmed access points once on the card. On vertex that is likely the two `netsink-2G`
  access points; `netsink` needs a confirmation only for any address not already confirmed in the core.
- `netsink-2G` arrives in the core list with nothing confirmed, so it is not judged until one access point is confirmed (`Lookalikes`' rule).

## Alternatives considered
- **A `Confirm` method on the port** — rejected: a stack would write the trusted list.
- **Import Radio's confirmations at start-up** — rejected for the same reason. A one-press confirmation keeps the owner as the only writer.
- **Keep both lists and dedupe the alerts** — rejected: two places to confirm and two answers to "is this mine".
- **Confirm by tag** — rejected: Radio's tags rotate with its epoch, so a stored tag would stop matching.

## Links
- ADR-0031 (the NetworkManager look), ADR-0058 (Radio identity), ADR-0061 (the air report), ADR-0062 (no dependency either way)
- Constitution I.16 (amended by this ADR)
- `docs/PLANS/cross-cutting/network-radio-join-handoff-2026-11-18.md` item 5 (3d)
