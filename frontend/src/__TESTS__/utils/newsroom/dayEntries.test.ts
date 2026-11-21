import { describe, it, expect } from 'vitest'
import { dayEntries, unreadIds } from '../../../utils/newsroom/dayEntries'
import type { NewsroomDay, NewsroomEntry } from '../../../types/newsroom'

const e = (id: number, at: string, read = false): NewsroomEntry => ({ id, watch_id: 1, at, source: 's', title: 't' + id, read })

describe('dayEntries', () => {
  it('flattens the watches of a day into one list, newest first, and lists the unread ids', () => {
    const day: NewsroomDay = {
      day: '2026-11-21', label: 'Today', updates: 3, unread: 2,
      watches: [
        { watch_id: 1, watch: 'A', updates: 2, unread: 1, last_at: '2026-11-21T09:00:00Z', current: e(2, '2026-11-21T09:00:00Z'), earlier: [e(1, '2026-11-21T07:00:00Z', true)] },
        { watch_id: 2, watch: 'B', updates: 1, unread: 1, last_at: '2026-11-21T08:00:00Z', current: e(3, '2026-11-21T08:00:00Z') },
      ],
    }
    const list = dayEntries(day)
    expect(list.map((x) => x.id)).toEqual([2, 3, 1])
    expect(unreadIds(list)).toEqual([2, 3])
  })
})
