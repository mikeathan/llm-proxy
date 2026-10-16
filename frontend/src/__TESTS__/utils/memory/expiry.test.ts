import { describe, expect, it } from 'vitest'
import { expiryLastDay } from '../../../utils/memory/expiry'

describe('expiryLastDay', () => {
  it.each([
    ['a dated fact reads as its own last day', '2027-01-02T00:00:00Z', '2027-01-01'],
    ['a mid-day session expiry reads as its own day', '2026-10-16T12:00:00Z', '2026-10-16'],
    ['no expiry', '', ''],
    ['unreadable', 'soon', ''],
  ])('%s', (_name, input, want) => {
    expect(expiryLastDay(input)).toBe(want)
  })
  it('treats a missing value as never', () => {
    expect(expiryLastDay(undefined)).toBe('')
    expect(expiryLastDay(null)).toBe('')
  })
})
