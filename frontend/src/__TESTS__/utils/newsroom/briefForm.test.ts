import { describe, expect, it } from 'vitest'
import { editionEdit, fromLines, fromWords, groupSources, hoursOf, topicEdit, topicFields, editionBlocks, editionJoining, editionSectionKeys, quickTopic, sectionGroups, topicIdsOf, topicReads } from '../../../utils/newsroom/briefForm'
import type { BriefSource, BriefTopic, BriefEdition } from '../../../types/newsroom'

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

  it('edits the section a topic belongs to, trimmed', () => {
    const was: BriefTopic = { id: 3, name: 'AI', category: 'Technology', queries: ['AI'] }
    const fields = topicFields(was)
    expect(fields.category).toBe('Technology')
    expect(topicEdit({ ...fields, category: '  Sport ' }, was).category).toBe('Sport')
  })

  it('sends an edition with durations as strings and sections in the order chosen', () => {
    expect(editionEdit({ name: ' Morning ', readyBy: '07:00', days: [1, 2], topics: [5, 4], enabled: true, every: 2, forHours: 12, watches: true, home: true })).toEqual({
      name: 'Morning', ready_by: '07:00', days: [1, 2], enabled: true, watches: true, home: true, collect_every: '2h', collect_for: '12h',
      sections: [{ kind: 'news', topic_id: 5 }, { kind: 'news', topic_id: 4 }],
    })
  })

  describe('sections', () => {
    const t = (id: number, name: string, category?: string): BriefTopic => ({ id, name, category, queries: [name] })
    const topics = [t(1, 'UK', 'News'), t(2, 'AI', 'Technology'), t(3, 'Greece', ' news '), t(4, 'Loner')]

    it('groups topics by section, spacing and case ignored, a topic with none being a section of its own', () => {
      const g = sectionGroups(topics)
      expect(g.map((x) => [x.label, x.topics.map((y) => y.id), x.own])).toEqual([['News', [1, 3], false], ['Technology', [2], false], ['Loner', [4], true]])
    })

    it('says which sections an edition reads, and turns picked sections back into topic ids in section order', () => {
      const g = sectionGroups(topics)
      const e: BriefEdition = { id: 1, name: 'M', ready_by: '07:00', days: [1], collect_every: '2h0m0s', collect_for: '12h0m0s', sections: [{ kind: 'news', topic_id: 2 }, { kind: 'news', topic_id: 1 }], enabled: true, home: true }
      expect(editionSectionKeys(e, g)).toEqual([g[0]!.key, g[1]!.key])
      expect(topicIdsOf([g[1]!.key, g[0]!.key], g)).toEqual([1, 3, 2])
      expect(editionBlocks(e, g)).toEqual(['News', 'Technology', 'Home'])
    })

    it('adds a topic to an edition that already reads its section, and to no other', () => {
      const e: BriefEdition = { id: 1, name: 'M', ready_by: '07:00', days: [1], collect_every: '2h0m0s', collect_for: '12h0m0s', sections: [{ kind: 'news', topic_id: 1 }], enabled: true }
      expect(editionJoining(e, topics, t(5, 'World', 'News'))?.sections).toEqual([{ kind: 'news', topic_id: 1 }, { kind: 'news', topic_id: 5 }])
      expect(editionJoining(e, topics, t(6, 'Chips', 'Technology'))).toBeNull()
      expect(editionJoining(e, topics, t(7, 'Alone'))).toBeNull()
      expect(editionJoining({ ...e, sections: Array.from({ length: 12 }, (_, i) => ({ kind: 'news' as const, topic_id: i + 100 })) }, [...topics, t(100, 'X', 'News')], t(5, 'World', 'News'))).toBeNull()
    })

    it('makes a topic from one typed word in a section, and lists what it reads in plain words', () => {
      expect(quickTopic(' Greece ', 'News')).toEqual({ name: 'Greece', category: 'News', queries: ['Greece'], region: '', sources: [], ignore: [] })
      expect(topicReads({ id: 1, name: 'x', sources: [{ kind: 'feed', label: 'BBC UK', target: 'https://b.example/f' }, { kind: 'hackernews', target: 'rust' }] })).toEqual(['BBC UK', 'rust'])
    })
  })
})
