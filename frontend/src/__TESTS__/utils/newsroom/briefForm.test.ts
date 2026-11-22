import { describe, expect, it } from 'vitest'
import { editionEdit, fromLines, fromWords, hoursOf, topicEdit, topicFields } from '../../../utils/newsroom/briefForm'
import type { BriefTopic } from '../../../types/newsroom'

describe('briefForm', () => {
  it('reads one value per line and per comma, dropping blanks and repeats', () => {
    expect(fromLines(' AI chips \n\nAI chips\nsite:reuters.com ')).toEqual(['AI chips', 'site:reuters.com'])
    expect(fromWords('Horoscope, , sport,HOROSCOPE')).toEqual(['horoscope', 'sport'])
  })

  it('reads whole hours of the durations the server sends, and nothing else', () => {
    expect(hoursOf('2h0m0s')).toBe(2)
    expect(hoursOf('12h')).toBe(12)
    expect(hoursOf('90m')).toBeNull()
    expect(hoursOf('')).toBeNull()
  })

  it('keeps a topic\'s other kinds of source when the form saves only searches and feeds', () => {
    const was: BriefTopic = { id: 3, name: 'AI', queries: ['AI chips'], sources: [{ kind: 'feed', target: 'https://a.example/f' }, { kind: 'hackernews', target: 'ai' }] }
    const fields = topicFields(was)
    expect(fields.feeds).toBe('https://a.example/f')
    fields.feeds = 'https://b.example/f'
    expect(topicEdit(fields, was).sources).toEqual([{ kind: 'feed', target: 'https://b.example/f' }, { kind: 'hackernews', target: 'ai' }])
  })

  it('sends an edition with durations as strings and sections in the order chosen', () => {
    expect(editionEdit({ name: ' Morning ', readyBy: '07:00', days: [1, 2], topics: [5, 4], enabled: true, every: 2, forHours: 12 })).toEqual({
      name: 'Morning', ready_by: '07:00', days: [1, 2], enabled: true, collect_every: '2h', collect_for: '12h',
      sections: [{ kind: 'news', topic_id: 5 }, { kind: 'news', topic_id: 4 }],
    })
  })
})
