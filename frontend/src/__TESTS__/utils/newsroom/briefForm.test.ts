import { describe, expect, it } from 'vitest'
import { editionEdit, fromLines, fromWords, groupSources, hoursOf, topicEdit, topicFields } from '../../../utils/newsroom/briefForm'
import type { BriefSource, BriefTopic } from '../../../types/newsroom'

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
    expect(fields.feeds).toEqual(['https://a.example/f'])
    expect(fields.searches).toEqual(['AI chips'])
    fields.feeds = ['https://b.example/f']
    expect(topicEdit(fields, was).sources).toEqual([{ kind: 'feed', target: 'https://b.example/f' }, { kind: 'hackernews', target: 'ai' }])
  })

  it('names a feed by the list entry it was picked from, else by the name it already had', () => {
    const was: BriefTopic = { id: 3, name: 'AI', queries: ['AI'], sources: [{ kind: 'feed', label: 'My blog', target: 'https://a.example/f' }] }
    const fields = topicFields(was)
    fields.feeds = ['https://a.example/f', 'https://bbc.example/f', ' https://bbc.example/f ', 'https://plain.example/f']
    expect(topicEdit(fields, was, { 'https://bbc.example/f': 'BBC News' }).sources).toEqual([
      { kind: 'feed', label: 'My blog', target: 'https://a.example/f' }, { kind: 'feed', label: 'BBC News', target: 'https://bbc.example/f' }, { kind: 'feed', target: 'https://plain.example/f' },
    ])
  })

  it('groups the sources with the group that matches the topic first and the owner\'s own last', () => {
    const e = (category: string, label: string, builtin = true): BriefSource => ({ label, category, kind: 'feed', target: label, builtin })
    const entries = [e('News', 'BBC'), e('Technology', 'Verge'), e('Business and finance', 'FT'), e('My sources', 'Blog', false)]
    const order = ['News', 'Technology', 'Business and finance']
    expect(groupSources(entries, 'Financial', order).map((g) => g.category)).toEqual(['Business and finance', 'News', 'Technology', 'My sources'])
    expect(groupSources(entries, 'World', order).map((g) => g.category)).toEqual(['News', 'Technology', 'Business and finance', 'My sources'])
    expect(groupSources([e('News', 'BBC')], 'Tech', order).map((g) => g.category)).toEqual(['News'])
  })

  it('keeps a topic\'s own category although the form has no field for it', () => {
    const was: BriefTopic = { id: 3, name: 'AI', category: 'Technology', queries: ['AI'] }
    expect(topicEdit(topicFields(was), was).category).toBe('Technology')
  })

  it('sends an edition with durations as strings and sections in the order chosen', () => {
    expect(editionEdit({ name: ' Morning ', readyBy: '07:00', days: [1, 2], topics: [5, 4], enabled: true, every: 2, forHours: 12 })).toEqual({
      name: 'Morning', ready_by: '07:00', days: [1, 2], enabled: true, collect_every: '2h', collect_for: '12h',
      sections: [{ kind: 'news', topic_id: 5 }, { kind: 'news', topic_id: 4 }],
    })
  })
})
