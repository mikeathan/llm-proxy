import { describe, expect, it } from 'vitest'
import { ACCENTS, accentChannels, accentOverrides } from '../../theme/accent'
import { evaluateContrast } from '../../theme/contrastPairs'
import { PRESETS } from '../../theme/presets'
import { ACCENT_IDS, PRESET_IDS } from '../../types/theme'
import type { Channels, ResolvedTokens } from '../../types/theme'

const withAccent = (id: (typeof PRESET_IDS)[number], rgb: Channels): ResolvedTokens => ({ ...PRESETS[id].tokens, ...accentOverrides(PRESETS[id].tokens, rgb) }) as ResolvedTokens

// Colours that are hard to read on something: the extremes and the middle.
const AWKWARD: Channels[] = [[0, 0, 0], [255, 255, 255], [128, 128, 128], [0, 0, 255], [255, 255, 0], [40, 40, 40]]

describe('accentOverrides', () => {
  it('overrides only the two tokens that carry the accent', () => {
    expect(Object.keys(accentOverrides(PRESETS['retro-dark-soft'].tokens, ACCENTS.sky.rgb)).sort()).toEqual(['accent-brand', 'focus-ring'])
  })

  it.each(PRESET_IDS)('%s: every swatch passes every contrast pair', (id) => {
    for (const accent of ACCENT_IDS) {
      const failures = evaluateContrast(withAccent(id, ACCENTS[accent].rgb)).filter((r) => !r.pass)
      expect(failures, `${id} + ${accent}`).toEqual([])
    }
  })

  it.each(PRESET_IDS)('%s: any colour, even an awkward one, ends up readable', (id) => {
    for (const rgb of AWKWARD) {
      const failures = evaluateContrast(withAccent(id, rgb)).filter((r) => !r.pass)
      expect(failures, `${id} + ${rgb}`).toEqual([])
    }
  })

  it('keeps a swatch that already reads exactly as it is', () => {
    const tokens = PRESETS['retro-dark-soft'].tokens
    expect(accentOverrides(tokens, [255, 106, 61])['accent-brand']).toBe(tokens['accent-brand'])
  })

  it('moves toward white on a dark preset and toward black on a light one', () => {
    const dark = accentOverrides(PRESETS['retro-dark-soft'].tokens, [60, 20, 20])['accent-brand']!.split(' ').map(Number)
    const light = accentOverrides(PRESETS['retro-paper'].tokens, [240, 240, 120])['accent-brand']!.split(' ').map(Number)
    expect(dark[0]!).toBeGreaterThan(60)
    expect(light[0]!).toBeLessThan(240)
  })
})

describe('accentChannels', () => {
  it('reads a named swatch and a custom colour, and rejects one that is not a colour', () => {
    expect(accentChannels({ kind: 'named', id: 'sky' })).toEqual(ACCENTS.sky.rgb)
    expect(accentChannels({ kind: 'custom', hex: '#102030' })).toEqual([16, 32, 48])
    expect(accentChannels({ kind: 'custom', hex: 'not a colour' })).toBeNull()
  })
})
