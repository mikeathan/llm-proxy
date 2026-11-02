import { describe, expect, it } from 'vitest'
import { deviceAddress, deviceLabel, deviceSeen, deviceStatus, deviceSummary, filterCounts, groupDevices, intervalOptions, matchesQuery, nameableDevices, portsLine, portsSummary, presenceLine } from '../../../utils/netaudit/devices'
import type { NetworkDevice, PresenceState } from '../../../types/netaudit'

const NOW = Date.parse('2026-10-30T12:00:00Z')
const device = (over: Partial<NetworkDevice> = {}): NetworkDevice => ({
  mac: 'aa:bb:cc:00:00:01', ip: '192.168.1.5', first_seen: '2026-10-29T12:00:00Z', last_seen: '2026-10-30T11:55:00Z',
  approved: true, randomized: false, new: false, ...over,
})
const presence = (over: Partial<PresenceState> = {}): PresenceState => ({ enabled: true, interval_minutes: 10, allowed_intervals: [5, 10, 15, 30], ...over })

describe('deviceLabel', () => {
  it('prefers the name the person gave', () => expect(deviceLabel(device({ name: 'Printer', randomized: true }))).toBe('Printer'))
  it('explains a private address instead of calling it unknown', () => expect(deviceLabel(device({ randomized: true }))).toContain('private address'))
  it('says unknown for a device nobody named', () => expect(deviceLabel(device())).toBe('Unknown device'))
  it('names the maker of an unnamed device when the registry knows it', () => expect(deviceLabel(device({ vendor: 'Espressif Inc.' }))).toBe('Espressif Inc. device'))
  it('still prefers the name the person gave over the maker', () => expect(deviceLabel(device({ name: 'Plug', vendor: 'Espressif Inc.' }))).toBe('Plug'))
  it('does not use the maker for a private address, which says nothing about the card', () => expect(deviceLabel(device({ randomized: true, vendor: 'Espressif Inc.' }))).toContain('private address'))
})

describe('presenceLine', () => {
  it('is plain about being off', () => expect(presenceLine(presence({ enabled: false }), NOW)).toBe('Off.'))
  it('states the trouble and never reads it as clean', () => {
    const line = presenceLine(presence({ problem: 'this computer cannot list the devices on its network (only Linux can)' }), NOW)
    expect(line).toContain('Cannot look right now')
    expect(line).toContain('only Linux can')
  })
  it('says the first check is still to come', () => expect(presenceLine(presence(), NOW)).toContain('first check runs within 10 minutes'))
  it('says when it last looked', () => expect(presenceLine(presence({ last_check: '2026-10-30T11:58:00Z' }), NOW)).toBe('On. Last checked 2 minutes ago.'))
})

describe('a time the server never recorded', () => {
  const NEVER = '0001-01-01T00:00:00Z'
  it('is not shown as thousands of days ago', () => {
    expect(presenceLine(presence({ last_check: NEVER }), NOW)).toContain('first check runs within')
    expect(portsLine(device({ ports_scanned_at: NEVER }), NOW)).toBe('')
    expect(portsSummary(device({ ports_scanned_at: NEVER }), NOW)).toBe('')
  })
})

describe('intervalOptions', () => {
  it('offers exactly what the server allows', () => expect(intervalOptions(presence()).map((o) => o.value)).toEqual(['5', '10', '15', '30']))
})

describe('deviceSummary', () => {
  it('counts devices and the ones that need a decision', () => {
    expect(deviceSummary([device(), device({ mac: 'x', new: true })])).toBe('2 devices · 1 new')
  })
  it('says nothing is new when nothing is', () => expect(deviceSummary([device(), device({ mac: 'x' })])).toBe('2 devices'))
  it('uses the singular for one device', () => expect(deviceSummary([device()])).toBe('1 device'))
  it('is plain about an empty list', () => expect(deviceSummary([])).toBe('none yet'))
})

describe('nameableDevices', () => {
  it('lists the devices the person can name usefully: no name, a real hardware address and an IPv4 address', () => {
    const list = [
      device({ mac: 'a1' }),
      device({ mac: 'a2', name: 'Printer' }),
      device({ mac: 'a3', randomized: true }),
      device({ mac: 'a4', ip: undefined, ipv6: ['fe80::1'] }),
      device({ mac: 'a5', name: '' }),
    ]
    expect(nameableDevices(list).map((d) => d.mac)).toEqual(['a1', 'a5'])
  })
})

describe('portsLine', () => {
  it('lists the open ports from the last scan and when it was', () => {
    const d = device({ open_ports: [22, 80], ports_scanned_at: '2026-10-28T12:00:00Z' })
    expect(portsLine(d, NOW)).toBe('Open ports: 22, 80 · scanned 2 days ago')
  })
  it('says no port was open when a scan finished and found none', () => {
    expect(portsLine(device({ open_ports: [], ports_scanned_at: '2026-10-30T11:00:00Z' }), NOW)).toBe('No open ports · scanned 1 hour ago')
  })
  it('says nothing for a device no scan has finished looking at', () => {
    expect(portsLine(device(), NOW)).toBe('')
  })
})


describe('deviceAddress and deviceSeen', () => {
  it('gives the IPv4 address, and when the device was last seen', () => {
    expect(deviceAddress(device(), false)).toBe('192.168.1.5')
    expect(deviceSeen(device(), NOW)).toBe('seen 5 minutes ago')
  })
  const v6 = device({ ip: undefined, ipv6: ['fe80::1', 'fd00::2', 'fd00::3'] })
  it('shows the newest IPv6 address and counts the rest in Simple, for a device with no IPv4 address', () => expect(deviceAddress(v6, false)).toBe('fe80::1 and 2 more'))
  it('lists every IPv6 address in Advanced', () => expect(deviceAddress(v6, true)).toBe('fe80::1, fd00::2, fd00::3'))
  it('does not say "and 0 more" for one address', () => expect(deviceAddress(device({ ip: undefined, ipv6: ['fe80::1'] }), false)).toBe('fe80::1'))
  it('says so when there is no address at all', () => expect(deviceAddress(device({ ip: undefined }), false)).toBe('no address'))
})

describe('portsSummary', () => {
  it('counts the open ports without listing them', () => {
    expect(portsSummary(device({ open_ports: [22, 80], ports_scanned_at: '2026-10-28T12:00:00Z' }), NOW)).toBe('2 open ports · scanned 2 days ago')
    expect(portsSummary(device({ open_ports: [22], ports_scanned_at: '2026-10-28T12:00:00Z' }), NOW)).toBe('1 open port · scanned 2 days ago')
  })
  it('says none were open, and nothing for a device no scan has finished', () => {
    expect(portsSummary(device({ open_ports: [], ports_scanned_at: '2026-10-30T11:00:00Z' }), NOW)).toBe('No open ports · scanned 1 hour ago')
    expect(portsSummary(device(), NOW)).toBe('')
  })
})

describe('deviceStatus', () => {
  it('needs a look when the server calls the device new', () => expect(deviceStatus(device({ new: true, approved: false }), NOW)).toBe('attention'))
  it('is online when seen within half an hour', () => expect(deviceStatus(device({ last_seen: '2026-10-30T11:35:00Z' }), NOW)).toBe('online'))
  it('is offline after that', () => expect(deviceStatus(device({ last_seen: '2026-10-30T11:20:00Z' }), NOW)).toBe('offline'))
  it('leaves an old visitor nobody approved offline, not in the news', () => {
    expect(deviceStatus(device({ approved: false, new: false, last_seen: '2026-10-01T00:00:00Z' }), NOW)).toBe('offline')
  })
})

describe('matchesQuery by maker', () => {
  it('finds a device by its maker', () => expect(matchesQuery(device({ vendor: 'Espressif Inc.' }), 'espressif')).toBe(true))
})

describe('matchesQuery', () => {
  const d = device({ name: 'Living Room TV', suggested_name: 'lg-webos', ipv6: ['fe80::beef'] })
  it.each(['living', 'LG-WEBOS', '192.168.1.5', 'fe80::beef', 'aa:bb:cc:00:00:01'])('finds a device by %s', (q) => expect(matchesQuery(d, q)).toBe(true))
  it('matches everything on an empty or blank search', () => {
    expect(matchesQuery(d, '')).toBe(true)
    expect(matchesQuery(d, '   ')).toBe(true)
  })
  it('does not match what is not there', () => expect(matchesQuery(d, 'printer')).toBe(false))
})

describe('groupDevices', () => {
  const fresh = device({ mac: 'n', ip: '192.168.1.77', approved: false, new: true })
  const tv = device({ mac: 't', ip: '192.168.1.20', name: 'TV' })
  const alpha = device({ mac: 'a', ip: '192.168.1.9', name: 'Alpha' })
  const bare10 = device({ mac: 'b10', ip: '192.168.1.10' })
  const bare2 = device({ mac: 'b2', ip: '192.168.1.2' })
  const gone = device({ mac: 'g', ip: '192.168.1.30', name: 'Old laptop', last_seen: '2026-10-01T00:00:00Z' })
  const all = [gone, bare10, tv, fresh, bare2, alpha]

  it('puts what needs a look first, then online, then offline, leaving out empty groups', () => {
    expect(groupDevices(all, 'all', '', NOW).map((g) => g.status)).toEqual(['attention', 'online', 'offline'])
    expect(groupDevices([tv], 'all', '', NOW).map((g) => g.status)).toEqual(['online'])
  })

  it('lists named devices first by name, then the rest by address number', () => {
    const online = groupDevices(all, 'all', '', NOW).find((g) => g.status === 'online')!
    expect(online.devices.map((d: NetworkDevice) => d.mac)).toEqual(['a', 't', 'b2', 'b10'])
  })

  it('applies each filter', () => {
    const macs = (filter: Parameters<typeof groupDevices>[1]) => groupDevices(all, filter, '', NOW).flatMap((g) => g.devices.map((d: NetworkDevice) => d.mac)).sort()
    expect(macs('attention')).toEqual(['n'])
    expect(macs('online')).toEqual(['a', 'b10', 'b2', 't'])
    expect(macs('offline')).toEqual(['g'])
    expect(macs('unnamed')).toEqual(['b10', 'b2', 'n'])
  })

  it('combines the search with the filter', () => {
    expect(groupDevices(all, 'online', 'tv', NOW).flatMap((g) => g.devices.map((d: NetworkDevice) => d.mac))).toEqual(['t'])
    expect(groupDevices(all, 'offline', 'tv', NOW)).toEqual([])
  })

  it('never hides a new device from All, whatever its name', () => {
    expect(groupDevices(all, 'all', '', NOW)[0]!.devices.map((d: NetworkDevice) => d.mac)).toEqual(['n'])
  })
})

describe('filterCounts', () => {
  it('counts what each chip would show', () => {
    const list = [device({ mac: 'n', new: true, approved: false }), device({ mac: 't', name: 'TV' }), device({ mac: 'g', last_seen: '2026-10-01T00:00:00Z' })]
    expect(filterCounts(list, NOW)).toEqual({ all: 3, attention: 1, online: 1, offline: 1, unnamed: 2 })
  })
})
