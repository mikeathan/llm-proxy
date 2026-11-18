import { describe, expect, it } from 'vitest'
import { networkGroups } from '../../../utils/radio/networks'
import type { RadioNetworkAP } from '../../../types/radio'

const ap = (id: string, name: string, channel: number, confirmed: boolean): RadioNetworkAP => ({ id, name, channel, security: 'wpa2', confirmed, last_seen: '2026-11-18' })

describe('networkGroups', () => {
  it('groups the access points under the name they broadcast, in your order, with the candidates apart', () => {
    const g = networkGroups({ names: ['home', 'home-2G', 'old'], access_points: [ap('a', 'home', 60, true), ap('b', 'home-2G', 6, true), ap('c', 'home', 60, true), ap('d', 'home', 36, false)] })
    expect(g.map((x) => x.name)).toEqual(['home', 'home-2G', 'old'])
    expect(g[0]!.confirmed.map((a) => a.id)).toEqual(['a', 'c'])
    expect(g[0]!.candidates.map((a) => a.id)).toEqual(['d'])
    expect(g[0]!.channels).toEqual([60])
    expect(g[2]!.confirmed).toEqual([])
  })

  it('keeps an access point whose name is no longer in your list', () => {
    expect(networkGroups({ names: [], access_points: [ap('a', 'gone', 1, true)] }).map((x) => x.name)).toEqual(['gone'])
  })
})
