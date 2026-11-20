import { describe, it, expect } from 'vitest'
import { compactInterval } from '../../../utils/newsroom/interval'

describe('compactInterval', () => {
  it.each([
    ['1h0m0s', '1h'],
    ['5m0s', '5m'],
    ['24h0m0s', '24h'],
    ['1h30m0s', '1h30m'],
    ['90s', '90s'],
  ])('%s is %s', (go, short) => {
    expect(compactInterval(go)).toBe(short)
  })

  it('leaves what it does not understand as it came', () => {
    expect(compactInterval('')).toBe('')
    expect(compactInterval('every hour')).toBe('every hour')
  })
})
