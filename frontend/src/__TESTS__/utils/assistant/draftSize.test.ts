import { describe, expect, it } from 'vitest'
import { formatDraftSize } from '../../../utils/assistant/draftSize'

describe('formatDraftSize', () => {
  it('counts lines and characters', () => {
    expect(formatDraftSize('one\ntwo\nthree')).toBe('3 lines · 13 characters')
  })

  it('uses the singular for one line and one character', () => {
    expect(formatDraftSize('x')).toBe('1 line · 1 character')
  })

  it('compacts large counts', () => {
    expect(formatDraftSize('a'.repeat(1500))).toBe('1 line · 1.5K characters')
  })

  it('treats an empty draft as one empty line', () => {
    expect(formatDraftSize('')).toBe('1 line · 0 characters')
  })
})
