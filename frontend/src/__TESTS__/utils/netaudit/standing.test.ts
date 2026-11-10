import { describe, expect, it } from 'vitest'
import { ageSentence, trendSentence } from '../../../utils/netaudit/standing'
import type { NetworkStanding } from '../../../types/netaudit'

const standing = (over: Partial<NetworkStanding> = {}): NetworkStanding => ({
  tracked: 3, oldest_days: 90, median_days: 10, over_month: 1, came_back: 1, week: { opened: 2, resolved: 3 }, previous: { opened: 0, resolved: 0 },
  trend: 'better', history_since: '2026-10-01T03:00:00Z', enough: true, ...over,
})

describe('standing sentences', () => {
  it('says how long the open problems have lasted, and that some came back', () => {
    expect(ageSentence(standing())).toBe('3 open problems have been there for 10 days in the middle, the oldest for 90 days. 1 has lasted over a month. 1 came back after going away.')
    expect(ageSentence(standing({ tracked: 0 }))).toBe('No problem is open that has a history.')
  })

  it('compares the last week with nothing guessed: no arrow words while the history is short', () => {
    expect(trendSentence(standing())).toBe('In the last 7 days 2 problems appeared and 3 went away. Better: more problems went away than appeared.')
    expect(trendSentence(standing({ trend: 'unknown', enough: false }))).toContain('less than two weeks of history')
    expect(trendSentence(standing({ history_since: undefined, trend: 'unknown' }))).toContain('no history yet')
  })
})
