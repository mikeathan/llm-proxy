import type { NetworkGroup, RadioNetworks } from '../../types/radio'

// Radio · Your networks: the access points grouped under the network name they broadcast, so a mesh with two boxes reads as one network with
// two access points. Pure functions over what the server sent.


/** One group per name you gave, in your order, then any name an access point broadcasts that is no longer in your list. */
export function networkGroups(n: RadioNetworks): NetworkGroup[] {
  const names = [...n.names, ...n.access_points.map((a) => a.name).filter((x) => !n.names.includes(x))]
  return [...new Set(names)].map((name) => {
    const aps = n.access_points.filter((a) => a.name === name)
    const confirmed = aps.filter((a) => a.confirmed)
    return { name, channels: [...new Set(confirmed.map((a) => a.channel))].sort((a, b) => a - b), confirmed, candidates: aps.filter((a) => !a.confirmed) }
  })
}
