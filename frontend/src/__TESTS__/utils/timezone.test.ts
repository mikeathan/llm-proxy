import { describe, it, expect } from 'vitest'
import { browserTimeZone, isKnownTimeZone } from '../../utils/format/timezone'

describe('timezone helpers', () => {
  it('accepts IANA names and rejects anything else', () => {
    expect(isKnownTimeZone('Europe/London')).toBe(true)
    expect(isKnownTimeZone('Asia/Tokyo')).toBe(true)
    expect(isKnownTimeZone('Mars/Base')).toBe(false)
    expect(isKnownTimeZone('tomorrow')).toBe(false)
  })

  it('treats an empty value as valid (unset)', () => {
    expect(isKnownTimeZone('')).toBe(true)
  })

  it("reads the browser's own zone", () => {
    expect(isKnownTimeZone(browserTimeZone())).toBe(true)
  })
})
