import { describe, expect, it } from 'vitest'
import { briefText } from '../../../utils/newsroom/briefText'
import type { BriefIssue } from '../../../types/newsroom'

describe('briefText', () => {
  it('writes the name, the top stories and each section, with real web links only and nothing from home', () => {
    const issue: BriefIssue = {
      id: 1, edition_id: 2, name: 'Morning', made_at: '2026-11-25T06:40:00Z', model_calls: 3,
      top: [{ pool_id: 1, headline: 'Lead', link: 'https://example.com/l', outlets: [] }],
      sections: [{ kind: 'news', topic_id: 7, name: 'Technology', stories: [{ pool_id: 1, headline: 'Lead', outlets: [] }, { pool_id: 2, headline: 'Bad link', link: 'javascript:alert(1)', outlets: [] }] }],
      home: [{ id: 'network_online', count: 3, words: 'Devices.' }],
      day: { items: [{ clock: '08:30', title: 'Dentist' }] },
    }
    expect(briefText(issue, 'Wednesday')).toBe('Morning · Wednesday\n\nTop stories\n• Lead\n  https://example.com/l\n\nTechnology\n• Bad link')
  })
})
