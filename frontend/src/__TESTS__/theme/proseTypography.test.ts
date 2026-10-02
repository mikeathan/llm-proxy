import { describe, it, expect } from 'vitest'
import { typography } from '../../../theme/tailwindTokens'

// Markdown answers keep the model's own text: the typography plugin's
// decorative backticks around inline code and curly quotes around blockquotes
// doubled what the model wrote (`` `name` `` and ""quote"").
describe('prose typography', () => {
  const css = typography.DEFAULT.css as Record<string, unknown>

  it('adds no backticks around inline code', () => {
    expect(css['code::before']).toEqual({ content: 'none' })
    expect(css['code::after']).toEqual({ content: 'none' })
  })

  it('adds no quote marks around a blockquote', () => {
    expect(css['blockquote p:first-of-type::before']).toEqual({ content: 'none' })
    expect(css['blockquote p:last-of-type::after']).toEqual({ content: 'none' })
  })
})
