import { describe, expect, it } from 'vitest'
import { nextTick } from 'vue'
import { createThemeController } from '../../../composables/ui/useTheme'
import { memoryStorage } from '../../helpers/memoryStorage'

function fakeMatchMedia(prefersLight: boolean) {
  const listeners = new Set<() => void>()
  const mql = {
    get matches() {
      return prefersLight
    },
    addEventListener: (_: string, fn: () => void) => listeners.add(fn),
    removeEventListener: (_: string, fn: () => void) => listeners.delete(fn),
  }
  return {
    matchMedia: () => mql as unknown as MediaQueryList,
    setLight(value: boolean) {
      prefersLight = value
      listeners.forEach((fn) => fn())
    },
    listenerCount: () => listeners.size,
  }
}

function setup(prefersLight = false, storage = memoryStorage()) {
  const media = fakeMatchMedia(prefersLight)
  const root = document.createElement('html')
  const theme = createThemeController({ storage, root, matchMedia: media.matchMedia })
  return { theme, root, media, storage }
}

const sea = { id: 'sea', label: 'Sea', base: 'retro-dark', overrides: { 'accent-brand': '#ffa07a' } }

describe('theme controller', () => {
  it('follows the OS scheme until the user chooses, live-updating', async () => {
    const { theme, root, media } = setup(true)
    theme.start()
    expect(root.dataset.theme).toBe('retro-paper')
    media.setLight(false)
    await nextTick()
    expect(root.dataset.theme).toBe('retro-dark-soft')
    expect(media.listenerCount()).toBe(1)
  })

  it('stops listening to the OS once a preset is chosen, and persists the choice', async () => {
    const { theme, root, media, storage } = setup(true)
    theme.start()
    theme.selectPreset('retro-dark-lifted')
    await nextTick()
    expect(root.dataset.theme).toBe('retro-dark-lifted')
    expect(media.listenerCount()).toBe(0)
    const again = createThemeController({ storage, root: document.createElement('html'), matchMedia: media.matchMedia })
    expect(again.current.value.themeId).toBe('retro-dark-lifted')
  })

  it('merges a saved custom theme over its base and applies it', async () => {
    const { theme, root } = setup()
    theme.start()
    const outcome = theme.saveCustomTheme(sea)
    expect(outcome.saved).toBe(true)
    theme.selectCustom('sea')
    await nextTick()
    expect(root.dataset.theme).toBe('retro-dark')
    expect(root.style.getPropertyValue('--accent-brand')).toBe('255 160 122')
  })

  it('never applies or saves a theme that fails validation', () => {
    const { theme } = setup()
    const outcome = theme.saveCustomTheme({ ...sea, overrides: { canvas: 'url(x)' } })
    expect(outcome).toMatchObject({ saved: false, result: { ok: false } })
    expect(theme.customThemes.value).toEqual([])
  })

  it('requires acknowledgement to save a theme with contrast failures', () => {
    const { theme } = setup()
    const weak = { ...sea, overrides: { 'text-primary': '#0c0b0a' } }
    expect(theme.saveCustomTheme(weak).saved).toBe(false)
    expect(theme.saveCustomTheme(weak, { acknowledgeContrast: true }).saved).toBe(true)
  })

  it('round-trips export → import, exporting colours as hex', () => {
    const { theme } = setup()
    theme.saveCustomTheme(sea)
    const json = theme.exportTheme('sea')
    expect(JSON.parse(json ?? '')).toEqual(sea)
    theme.deleteCustomTheme('sea')
    expect(theme.importTheme(json ?? '').saved).toBe(true)
    expect(theme.customThemes.value.map((t) => t.id)).toEqual(['sea'])
  })

  it('rejects malformed import JSON', () => {
    const { theme } = setup()
    expect(theme.importTheme('{not json').result.ok).toBe(false)
  })

  it('falls back to the base preset when the selected custom theme is deleted', async () => {
    const { theme, root } = setup()
    theme.start()
    theme.saveCustomTheme(sea)
    theme.selectCustom('sea')
    theme.deleteCustomTheme('sea')
    await nextTick()
    expect(root.dataset.theme).toBe('retro-dark')
    expect(root.style.getPropertyValue('--accent-brand')).toBe('')
  })

  it('persists the applied record for the pre-paint boot script', async () => {
    const { theme, storage } = setup()
    theme.start()
    theme.selectPreset('retro-paper')
    await nextTick()
    expect(JSON.parse(storage.getItem('admin-ui:theme-applied') ?? '')).toEqual({
      version: 1,
      data: { themeId: 'retro-paper', presetId: 'retro-paper', scheme: 'light', overrides: {} },
    })
  })
})
