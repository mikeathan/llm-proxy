import { describe, expect, it } from 'vitest'
import { sessionLengthProblem } from '../../../utils/auth/sessionLength'
import type { SessionLength } from '../../../types/auth'

const bounds: SessionLength = { idle_hours: 24, absolute_days: 14, max_idle_hours: 720, max_absolute_days: 90, default_idle_hours: 24, default_absolute_days: 14 }

describe('sessionLengthProblem', () => {
  it('accepts the defaults, the smallest and the largest', () => {
    for (const [idle, days] of [[24, 14], [1, 1], [720, 90]] as const) expect(sessionLengthProblem(idle, days, bounds)).toBe('')
  })
  it.each([
    [0, 14, /idle limit must be a whole number/],
    [721, 90, /idle limit must be a whole number/],
    [1.5, 14, /idle limit must be a whole number/],
    [NaN, 14, /idle limit must be a whole number/],
    [24, 0, /absolute limit must be a whole number/],
    [24, 91, /absolute limit must be a whole number/],
    [49, 2, /cannot be longer/],
  ])('refuses %s hours / %s days', (idle, days, message) => {
    expect(sessionLengthProblem(idle, days, bounds)).toMatch(message)
  })
})
