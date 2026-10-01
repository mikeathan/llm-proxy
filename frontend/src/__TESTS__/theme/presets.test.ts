import { describe, expect, it } from 'vitest'
import { BASE_PRESET_ID, DEFAULT_PRESET_ID, PRESETS, PRESET_IDS } from '../../theme/presets'
import { TOKEN_NAMES } from '../../theme/tokenRegistry'
import { evaluateContrast } from '../../theme/contrastPairs'
import { parseColour } from '../../theme/colour'

describe('presets (from styles/tokens.css)', () => {
  it('defaults to retro-dark and lists every preset', () => {
    expect(DEFAULT_PRESET_ID).toBe('retro-dark-soft')
    expect(BASE_PRESET_ID).toBe('retro-dark')
    expect(PRESET_IDS).toEqual(['retro-dark', 'retro-dark-soft', 'retro-dark-lifted', 'retro-paper'])
  })

  it.each(PRESET_IDS)('%s defines every registered token', (id) => {
    expect(Object.keys(PRESETS[id].tokens).sort()).toEqual([...TOKEN_NAMES].sort())
  })

  it.each(PRESET_IDS)('%s stores every colour as an opaque channel triple', (id) => {
    const tokens = PRESETS[id].tokens
    expect(parseColour(tokens.canvas)).not.toBeNull()
    expect(parseColour(tokens['text-on-accent'])).not.toBeNull()
  })

  it.each(PRESET_IDS)('%s passes every contrast pair', (id) => {
    const failures = evaluateContrast(PRESETS[id].tokens).filter((r) => !r.pass)
    expect(failures).toEqual([])
  })

  it('carries each preset scheme, and lifted inherits dark except what it lifts', () => {
    expect(PRESETS['retro-dark'].scheme).toBe('dark')
    expect(PRESETS['retro-paper'].scheme).toBe('light')
    expect(PRESETS['retro-dark-lifted'].tokens.canvas).not.toBe(PRESETS['retro-dark'].tokens.canvas)
    expect(PRESETS['retro-dark-lifted'].tokens['text-primary']).toBe(PRESETS['retro-dark'].tokens['text-primary'])
  })

  it('soft dark is a lighter, dark-scheme step above the ink, with the same cream text', () => {
    const [ink, soft, lifted] = [PRESETS['retro-dark'], PRESETS['retro-dark-soft'], PRESETS['retro-dark-lifted']]
    expect(soft.scheme).toBe('dark')
    expect(soft.label).toBe('Retro dark · soft')
    expect(soft.tokens.canvas).toBe('34 32 29')
    const lightness = (t: { canvas: string }) => t.canvas.split(' ').map(Number).reduce((a, b) => a + b, 0)
    expect(lightness(soft.tokens)).toBeGreaterThan(lightness(lifted.tokens))
    expect(lightness(lifted.tokens)).toBeGreaterThan(lightness(ink.tokens))
    expect(soft.tokens['text-primary']).toBe(ink.tokens['text-primary'])
  })
})
