import { describe, expect, it } from 'vitest'
import { peekDraft, stashDraft } from '../../../composables/assistant/useComposerDraft'

describe('composer draft', () => {
  it('keeps a draft per workspace until it is emptied', () => {
    stashDraft('a', 'first')
    stashDraft('b', 'second')
    expect(peekDraft('a')).toBe('first')
    expect(peekDraft('b')).toBe('second')
    expect(peekDraft('a')).toBe('first')
    stashDraft('a', '   \n')
    expect(peekDraft('a')).toBe('')
    expect(peekDraft('b')).toBe('second')
    stashDraft('b', '')
  })
  it('knows nothing about a workspace it has not seen', () => {
    expect(peekDraft('never')).toBe('')
  })
})
