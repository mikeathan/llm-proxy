import { describe, expect, it } from 'vitest'
import { deviceDetail, deviceLabel, intervalOptions, presenceLine } from '../../../utils/netaudit/devices'
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
