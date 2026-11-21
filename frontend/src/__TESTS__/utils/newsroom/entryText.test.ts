import { describe, it, expect } from 'vitest'
import { textBeyondTitle } from '../../../utils/newsroom/entryText'

describe('textBeyondTitle', () => {
  it('is empty when the text is the title, with or without line breaks', () => {
    expect(textBeyondTitle('A B C', 'A B C')).toBe('')
    expect(textBeyondTitle('A B C D', 'A B\n\nC   D')).toBe('')
    expect(textBeyondTitle('A', undefined)).toBe('')
  })
  it('keeps only what follows a title that is the start of the text, clipped or not', () => {
    expect(textBeyondTitle('One two…', 'One two three four')).toBe('three four')
    expect(textBeyondTitle('One two', 'One two three')).toBe('three')
  })
  it('keeps unrelated text whole', () => {
    expect(textBeyondTitle('Go 1.26', 'A different line')).toBe('A different line')
  })
})
