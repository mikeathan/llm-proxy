import { describe, expect, it } from 'vitest'
import { callsToday, chartColumns, speedOf, tokensOf, trendOf, weekTotals, weekdayOf } from '../../utils/modelUse'
import type { ModelUse } from '../../types/modelStats'

const use = (over: Partial<ModelUse> = {}): ModelUse => ({ model: 'm', local: true, calls: 1, failed: 0, prompt_tokens: 10, completion_tokens: 5, ...over })

describe('modelUse helpers', () => {
  it('adds today\'s calls and the days into a trend', () => {
    expect(callsToday({ days: [], today: [use({ calls: 3 }), use({ calls: 4, local: false })] })).toBe(7)
    expect(trendOf([{ date: '2026-11-23', local: 2, cloud: 1 }, { date: '2026-11-24', local: 0, cloud: 5 }])).toEqual([3, 5])
  })

  it('draws the tallest day full and the others in proportion, split into local and cloud', () => {
    const cols = chartColumns([{ date: 'a', local: 6, cloud: 2 }, { date: 'b', local: 2, cloud: 2 }, { date: 'c', local: 0, cloud: 0 }])
    expect(cols.map((c) => c.heightPercent)).toEqual([100, 50, 0])
    expect(cols[0]).toMatchObject({ localPercent: 75, cloudPercent: 25, total: 8 })
    expect(cols[2]).toMatchObject({ localPercent: 0, cloudPercent: 0 })
    expect(chartColumns([]).length).toBe(0)
  })

  it('reads a day as a day, never as a moment that can slip to the day before', () => {
    expect(weekdayOf('2026-11-24')).toBe(new Date(2026, 10, 24).toLocaleDateString(undefined, { weekday: 'short' }))
    expect(weekdayOf('not a date')).toBe('not a date')
  })

  it('shows tokens in and out, a local speed, and a dash for a model that reports none', () => {
    expect(tokensOf(use({ prompt_tokens: 100, completion_tokens: 20 }))).toBe(120)
    expect(speedOf(use({ tokens_per_sec: 41.4 }))).toBe('41 tok/s')
    expect(speedOf(use({ tokens_per_sec: undefined }))).toBe('—')
    expect(weekTotals([{ date: 'a', local: 1, cloud: 2 }, { date: 'b', local: 3, cloud: 4 }])).toEqual({ local: 4, cloud: 6 })
  })
})
