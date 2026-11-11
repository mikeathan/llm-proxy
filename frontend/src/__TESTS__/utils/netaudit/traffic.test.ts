import { describe, expect, it } from 'vitest'
import { chartBars, countryBars, countryName, destinationLabel, flowsOfKind, ownerBars, ownerLine, isSharedAddress, kindShares, sampleNote, secondAt, secondClock, splitPercent, stoppedNote, trafficPlace, trafficSentence } from '../../../utils/netaudit/traffic'
import type { HostTraffic } from '../../../types/netaudit'

const MB = 1024 * 1024
const traffic = (over: Partial<HostTraffic> = {}): HostTraffic => ({
  at: '2026-11-09T20:00:00Z', interface: 'wlp2s0', stopped_by: 'window', elapsed_ms: 60_000, frames: 100, unreadable: 0, kernel_seen: 100, kernel_dropped: 0, sample: 1,
  bytes_in: 38 * MB, bytes_out: 4 * MB, bytes_through: 0, internet_bytes: 41 * MB, local_network_bytes: MB, lookups: 212,
  kinds: [{ kind: 'https', bytes: 40 * MB }, { kind: 'dns', bytes: MB }, { kind: 'udp', bytes: MB }],
  per_second: [], destinations: [], flows: [], other_flows: 0, other_bytes: 0, ...over,
})

describe('trafficSentence', () => {
  it('says what went out and came in over how long, with how concentrated and how encrypted it was', () => {
    const t = traffic({
      destinations: [
        { name: 'a.example', address: '1.1.1.1', bytes: 22 * MB, internet: true }, { name: 'b.example', address: '2.2.2.2', bytes: 10 * MB, internet: true },
        { address: '3.3.3.3', bytes: 6 * MB, internet: true }, { address: '4.4.4.4', bytes: 4 * MB, internet: true },
      ],
    })
    const s = trafficSentence(t)
    expect(s.headline).toBe('In 60 seconds this computer sent 4 MB and received 38 MB.')
    expect(s.detail).toContain('3 destinations took 90% of it.')
    expect(s.detail).toContain('95% was encrypted web traffic (HTTPS or QUIC).')
    expect(s.detail).toContain('212 names were looked up.')
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

describe('destinationLabel', () => {
  it('shows the name when a DNS answer gave one and says so when it did not', () => {
    expect(destinationLabel({ name: 'github.com', address: '1.1.1.1', bytes: 1, internet: true })).toEqual({ text: 'github.com', named: true })
    expect(destinationLabel({ address: '34.120.1.1', bytes: 1, internet: true })).toEqual({ text: '34.120.1.1', named: false })
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
  it('writes the operator and the country on one line, and the number when the holder has no name', () => {
    expect(ownerLine({ asn: 15169, name: 'GOOGLE - Google LLC', country: 'US' })).toBe('GOOGLE - Google LLC · United States')
    expect(ownerLine({ asn: 64500, name: '', country: 'DE' })).toBe('AS64500 · Germany')
    // With the table of cities: smallest place first, and a region that repeats the city is said once.
    expect(ownerLine({ asn: 15169, name: 'GOOGLE', country: 'US', city: 'Mountain View', region: 'California' })).toBe('GOOGLE · Mountain View, California, United States')
    expect(ownerLine({ asn: 1, name: 'SG', country: 'SG', city: 'Singapore', region: 'Singapore' })).toBe('SG · Singapore, Singapore')
    expect(ownerLine({ asn: 1, name: 'X', country: 'US', region: 'Texas' })).toBe('X · Texas, United States')
    expect(ownerLine({ asn: 64500, name: 'X' })).toBe('X')
    expect(ownerLine(undefined)).toBe('')
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
