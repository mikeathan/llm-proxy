import { afterEach, describe, expect, it, vi } from 'vitest'
import { motionScroll } from '../../utils/motion'

const reduce = (matches: boolean) => vi.stubGlobal('matchMedia', (q: string) => ({ matches: matches && q === '(prefers-reduced-motion: reduce)' }))

describe('motionScroll', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('turns smooth scrolling into a jump for users who ask for reduced motion', () => {
    reduce(true)
    expect(motionScroll('smooth')).toBe('auto')
  })

  it('keeps smooth scrolling otherwise, and never changes an explicit jump', () => {
    reduce(false)
    expect(motionScroll('smooth')).toBe('smooth')
    reduce(true)
    expect(motionScroll('instant')).toBe('instant')
  })

  it('keeps the requested behaviour where matchMedia is unavailable', () => {
    vi.stubGlobal('matchMedia', undefined)
    expect(motionScroll('smooth')).toBe('smooth')
  })
})
