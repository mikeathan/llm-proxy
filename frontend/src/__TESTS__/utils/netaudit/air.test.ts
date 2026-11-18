import { describe, expect, it } from 'vitest'
import { airPhrase } from '../../../utils/netaudit/air'
import type { DeviceAir } from '../../../types/netaudit'

const NOW = Date.parse('2026-11-18T12:00:00Z')
const heard: DeviceAir = { heard: '2026-11-18T11:58:00Z', ring: 'house', band: '5G', moving: true, on_your_wifi: true, kind: 'phone' }

describe('airPhrase', () => {
  it('says when and where in Simple', () => {
    const p = airPhrase(heard, 'listening', false, NOW)
    expect(p).toMatch(/^heard over the air .+, in the house$/)
    expect(p).not.toContain('GHz')
  })
  it('adds the band, movement, your Wi-Fi and the kind in Advanced', () => {
    const p = airPhrase(heard, 'listening', true, NOW)
    for (const want of ['5 GHz', 'moving', 'on your Wi-Fi', 'Radio thinks: Phone or tablet']) expect(p).toContain(want)
  })
  it('says a device was not heard lately only while Radio listened', () => {
    expect(airPhrase({ kind: 'phone' }, 'listening', false, NOW)).toBe('not heard over the air lately')
    expect(airPhrase({ kind: 'phone' }, 'not_listening', false, NOW)).toBe('')
  })
  it('says nothing while Radio is off, unreadable or has nothing for the device', () => {
    expect(airPhrase(heard, 'off', true, NOW)).toBe('')
    expect(airPhrase(heard, 'unreadable', true, NOW)).toBe('')
    expect(airPhrase(heard, undefined, true, NOW)).toBe('')
    expect(airPhrase(undefined, 'listening', true, NOW)).toBe('')
  })
  it('calls the outer rings only far, as Radio does', () => {
    expect(airPhrase({ heard: heard.heard, ring: 'nearby' }, 'listening', false, NOW)).toContain('far from this computer')
  })
})
