import { describe, expect, it } from 'vitest'
import { PRESETS } from '../../theme/presets'
import { channelsToHex, parseColour } from '../../theme/colour'
import {
  EDITABLE_TOKENS,
  draftFromPreset,
  draftFromTheme,
  draftToInput,
  previewOverrides,
  tokenErrors,
  uniqueThemeId,
} from '../../theme/themeDraft'
import { validateCustomTheme } from '../../theme/validate'
import { TOKEN_REGISTRY } from '../../theme/tokenRegistry'

const hexOf = (stored: string) => channelsToHex(parseColour(stored)!)

describe('theme drafts', () => {
  it('edits exactly the tokens the registry marks editable', () => {
    expect(EDITABLE_TOKENS.length).toBeGreaterThan(0)
    expect(EDITABLE_TOKENS.every((t) => TOKEN_REGISTRY[t].editable)).toBe(true)
    expect(EDITABLE_TOKENS).not.toContain('font-sans')
    expect(EDITABLE_TOKENS).toContain('canvas')
  })

  it('starts a duplicate from the preset, colours as hex', () => {
    const draft = draftFromPreset('retro-paper', 'Paper copy', [])
    expect(draft).toMatchObject({ id: 'paper-copy', label: 'Paper copy', base: 'retro-paper' })
    expect(draft.values.canvas).toBe(hexOf(PRESETS['retro-paper'].tokens.canvas))
    expect(Object.keys(draft.values)).toHaveLength(EDITABLE_TOKENS.length)
  })

  it('saves only what differs from the base preset', () => {
    const draft = draftFromPreset('retro-dark', 'Sea', [])
    expect(draftToInput(draft).overrides).toEqual({})
    draft.values['accent-brand'] = 'rgb(255, 160, 122)'
    draft.values['radius-md'] = '6'
    expect(draftToInput(draft).overrides).toEqual({ 'accent-brand': 'rgb(255, 160, 122)', 'radius-md': '6' })
  })

  it('round-trips a saved theme through a draft', () => {
    const saved = validateCustomTheme({ id: 'sea', label: 'Sea', base: 'retro-dark', overrides: { 'accent-brand': '#ffa07a' } })
    if (!saved.ok) throw new Error('fixture invalid')
    const draft = draftFromTheme(saved.theme)
    expect(draft.values['accent-brand']).toBe('#ffa07a')
    const again = validateCustomTheme(draftToInput(draft))
    expect(again.ok && again.theme).toEqual(saved.theme)
  })

  it('names each invalid token, and previews only valid values', () => {
    const draft = draftFromPreset('retro-dark', 'Broken', [])
    draft.values.canvas = 'url(https://evil)'
    draft.values['accent-brand'] = '#ff0000'
    const result = validateCustomTheme(draftToInput(draft))
    expect(result.ok).toBe(false)
    const errors = tokenErrors(result.ok ? [] : result.errors)
    expect(Object.keys(errors)).toEqual(['canvas'])
    const preview = previewOverrides(draftToInput(draft))
    expect(preview.canvas).toBeUndefined()
    expect(preview['accent-brand']).toBe('255 0 0')
  })

  it('makes a unique, valid id from a label', () => {
    expect(uniqueThemeId('Amber Terminal!', [])).toBe('amber-terminal')
    expect(uniqueThemeId('Amber terminal', ['amber-terminal'])).toBe('amber-terminal-2')
    expect(uniqueThemeId('Retro dark', [])).toBe('retro-dark-2')
    expect(uniqueThemeId('%%%', [])).toBe('theme')
    expect(uniqueThemeId('x'.repeat(80), [])).toHaveLength(40)
  })
})
