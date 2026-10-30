import { describe, expect, it } from 'vitest'
import { deviceDetail, deviceLabel, devicesIn, deviceSummary, familyOptions, intervalOptions, ipv6Detail, presenceLine } from '../../../utils/netaudit/devices'
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
})

describe('deviceDetail', () => {
  it('gives the address and when it was last seen', () => expect(deviceDetail(device(), NOW)).toBe('192.168.1.5 · seen 5 minutes ago'))
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

describe('intervalOptions', () => {
  it('offers exactly what the server allows', () => expect(intervalOptions(presence()).map((o) => o.value)).toEqual(['5', '10', '15', '30']))
})

describe('deviceDetail for a device seen only over IPv6', () => {
  it('shows its IPv6 address instead of an empty one', () => {
    const d = device({ ip: undefined, ipv6: ['fe80::9'], approved: false, new: true })
    expect(deviceDetail(d, NOW)).toBe('fe80::9 · seen 5 minutes ago')
  })
})

describe('devicesIn', () => {
  const both = device({ mac: 'aa:bb:cc:00:00:01', ipv6: ['fe80::1'] })
  const v4only = device({ mac: 'aa:bb:cc:00:00:02' })
  const v6only = device({ mac: 'aa:bb:cc:00:00:03', ip: undefined, ipv6: ['fe80::3'] })
  const all = [both, v4only, v6only]

  it('puts a device on the IPv4 tab when it has an IPv4 address', () => expect(devicesIn(all, 'ipv4').map((d) => d.mac)).toEqual([both.mac, v4only.mac]))
  it('puts a device on the IPv6 tab when it has an IPv6 address, so a dual-stack device is on both', () => expect(devicesIn(all, 'ipv6').map((d) => d.mac)).toEqual([both.mac, v6only.mac]))
})

describe('deviceSummary', () => {
  it('counts devices and the ones that need a decision', () => {
    expect(deviceSummary([device(), device({ mac: 'x', new: true })])).toBe('2 devices · 1 new')
  })
  it('says nothing is new when nothing is', () => expect(deviceSummary([device(), device({ mac: 'x' })])).toBe('2 devices'))
  it('uses the singular for one device', () => expect(deviceSummary([device()])).toBe('1 device'))
  it('is plain about an empty list', () => expect(deviceSummary([])).toBe('none yet'))
})

describe('familyOptions', () => {
  it('shows how many devices each tab holds', () => {
    const labels = familyOptions([device({ ipv6: ['fe80::1'] }), device({ mac: 'x' })]).map((o) => o.label)
    expect(labels).toEqual(['IPv4 (2)', 'IPv6 (1)'])
  })
})

describe('ipv6Detail', () => {
  const d = device({ ip: undefined, ipv6: ['fe80::1', 'fd00::2', 'fd00::3'] })
  it('shows the newest address and counts the rest in Simple', () => expect(ipv6Detail(d, false, NOW)).toBe('fe80::1 and 2 more · seen 5 minutes ago'))
  it('lists every address in Advanced', () => expect(ipv6Detail(d, true, NOW)).toBe('fe80::1, fd00::2, fd00::3 · seen 5 minutes ago'))
  it('does not say "and 0 more" for one address', () => expect(ipv6Detail(device({ ipv6: ['fe80::1'] }), false, NOW)).toBe('fe80::1 · seen 5 minutes ago'))
})
