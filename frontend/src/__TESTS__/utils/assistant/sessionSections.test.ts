import { describe, expect, it } from 'vitest'
import { buildSessionSections } from '../../../utils/assistant/sessionSections'
import type { SessionBrief } from '../../../types/assistant'

// Local noon on 2026-09-30 (a Wednesday), so day boundaries are unambiguous.
const NOW = new Date(2026, 8, 30, 12, 0).getTime()
const at = (days: number, hours = 0) => new Date(2026, 8, 30 - days, 9 + hours).toISOString()
const s = (id: string, updated: string, extra: Partial<SessionBrief> = {}): SessionBrief => ({ id, snippet: `about ${id}`, updated_at: updated, ...extra })

const SESSIONS = [
  s('today', at(0)),
  s('yesterday', at(1)),
  s('week', at(4)),
  s('old', at(30)),
  s('hook', at(0), { source: 'webhook-telegram' }),
  s('pinned-old', at(60)),
]

const summary = (sections: ReturnType<typeof buildSessionSections>) => sections.map((x) => [x.label, x.sessions.map((y) => y.id)])

describe('buildSessionSections', () => {
  it('puts pins first, groups manual chats by recency, and keeps webhook chats in their own group', () => {
    expect(summary(buildSessionSections(SESSIONS, { pinned: new Set(['pinned-old']), query: '', now: NOW }))).toEqual([
      ['Pinned', ['pinned-old']],
      ['Today', ['today']],
      ['Yesterday', ['yesterday']],
      ['This week', ['week']],
      ['Older', ['old']],
      ['Webhook', ['hook']],
    ])
  })

  it('leaves out empty groups', () => {
    expect(summary(buildSessionSections([s('a', at(0))], { pinned: new Set(), query: '', now: NOW }))).toEqual([['Today', ['a']]])
  })

  it('searches titles across every group, ignoring case', () => {
    const found = buildSessionSections(SESSIONS, { pinned: new Set(['pinned-old']), query: 'ABOUT HOOK', now: NOW })
    expect(summary(found)).toEqual([['Webhook', ['hook']]])
  })

  it('files a conversation without a usable date under Older', () => {
    expect(summary(buildSessionSections([s('x', '')], { pinned: new Set(), query: '', now: NOW }))).toEqual([['Older', ['x']]])
  })

  it('marks which groups are pinned, recency or source groups', () => {
    const kinds = buildSessionSections(SESSIONS, { pinned: new Set(['pinned-old']), query: '', now: NOW }).map((x) => x.kind)
    expect(kinds).toEqual(['pinned', 'recency', 'recency', 'recency', 'recency', 'source'])
  })
})
