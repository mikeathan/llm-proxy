import { describe, expect, it } from 'vitest'
import { validateCustomTheme } from '../../theme/validate'

const valid = { id: 'custom-sea', label: 'Sea', base: 'retro-dark', overrides: { 'accent-brand': '#ff6a3d' } }

describe('validateCustomTheme', () => {
  it('accepts a valid partial and normalises colours to channels', () => {
    const result = validateCustomTheme(valid)
    expect(result.ok).toBe(true)
    if (!result.ok) return
    expect(result.theme.overrides).toEqual({ 'accent-brand': '255 106 61' })
    expect(result.contrastFailures).toEqual([])
  })

  it('normalises bare numbers for length and duration tokens', () => {
    const result = validateCustomTheme({ ...valid, overrides: { 'radius-sm': '3', 'motion-base': 150 } })
    expect(result.ok && result.theme.overrides).toEqual({ 'radius-sm': '3px', 'motion-base': '150ms' })
  })

  it.each([
    ['not an object', 'nope'],
    ['missing label', { ...valid, label: '' }],
    ['unknown base', { ...valid, base: 'solarized' }],
    ['bad id', { ...valid, id: 'has spaces' }],
    ['unknown token', { ...valid, overrides: { 'made-up': '#fff' } }],
    ['non-editable token', { ...valid, overrides: { 'font-mono': 'Comic Sans' } }],
    ['translucent colour', { ...valid, overrides: { canvas: 'rgba(0,0,0,0.5)' } }],
    ['raw CSS in a colour', { ...valid, overrides: { canvas: 'red; background: url(x)' } }],
    ['url() in a length', { ...valid, overrides: { 'radius-sm': 'url(x)' } }],
    ['out-of-range duration', { ...valid, overrides: { 'motion-fast': '99999' } }],
  ])('rejects %s', (_case, input) => {
    const result = validateCustomTheme(input)
    expect(result.ok).toBe(false)
    if (!result.ok) expect(result.errors.length).toBeGreaterThan(0)
  })

  it('reports contrast failures without rejecting the theme', () => {
    const result = validateCustomTheme({ ...valid, overrides: { 'text-primary': '#0c0b0a' } })
    expect(result.ok).toBe(true)
    if (!result.ok) return
    expect(result.contrastFailures.some((f) => f.fg === 'text-primary')).toBe(true)
  })
})
