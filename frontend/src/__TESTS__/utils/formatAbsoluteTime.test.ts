import { describe, it, expect } from 'vitest'
import { formatAbsoluteTime } from '../../utils/format/time'

describe('formatAbsoluteTime', () => {
  it('formats in the given IANA zone', () => {
    expect(formatAbsoluteTime('2026-10-12T08:00:00Z', 'en-GB', 'Europe/London')).toContain('09:00')
  })

  it('does not throw for a zone the browser does not know', () => {
    expect(() => formatAbsoluteTime('2026-10-12T08:00:00Z', 'en-GB', 'Local')).not.toThrow()
    expect(formatAbsoluteTime('2026-10-12T08:00:00Z', 'en-GB', 'Local')).toMatch(/2026/)
  })
})
