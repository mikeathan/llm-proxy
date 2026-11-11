import { describe, expect, it } from 'vitest'
import {
  chartBars, countryBars, countryName, directionUnknown, flowsOfKind, hopWords, mapDots, ownerBars, ownerLine, ownerName, isSharedAddress, kindShares, locationOf, routeOutcome,
  routePoints, sampleNote, secondAt, secondClock, selfLine, serviceOf, splitPercent, stoppedNote, talkGroups, trafficPlace, trafficSentence, usedIn,
} from '../../../utils/netaudit/traffic'
import type { HostTraffic, RouteTrace, TrafficFlow } from '../../../types/netaudit'

const MB = 1024 * 1024
const traffic = (over: Partial<HostTraffic> = {}): HostTraffic => ({
  at: '2026-11-09T20:00:00Z', interface: 'wlp2s0', stopped_by: 'window', elapsed_ms: 60_000, frames: 100, unreadable: 0, kernel_seen: 100, kernel_dropped: 0, sample: 1,
  bytes_in: 38 * MB, bytes_out: 4 * MB, bytes_through: 0, internet_bytes: 41 * MB, local_network_bytes: MB, lookups: 212,
  kinds: [{ kind: 'https', bytes: 40 * MB }, { kind: 'dns', bytes: MB }, { kind: 'udp', bytes: MB }],
  per_second: [], destinations: [], flows: [], other_flows: 0, other_bytes: 0, ...over,
})

describe('trafficSentence', () => {
  it('says what went out and came in over how long, where it went and how encrypted it was', () => {
    const s = trafficSentence(traffic())
    expect(s.headline).toBe('In 60 seconds this computer sent 4 MB and received 38 MB.')
    expect(s.detail).toContain('41 MB to the internet, 1 MB on this network.')
    expect(s.detail).toContain('95% of it was encrypted web traffic.')
    expect(s.detail).toContain('212 names were looked up.')
  })

  it('never says nothing was sent when it could not tell which way the traffic went', () => {
    // The look vertex kept on 2026-11-11, before the direction was fixed: everything counted as passing through, no flag.
    const old = traffic({ bytes_in: 0, bytes_out: 0, bytes_through: 1_664_770 })
    expect(directionUnknown(old)).toBe(true)
    expect(trafficSentence(old).headline).toContain('could not tell which way it went')
    expect(trafficSentence(old).headline).not.toContain('nothing was sent')
    expect(directionUnknown(traffic({ direction_unknown: true }))).toBe(true)
    expect(directionUnknown(traffic())).toBe(false)
  })

  it('leaves out what it cannot say: no destinations, no lookups, nothing sent', () => {
    const quiet = trafficSentence(traffic({ bytes_in: 0, bytes_out: 0, internet_bytes: 0, local_network_bytes: 0, kinds: [], lookups: 0, destinations: [] }))
    expect(quiet.headline).toBe('In 60 seconds nothing was sent or received by this computer.')
    expect(quiet.detail).toBe('')
  })

  it('says one name, not "1 names", and a window that was cut short is not called a minute', () => {
    const s = trafficSentence(traffic({ elapsed_ms: 1_000, lookups: 1 }))
    expect(s.headline).toContain('In 1 second')
    expect(s.detail).toContain('1 name was looked up.')
  })
})

describe('kindShares', () => {
  it('gives each kind its share of the whole, biggest first, as words the screen can show', () => {
    const shares = kindShares(traffic().kinds)
    expect(shares.map((k) => [k.label, k.percent])).toEqual([['Web, encrypted (HTTPS)', 95], ['Name lookups (DNS)', 2], ['Other (UDP)', 2]])
  })

  it('has no shares for a window with no traffic', () => {
    expect(kindShares([])).toEqual([])
    expect(kindShares([{ kind: 'https', bytes: 0 }])).toEqual([])
  })
})

describe('chartBars', () => {
  it('scales the seconds to the chart, the busiest one to its full height, and a quiet second to a hairline', () => {
    const { bars, busiest } = chartBars([0, 50, 100], 300, 100)
    expect(bars).toHaveLength(3)
    expect(bars[2]!.height).toBe(100)
    expect(bars[1]!.height).toBe(50)
    expect(bars[0]!.height).toBe(1)
    expect(bars[2]!.y).toBe(0)
    expect(busiest).toEqual({ second: 2, bytes: 100 })
    expect(bars[0]!.x).toBeLessThan(bars[1]!.x)
    expect(bars[2]!.x + bars[2]!.width).toBeLessThanOrEqual(300)
  })

  it('draws nothing for a window with no seconds and nothing busiest for a silent one', () => {
    expect(chartBars([], 300, 100)).toEqual({ bars: [], busiest: null })
    expect(chartBars([0, 0], 300, 100).busiest).toBeNull()
  })
})

describe('selfLine', () => {
  it('sets this program\'s own page and API apart with its share, and says nothing when there was none', () => {
    const t = traffic({ bytes_in: 300, bytes_out: 700, self_bytes: 890 })
    expect(selfLine(t)).toContain('890 B (89%) was this program\'s own page and API')
    expect(selfLine(t)).toContain('most of it likely your browser')
    expect(selfLine(traffic())).toBe('')
  })
})

describe('the notes about how trustworthy the numbers are', () => {
  it('calls sampled bytes an estimate and drops a complete window\'s note', () => {
    expect(sampleNote(traffic({ sample: 1 }))).toBe('')
    expect(sampleNote(traffic({ sample: 8 }))).toBe('Estimated: one ordinary frame in 8 was counted, because there was too much traffic to keep every frame.')
  })

  it('says every way a window ended, and how many frames the computer dropped', () => {
    expect(stoppedNote(traffic())).toBe('')
    expect(stoppedNote(traffic({ stopped_by: 'frame_limit' }))).toContain('limit')
    expect(stoppedNote(traffic({ stopped_by: 'cut' }))).toContain('partial')
    expect(stoppedNote(traffic({ kernel_dropped: 7 }))).toContain('7 frames were dropped')
  })
})

describe('splitPercent', () => {
  it('gives two shares that always add to 100, and zeros when nothing moved', () => {
    expect(splitPercent(1, 2)).toEqual([33, 67])
    expect(splitPercent(1, 1)).toEqual([50, 50])
    expect(splitPercent(0, 0)).toEqual([0, 0])
  })
})

describe('trafficPlace and friends', () => {
  const t = {
    bytes_in: 900, bytes_out: 100, bytes_through: 0,
    destinations: [{ name: 'a.example', address: '1.1.1.1', bytes: 500, internet: true }],
    flows: [
      { address: '1.1.1.1', name: 'a.example', port: 443, proto: 'tcp', kind: 'https', internet: true, bytes_out: 50, bytes_in: 450 },
      { address: '192.168.1.9', port: 22, proto: 'tcp', kind: 'ssh', internet: false, bytes_out: 10, bytes_in: 20 },
    ],
  } as unknown as HostTraffic

  it('gathers a place from the destinations and the conversations, with its share', () => {
    expect(trafficPlace(t, '1.1.1.1')).toMatchObject({ name: 'a.example', bytes: 500, percent: 50, scannable: false, internet: true })
  })

  it('sizes a place that is only in the conversations, and lets a device on this network be scanned', () => {
    expect(trafficPlace(t, '192.168.1.9')).toMatchObject({ bytes: 30, percent: 3, scannable: true })
  })

  it('knows nothing of an address the look never saw', () => {
    expect(trafficPlace(t, '9.9.9.9')).toBeNull()
  })

  it.each([['224.0.0.251', true], ['239.255.255.250', true], ['255.255.255.255', true], ['ff02::fb', true], ['192.168.1.9', false], ['10.0.0.1', false]])('says %s is shared: %s', (address, shared) => {
    expect(isSharedAddress(address)).toBe(shared)
  })

  it('keeps the conversations of one kind, or all', () => {
    expect(flowsOfKind(t.flows, 'ssh')).toHaveLength(1)
    expect(flowsOfKind(t.flows, null)).toHaveLength(2)
  })

  it('keeps a click inside the seconds there are', () => {
    expect(secondAt(0, 960, 60)).toBe(0)
    expect(secondAt(480, 960, 60)).toBe(30)
    expect(secondAt(5000, 960, 60)).toBe(59)
    expect(secondAt(-4, 960, 60)).toBe(0)
    expect(secondClock(75)).toBe('1:15')
  })
})

describe('owners of an address', () => {
  it('writes the operator and where the address is registered when no city is known, and the number when the holder has no name', () => {
    expect(ownerLine({ asn: 15169, name: 'GOOGLE - Google LLC', country: 'US' })).toBe('Google LLC · registered in United States')
    expect(ownerLine({ asn: 64500, name: '', country: 'DE' })).toBe('AS64500 · registered in Germany')
    expect(ownerLine({ asn: 64500, name: 'X' })).toBe('X')
    expect(ownerLine(undefined)).toBe('')
  })

  it('says where an address is used from the table of cities, in that table\'s country, never mixed with the registration', () => {
    // The Cloudflare address of the vertex look: registered in the US, used in Toronto.
    const cf = { asn: 13335, name: 'CLOUDFLARENET - Cloudflare, Inc.', country: 'US', place_country: 'CA', region: 'Ontario', city: 'Toronto' }
    expect(usedIn(cf)).toBe('Toronto, Ontario, Canada')
    expect(locationOf(cf)).toBe('Toronto, Ontario, Canada')
    expect(ownerLine(cf)).toBe('Cloudflare, Inc. · Toronto, Ontario, Canada')
    expect(usedIn({ asn: 1, name: 'SG', country: 'SG', place_country: 'SG', city: 'Singapore', region: 'Singapore' })).toBe('Singapore, Singapore')
    expect(usedIn({ asn: 1, name: 'X', country: 'US' })).toBe('')
  })

  it('shortens a holder named "HANDLE - Name" to its name, and keeps one that has no separator', () => {
    expect(ownerName({ asn: 1, name: 'CLOUDFLARENET - Cloudflare, Inc.' })).toBe('Cloudflare, Inc.')
    expect(ownerName({ asn: 1, name: 'AKAMAI-AS' })).toBe('AKAMAI-AS')
    expect(ownerName({ asn: 1, name: 'X - ' })).toBe('X - ')
  })

  it('gives the country code back when it is not one the browser knows', () => {
    expect(countryName('ZZ')).toBeTruthy()
    expect(countryName('not a code')).toBe('not a code')
  })

  it('makes bars that are shares of the internet traffic, and none from a look kept before owners existed', () => {
    const t = { internet_bytes: 200, owners: [{ asn: 1, name: 'A', country: 'US', bytes: 100 }], countries: [{ country: 'US', bytes: 150 }] } as unknown as HostTraffic
    expect(ownerBars(t)).toEqual([{ key: 'AS1', label: 'A', note: 'AS1 · United States', bytes: 100, percent: 50 }])
    expect(countryBars(t)[0]).toMatchObject({ label: 'United States', percent: 75 })
    const old = { internet_bytes: 200 } as unknown as HostTraffic
    expect(ownerBars(old)).toEqual([])
    expect(countryBars({ ...old, owners: null, countries: null } as unknown as HostTraffic)).toEqual([])
  })
})

const flow = (over: Partial<TrafficFlow>): TrafficFlow => ({ address: '1.1.1.1', port: 443, proto: 'tcp', kind: 'https', internet: true, bytes_out: 0, bytes_in: 0, ...over })

describe('serviceOf', () => {
  it('names a conversation by its kind, by protocol and port when the kind is plain, and as served here when another machine connected', () => {
    expect(serviceOf(flow({}))).toBe('HTTPS')
    expect(serviceOf(flow({ kind: 'udp', proto: 'udp', port: 9999 }))).toBe('UDP 9999')
    expect(serviceOf(flow({ kind: 'icmp', proto: 'icmp', port: 0 }))).toBe('Ping')
    expect(serviceOf(flow({ kind: 'tcp', port: 4001, served: true }))).toBe('to this computer :4001')
    expect(serviceOf(flow({ kind: 'ssh', port: 22, served: true }))).toBe('SSH to this computer :22')
  })
})

describe('talkGroups', () => {
  // Shaped like the vertex look once it is read right: the owner's browser on .251 (this program's port), one Cloudflare address, mDNS chatter.
  const t = traffic({
    bytes_in: 20_000, bytes_out: 1_500_000, bytes_through: 0, self_bytes: 1_480_000,
    destinations: [
      { address: '192.168.50.251', bytes: 1_485_688, internet: false },
      { address: '224.0.0.251', bytes: 46_629, internet: false },
      { address: '104.16.1.1', bytes: 104, internet: true, owner: { asn: 13335, name: 'CLOUDFLARENET - Cloudflare, Inc.', country: 'US', place_country: 'CA', city: 'Toronto', point: { lat: 43.7, lon: -79.4 } } },
    ],
    flows: [
      flow({ address: '192.168.50.251', port: 4001, kind: 'tcp', internet: false, served: true, self: true, bytes_out: 1_460_000, bytes_in: 20_000 }),
      flow({ address: '192.168.50.251', port: 22, kind: 'ssh', internet: false, served: true, bytes_out: 4_000, bytes_in: 1_688 }),
      flow({ address: '224.0.0.251', port: 5353, proto: 'udp', kind: 'discovery', internet: false, bytes_out: 46_629 }),
      flow({ address: '104.16.1.1', name: 'example.com', name_from: 'tls', bytes_out: 60, bytes_in: 44 }),
    ],
  })
  const groups = talkGroups(t, { '192.168.50.251': 'Mike\'s Mac' })
  const of = (id: string) => groups.find((g) => g.id === id)!

  it('puts each place in its group: the internet, this network, and broadcast chatter', () => {
    expect(groups.map((g) => g.id)).toEqual(['internet', 'network', 'shared'])
    expect(of('internet').rows.map((r) => r.address)).toEqual(['104.16.1.1'])
    expect(of('network').rows.map((r) => r.address)).toEqual(['192.168.50.251'])
    expect(of('shared').rows.map((r) => r.address)).toEqual(['224.0.0.251'])
  })

  it('leaves this program\'s own page out of a place\'s total and its services, and keeps the rest', () => {
    const mac = of('network').rows[0]!
    expect(mac.bytes).toBe(5_688)
    expect(mac.services).toEqual(['SSH to this computer :22'])
    expect(mac.label).toBe('Mike\'s Mac')
    expect(mac.named).toBe(true)
    expect([mac.sent, mac.received]).toEqual([4_000, 1_688])
  })

  it('drops a place whose only traffic was this program\'s own page', () => {
    const only = talkGroups(traffic({ destinations: [{ address: '192.168.50.9', bytes: 500, internet: false }], flows: [flow({ address: '192.168.50.9', internet: false, served: true, self: true, bytes_out: 500 })] }), {})
    expect(only.every((g) => g.rows.length === 0)).toBe(true)
  })

  it('names an internet place by the TLS name of its conversation and carries its owner', () => {
    const cf = of('internet').rows[0]!
    expect(cf.label).toBe('example.com')
    expect(cf.owner?.asn).toBe(13335)
  })

  it('puts the internet places with a city on the map, joined by city, and marks the open one', () => {
    const rows = groups.flatMap((g) => g.rows)
    const dots = mapDots(rows, '104.16.1.1')
    expect(dots).toHaveLength(1)
    expect(dots[0]).toMatchObject({ address: '104.16.1.1', lat: 43.7, lon: -79.4, bytes: 104, picked: true })
    expect(dots[0]!.label).toBe('Toronto, Canada · Cloudflare, Inc.')
    expect(mapDots(rows, null)[0]!.picked).toBe(false)
  })
})

describe('a route trace on the screen', () => {
  const trace: RouteTrace = {
    address: '1.1.1.1', state: 'done', started_at: '2026-11-11T14:00:00Z', reached: true,
    hops: [
      { ttl: 1, address: '192.168.50.1', sent: 3, answered: 3, best_ms: 4.4, where: 'this_network' },
      { ttl: 2, sent: 3, answered: 0 },
      { ttl: 3, address: '90.212.129.56', sent: 3, answered: 2, best_ms: 8.49, where: 'internet', owner: { asn: 5607, name: 'BSKYB-BROADBAND-AS', country: 'GB', place_country: 'GB', city: 'London', point: { lat: 51.5, lon: -0.1 } } },
      { ttl: 4, address: '62.252.1.1', sent: 3, answered: 3, best_ms: 12, where: 'internet', owner: { asn: 1, name: 'X', point: { lat: 51.5, lon: -0.1 } } },
      { ttl: 5, address: '1.1.1.1', sent: 3, answered: 3, best_ms: 16.9, where: 'internet', reached: true, owner: { asn: 13335, name: 'CF', point: { lat: 51.5, lon: -0.2 } } },
    ],
  }

  it('words each step: who answered, where it is, how quickly; a silent step says so', () => {
    expect(hopWords(trace.hops[0]!)).toEqual({ who: '192.168.50.1', where: 'your network', time: '4.4 ms' })
    expect(hopWords(trace.hops[1]!)).toEqual({ who: 'no answer', where: '', time: '' })
    expect(hopWords(trace.hops[2]!).where).toBe('BSKYB-BROADBAND-AS · London, United Kingdom')
    expect(hopWords(trace.hops[4]!).time).toBe('17 ms')
  })

  it('draws a line through the steps with a city, a city repeated in a row once', () => {
    expect(routePoints(trace)).toEqual([{ lat: 51.5, lon: -0.1 }, { lat: 51.5, lon: -0.2 }])
    expect(routePoints(null)).toEqual([])
  })

  it('says how a trace ended in plain words', () => {
    expect(routeOutcome(trace)).toBe('Reached in 5 steps.')
    expect(routeOutcome({ ...trace, reached: false })).toContain('did not answer')
    expect(routeOutcome({ ...trace, state: 'failed', error: 'unsupported' })).toContain('needs Linux')
    expect(routeOutcome({ ...trace, state: 'running' })).toBe('')
  })
})

describe('broadcast chatter', () => {
  it('files the limited and the subnet broadcast as chatter, even a look kept before the server marked them, and never on the internet', () => {
    // The vertex look of 2026-11-11 (v0.73.0): 255.255.255.255 came marked as internet, and 192.168.50.255 as a device.
    const t = traffic({
      destinations: [
        { address: '255.255.255.255', bytes: 625, internet: true },
        { address: '192.168.50.255', bytes: 5000, internet: false, shared: true },
        { address: '192.168.50.1', bytes: 9000, internet: false },
      ],
      flows: [],
    })
    const groups = talkGroups(t, {})
    const ids = (id: string) => groups.find((g) => g.id === id)!.rows.map((r) => r.address)
    expect(ids('internet')).toEqual([])
    expect(ids('shared')).toEqual(['192.168.50.255', '255.255.255.255'])
    expect(ids('network')).toEqual(['192.168.50.1'])
    expect(trafficPlace(t, '255.255.255.255')).toMatchObject({ internet: false, scannable: false })
    expect(trafficPlace(t, '192.168.50.255')).toMatchObject({ internet: false, scannable: false })
  })
})
