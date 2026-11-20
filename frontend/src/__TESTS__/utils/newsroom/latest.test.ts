import { describe, expect, it } from 'vitest'
import { failingTopics, latestHeadlines, sourceRows } from '../../../utils/newsroom/latest'
import type { NewsroomDay, NewsroomEntry, NewsroomTopic } from '../../../types/newsroom'

const entry = (id: number, at: string): NewsroomEntry => ({ id, topic_id: 1, at, source: 's', title: `t${id}` })
const topic = (over: Partial<NewsroomTopic>): NewsroomTopic => ({ id: 1, name: 'A', interval: '1h0m0s', enabled: true, sources: [{ kind: 'feed', target: 'https://x.test/f' }], ...over })

describe('latestHeadlines', () => {
  it('mixes topics and days, newest first, with the earlier findings of a day, cut to the limit', () => {
    const days: NewsroomDay[] = [{
      day: 'd', label: 'Today', updates: 3, unread: 3,
      topics: [
        { topic_id: 1, topic: 'A', updates: 2, unread: 2, last_at: '', current: entry(2, '2026-11-20T10:00:00Z'), earlier: [entry(1, '2026-11-20T08:00:00Z')] },
        { topic_id: 2, topic: 'B', updates: 1, unread: 1, last_at: '', current: entry(3, '2026-11-20T09:00:00Z') },
      ],
    }]
    expect(latestHeadlines(days, 10).map((h) => h.entry.id)).toEqual([2, 3, 1])
    expect(latestHeadlines(days, 2).map((h) => h.topic)).toEqual(['A', 'B'])
  })
})

describe('failingTopics', () => {
  it('names an enabled topic that cannot read a source, and not a paused one', () => {
    const list = [topic({ id: 1, last_status: 'failing' }), topic({ id: 2, last_status: 'failing', enabled: false }), topic({ id: 3, last_status: 'in_feed' })]
    expect(failingTopics(list).map((t) => t.id)).toEqual([1])
  })
})

describe('sourceRows', () => {
  it('lists a shared source once with every topic that reads it, failing if any live topic failed', () => {
    const rows = sourceRows([
      topic({ id: 1, name: 'A', last_status: 'in_feed' }),
      topic({ id: 2, name: 'B', last_status: 'failing', last_detail: 'timed out' }),
    ])
    expect(rows).toHaveLength(1)
    expect(rows[0]!.topics.map((t) => t.name)).toEqual(['A', 'B'])
    expect(rows[0]!.state).toBe('failing')
    expect(rows[0]!.detail).toBe('timed out')
  })

  it('says working, not checked yet and paused apart', () => {
    const rows = sourceRows([
      topic({ id: 1, sources: [{ kind: 'feed', target: 'a' }], last_status: 'nothing_new' }),
      topic({ id: 2, sources: [{ kind: 'feed', target: 'b' }] }),
      topic({ id: 3, sources: [{ kind: 'feed', target: 'c' }], enabled: false, last_status: 'failing' }),
    ])
    expect(rows.map((r) => r.state)).toEqual(['working', 'unchecked', 'paused'])
  })
})
