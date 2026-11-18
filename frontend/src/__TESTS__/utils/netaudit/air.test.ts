import { describe, expect, it } from 'vitest'
import { airWords } from '../../../utils/netaudit/air'
import type { DeviceAir } from '../../../types/netaudit'

const NOW = Date.parse('2026-11-18T12:00:00Z')
const heard: DeviceAir = { heard: '2026-11-18T11:58:00Z', ring: 'house', band: '5G', moving: true, on_your_wifi: true, kind: 'phone' }
const phrase = (w: ReturnType<typeof airWords>) => (w ? [w.heard, w.rest].filter(Boolean).join(', ') : '')

describe('airWords', () => {
  it('says when and where in Simple', () => {
    const p = phrase(airWords(heard, 'listening', false, undefined, NOW))
    expect(p).toMatch(/^heard over the air .+, in the house$/)
    expect(p).not.toContain('GHz')
  })
  it('adds the band, movement, your Wi-Fi and the kind in Advanced', () => {
    const p = phrase(airWords(heard, 'listening', true, undefined, NOW))
    for (const want of ['5 GHz', 'moving', 'on your Wi-Fi', 'Radio thinks: Phone or tablet']) expect(p).toContain(want)
  })
  // vertex 2026-11-18: every device heard carried the cycle's own time, and "3 minutes ago" beside "Online" read as if the phone had left.
  it("says a device heard in Radio's latest cycle was heard in its last listen, with no time", () => {
    const w = airWords(heard, 'listening', false, heard.heard, NOW)
    expect(w?.heard).toBe("heard over the air in Radio's last listen")
    expect(w?.rest).toBe('in the house')
  })
  it('keeps the time for a device heard in an earlier cycle, or when the cycle is not known', () => {
    expect(airWords(heard, 'listening', false, '2026-11-18T11:59:00Z', NOW)?.heard).toMatch(/^heard over the air \d+ minutes? ago$/)
    expect(airWords(heard, 'listening', false, 'not a time', NOW)?.heard).not.toContain('last listen')
  })
  it('says a device was not heard lately only while Radio listened', () => {
    expect(phrase(airWords({ kind: 'phone' }, 'listening', false, undefined, NOW))).toBe('not heard over the air lately')
    expect(airWords({ kind: 'phone' }, 'not_listening', false, undefined, NOW)).toBeNull()
  })
  it('says nothing while Radio is off, unreadable or has nothing for the device', () => {
    expect(airWords(heard, 'off', true, undefined, NOW)).toBeNull()
    expect(airWords(heard, 'unreadable', true, undefined, NOW)).toBeNull()
    expect(airWords(heard, undefined, true, undefined, NOW)).toBeNull()
    expect(airWords(undefined, 'listening', true, undefined, NOW)).toBeNull()
  })
  it('calls the outer rings only far, as Radio does', () => {
    expect(phrase(airWords({ heard: heard.heard, ring: 'nearby' }, 'listening', false, undefined, NOW))).toContain('far from this computer')
  })
})
