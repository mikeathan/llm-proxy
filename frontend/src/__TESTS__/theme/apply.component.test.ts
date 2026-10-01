import { describe, expect, it } from 'vitest'
import { applyTheme } from '../../theme/apply'

describe('applyTheme', () => {
  it('sets the preset attribute, dark class and custom overrides', () => {
    const root = document.createElement('html')
    applyTheme(root, { themeId: 'sea', presetId: 'retro-dark', scheme: 'dark', overrides: { 'accent-brand': '1 2 3' } })
    expect(root.dataset.theme).toBe('retro-dark')
    expect(root.classList.contains('dark')).toBe(true)
    expect(root.style.getPropertyValue('--accent-brand')).toBe('1 2 3')
  })

  it('is idempotent and clears overrides the next theme does not set', () => {
    const root = document.createElement('html')
    applyTheme(root, { themeId: 'sea', presetId: 'retro-dark', scheme: 'dark', overrides: { 'accent-brand': '1 2 3' } })
    applyTheme(root, { themeId: 'retro-paper', presetId: 'retro-paper', scheme: 'light', overrides: {} })
    applyTheme(root, { themeId: 'retro-paper', presetId: 'retro-paper', scheme: 'light', overrides: {} })
    expect(root.dataset.theme).toBe('retro-paper')
    expect(root.classList.contains('dark')).toBe(false)
    expect(root.style.getPropertyValue('--accent-brand')).toBe('')
    expect(root.getAttribute('style') ?? '').toBe('')
  })
})
