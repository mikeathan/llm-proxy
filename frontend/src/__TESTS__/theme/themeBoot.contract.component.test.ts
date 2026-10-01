import { afterEach, describe, expect, it, vi } from 'vitest'
import bootSource from '../../../public/theme-boot.js?raw'
import { applyTheme } from '../../theme/apply'
import { PRESETS } from '../../theme/presets'
import type { AppliedTheme } from '../../types/theme'
import { memoryStorage } from '../helpers/memoryStorage'

// Contract: public/theme-boot.js (pre-paint) and theme/apply.ts (runtime) must
// leave <html> in the same state for the same stored record (plan D19).

const KEY = 'admin-ui:theme-applied'
const root = document.documentElement

function snapshot() {
  return { theme: root.dataset.theme, dark: root.classList.contains('dark'), style: root.getAttribute('style') ?? '' }
}

function reset() {
  root.removeAttribute('data-theme')
  root.removeAttribute('style')
  root.classList.remove('dark')
}

function runBoot(stored: string | null, prefersLight = false) {
  reset()
  vi.stubGlobal('localStorage', memoryStorage(stored === null ? {} : { [KEY]: stored }))
  vi.stubGlobal('matchMedia', () => ({ matches: prefersLight }))
  new Function(bootSource)()
  return snapshot()
}

function runApply(theme: AppliedTheme) {
  reset()
  applyTheme(root, theme)
  return snapshot()
}

const record = (data: unknown) => JSON.stringify({ version: 1, data })
const preset = (id: AppliedTheme['presetId']): AppliedTheme => ({ themeId: id, presetId: id, scheme: PRESETS[id].scheme, overrides: {} })

describe('theme-boot.js ≡ applyTheme', () => {
  afterEach(() => vi.unstubAllGlobals())

  it.each(Object.values(PRESETS).map((p) => p.id))('stored preset %s', (id) => {
    expect(runBoot(record(preset(id)))).toEqual(runApply(preset(id)))
  })

  it('stored custom theme with overrides', () => {
    const custom: AppliedTheme = { themeId: 'sea', presetId: 'retro-paper', scheme: 'light', overrides: { 'accent-brand': '1 2 3', 'radius-sm': '3px' } }
    expect(runBoot(record(custom))).toEqual(runApply(custom))
  })

  it('following the OS (null record) picks the light preset when the OS is light', () => {
    expect(runBoot(record(null), true)).toEqual(runApply(preset('retro-paper')))
    expect(runBoot(null, false)).toEqual(runApply(preset('retro-dark-soft')))
  })

  it.each([
    ['corrupt JSON', '{nope'],
    ['unknown preset', record({ ...preset('retro-dark'), presetId: 'solarized' })],
    ['newer version', JSON.stringify({ version: 9, data: preset('retro-paper') })],
  ])('%s falls back to the default preset', (_case, stored) => {
    expect(runBoot(stored)).toEqual(runApply(preset('retro-dark-soft')))
  })

  it('skips a malicious value but applies the valid ones', () => {
    const tampered = { ...preset('retro-dark'), overrides: { canvas: 'red; background: url(x)', 'accent-brand': '1 2 3' } }
    expect(runBoot(record(tampered))).toEqual(runApply({ ...preset('retro-dark'), overrides: { 'accent-brand': '1 2 3' } }))
  })

  it('knows the same scheme for every preset as the token sheet', () => {
    for (const p of Object.values(PRESETS)) {
      expect(runBoot(record(preset(p.id))).dark).toBe(p.scheme === 'dark')
    }
  })
})
