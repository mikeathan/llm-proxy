import { describe, expect, it } from 'vitest'
import { failingWatches, latestHeadlines } from '../../../utils/newsroom/latest'
import type { NewsroomDay, NewsroomEntry, NewsroomWatch } from '../../../types/newsroom'

const entry = (id: number, at: string): NewsroomEntry => ({ id, watch_id: 1, at, source: 's', title: `t${id}` })
const watch = (over: Partial<NewsroomWatch>): NewsroomWatch => ({ id: 1, name: 'A', interval: '1h0m0s', max_age: '72h0m0s', enabled: true, sources: [{ kind: 'feed', target: 'https://x.test/f' }], ...over })

describe('latestHeadlines', () => {
  it('mixes watches and days, newest first, with the earlier findings of a day, cut to the limit', () => {
    const days: NewsroomDay[] = [{
      day: 'd', label: 'Today', updates: 3, unread: 3,
      watches: [
        { watch_id: 1, watch: 'A', updates: 2, unread: 2, last_at: '', current: entry(2, '2026-11-20T10:00:00Z'), earlier: [entry(1, '2026-11-20T08:00:00Z')] },
        { watch_id: 2, watch: 'B', updates: 1, unread: 1, last_at: '', current: entry(3, '2026-11-20T09:00:00Z') },
      ],
    }]
    expect(latestHeadlines(days, 10).map((h) => h.entry.id)).toEqual([2, 3, 1])
    expect(latestHeadlines(days, 2).map((h) => h.watch)).toEqual(['A', 'B'])
  })
})

describe('failingWatches', () => {
  it('names an enabled watch that cannot read a source, and not a paused one', () => {
    const list = [watch({ id: 1, last_status: 'failing' }), watch({ id: 2, last_status: 'failing', enabled: false }), watch({ id: 3, last_status: 'in_feed' })]
    expect(failingWatches(list).map((t) => t.id)).toEqual([1])
  })
})

