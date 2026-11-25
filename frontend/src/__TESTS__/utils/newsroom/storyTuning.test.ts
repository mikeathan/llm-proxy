import { describe, expect, it } from 'vitest'
import { followTopic, followWatch, lessLikeThis, moreLikeThis, storyWords } from '../../../utils/newsroom/storyTuning'
import type { BriefFollowEdit, BriefTopic } from '../../../types/newsroom'

const topic: BriefTopic = { id: 1, name: 'AI', category: 'Technology', queries: ['AI chips'], region: 'en-GB', sources: [{ kind: 'feed', target: 'https://example.com/f' }], ignore: ['horoscope'] }

describe('storyTuning', () => {
  it('takes the words worth searching for from a headline, in order, without the little ones', () => {
    expect(storyWords('The Fed holds rates as inflation cools, says the Fed')).toEqual(['fed', 'holds', 'rates', 'inflation', 'cools'])
    expect(storyWords('Ελλάδα: νέα μέτρα για την ενέργεια')).toContain('ενέργεια')
  })

  it('adds a search for more like this, keeping everything else, and refuses a repeat or a full topic', () => {
    expect(moreLikeThis(topic, ['open', 'weights'])).toEqual({ name: 'AI', category: 'Technology', queries: ['AI chips', 'open weights'], region: 'en-GB', sources: topic.sources, ignore: ['horoscope'] })
    expect(moreLikeThis(topic, ['ai', 'chips'])).toBeNull()
    expect(moreLikeThis(topic, [])).toBeNull()
    expect(moreLikeThis({ ...topic, queries: Array.from({ length: 10 }, (_, i) => `q${i}`) }, ['x'])).toBeNull()
  })

  it('skips words for less like this, and refuses when nothing is new or the list would be too long', () => {
    expect(lessLikeThis(topic, ['Crypto', 'horoscope'])?.ignore).toEqual(['horoscope', 'crypto'])
    expect(lessLikeThis(topic, ['horoscope'])).toBeNull()
    expect(lessLikeThis({ ...topic, ignore: Array.from({ length: 20 }, (_, i) => `w${i}`) }, ['x'])).toBeNull()
  })

  it('turns the Follow form into a topic and a watch', () => {
    const f: BriefFollowEdit = { text: ' open-weights models ', mode: 'both', section: '', feed: 'https://example.com/feed', lookFor: ' a release ', channel: 'telegram' }
    expect(followTopic(f)).toEqual({ name: 'open-weights models', category: 'open-weights models', queries: ['open-weights models'], region: '', sources: [{ kind: 'feed', target: 'https://example.com/feed' }], ignore: [] })
    expect(followWatch(f)).toMatchObject({ name: 'open-weights models', sources: [{ kind: 'newssearch', target: 'open-weights models' }, { kind: 'feed', target: 'https://example.com/feed' }], look_for: 'a release', channel: 'telegram', enabled: true })
    expect(followTopic({ ...f, section: 'Technology', feed: '' })).toMatchObject({ category: 'Technology', sources: [] })
  })
})
