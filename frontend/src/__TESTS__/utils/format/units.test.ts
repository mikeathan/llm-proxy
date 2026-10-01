import { describe, it, expect } from 'vitest'
import { formatContrastRatio, formatCost, formatMemoryUsage, formatPercent, formatTokenCount, formatTokenRate } from '../../../utils/format/units'
import { asUtc, formatAbsoluteTime, formatDuration, formatRelativeTime } from '../../../utils/format/time'

describe('D24 formatters', () => {
  it.each([
    [0, '0'],
    [999, '999'],
    [1_500, '1.5K'],
    [334_100_000, '334.1M'],
    [2_000_000_000, '2.0B'],
  ])('formatTokenCount(%d) → %s', (n, out) => {
    expect(formatTokenCount(n)).toBe(out)
  })

  it.each([
    [0.00123, '$0.00123'],
    [0.5, '$0.5000'],
    [12.3456789, '$12.3457'],
  ])('formatCost(%d) → %s', (usd, out) => {
    expect(formatCost(usd)).toBe(out)
  })

  it.each([
    [8403, '8.403s'],
    [59_999, '59.999s'],
    [125_000, '2m 5s'],
    [11_220_000, '3h 7m'],
  ])('formatDuration(%d ms) → %s', (ms, out) => {
    expect(formatDuration(ms)).toBe(out)
  })

  it('formats relative time in the user locale, against a given now', () => {
    const now = Date.parse('2026-09-28T12:00:00Z')
    expect(formatRelativeTime('2026-09-28T11:55:00Z', now, 'en')).toBe('5 minutes ago')
    expect(formatRelativeTime('2026-09-28T11:59:58Z', now, 'en')).toBe('now')
    expect(formatRelativeTime('2026-09-26T12:00:00Z', now, 'en')).toBe('2 days ago')
  })

  it('formats absolute time with date and time in the given zone', () => {
    expect(formatAbsoluteTime('2026-09-28T12:00:00Z', 'en-GB', 'UTC')).toBe('28 Sept 2026, 12:00')
  })

  it('reads zone-less stored timestamps as UTC, leaving zoned ones alone', () => {
    expect(asUtc('2026-09-29 10:00:00')).toBe('2026-09-29 10:00:00Z')
    expect(asUtc('2026-09-29T10:00:00Z')).toBe('2026-09-29T10:00:00Z')
    expect(asUtc('2026-09-29T10:00:00+03:00')).toBe('2026-09-29T10:00:00+03:00')
  })
})

describe('formatContrastRatio', () => {
  it('shows two decimals and the ratio sign', () => {
    expect(formatContrastRatio(4.5)).toBe('4.50:1')
    expect(formatContrastRatio(21)).toBe('21.00:1')
    expect(formatContrastRatio(1.3149)).toBe('1.31:1')
  })
})

describe('host stat formatters', () => {
  it('formatPercent keeps one decimal and the sign', () => {
    expect(formatPercent(0.2)).toBe('0.2%')
    expect(formatPercent(14.27)).toBe('14.3%')
    expect(formatPercent(100)).toBe('100.0%')
  })

  it('formatTokenRate keeps one decimal', () => {
    expect(formatTokenRate(33.24)).toBe('33.2')
    expect(formatTokenRate(0)).toBe('0.0')
  })

  it('formatMemoryUsage shows used / total in GB', () => {
    expect(formatMemoryUsage(7475.2, 47923.2)).toBe('7.3 / 46.8 GB')
    expect(formatMemoryUsage(16281.6, 16384)).toBe('15.9 / 16.0 GB')
  })
})
