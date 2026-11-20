import { describe, expect, it } from 'vitest'
import { channelName, sourceProblem } from '../../../utils/newsroom/sourceCheck'
import type { NewsroomSource } from '../../../types/newsroom'

const p = (s: NewsroomSource) => sourceProblem(s)

describe('sourceProblem', () => {
  it('accepts a pasted Telegram link and refuses a short name', () => {
    expect(channelName('https://t.me/s/aipost')).toBe('aipost')
    expect(p({ kind: 'telegram', target: 'https://t.me/s/aipost' })).toBe('')
    expect(p({ kind: 'telegram', target: 'ab' })).not.toBe('')
  })
  it('needs a full https address for a feed or a page', () => {
    expect(p({ kind: 'feed', target: 'http://x.example/feed' })).not.toBe('')
    expect(p({ kind: 'page', target: 'https://x.example/' })).toBe('')
  })
  it('checks the Mastodon server and tag together, accepting a leading #', () => {
    expect(p({ kind: 'mastodon', target: '#golang', instance: 'Mastodon.social' })).toBe('')
    expect(p({ kind: 'mastodon', target: 'go lang', instance: 'mastodon.social' })).not.toBe('')
    expect(p({ kind: 'mastodon', target: 'golang', instance: 'nothost' })).not.toBe('')
    expect(p({ kind: 'mastodon', target: 'golang' })).not.toBe('')
  })
  it('says nothing about an empty field', () => {
    expect(p({ kind: 'feed', target: '' })).toBe('')
  })
})
