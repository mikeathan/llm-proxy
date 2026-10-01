import { describe, expect, it, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { memoryStorage } from '../../helpers/memoryStorage'
import { fieldByLabel } from '../../helpers/fieldByLabel'
import type { createThemeController as CreateController } from '../../../composables/ui/useTheme'

// A real theme controller on in-memory storage and a detached root, so the
// page is tested against the actual validator and persistence.
let controller: ReturnType<typeof CreateController>
vi.mock('../../../composables/ui/useTheme', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../../composables/ui/useTheme')>()
  return { ...actual, useTheme: () => controller }
})
const download = vi.fn()
vi.mock('../../../utils/download', () => ({ downloadText: (...args: unknown[]) => download(...args) }))
const confirm = vi.fn()
vi.mock('../../../composables/ui/useConfirm', () => ({ useConfirm: () => ({ confirm }) }))
const toast = { success: vi.fn(), error: vi.fn(), info: vi.fn() }
vi.mock('../../../composables/useToast', () => ({ useToast: () => toast }))

import { createThemeController } from '../../../composables/ui/useTheme'
import AppearanceSettings from '../../../components/settings/AppearanceSettings.vue'

const SEA = { id: 'sea', label: 'Sea', base: 'retro-dark', overrides: { 'accent-brand': '#ffa07a' } }
const LOW_CONTRAST = { id: 'murky', label: 'Murky', base: 'retro-dark', overrides: { 'text-muted': '#1a1a1a' } }

function setup() {
  controller = createThemeController({ storage: memoryStorage(), root: document.createElement('html'), matchMedia: undefined })
  controller.start()
  const onDirty = vi.fn()
  const w = mount(AppearanceSettings, { attrs: { onDirtyChange: onDirty }, global: { stubs: { Icon: true } } })
  const button = (name: RegExp) =>
    w.findAll('button').find((b) => name.test(b.text()) || name.test(b.attributes('aria-label') ?? ''))
  return { w, onDirty, button }
}

async function importFile(w: ReturnType<typeof setup>['w'], text: string, name = 'theme.json') {
  const input = w.get('input[type="file"]')
  Object.defineProperty(input.element, 'files', { value: [new File([text], name, { type: 'application/json' })], configurable: true })
  await input.trigger('change')
  await flushPromises()
}

describe('AppearanceSettings', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    confirm.mockResolvedValue(true)
  })

  it('applies a preset as soon as it is picked', async () => {
    const { w } = setup()
    const radios = w.findAll('input[type="radio"][name="theme"]')
    expect(radios.map((r) => r.attributes('value'))).toEqual(['retro-dark', 'retro-dark-soft', 'retro-dark-lifted', 'retro-paper'])
    await radios[3]!.setValue(true)
    expect(controller.current.value.themeId).toBe('retro-paper')
  })

  it('follows the system setting until a theme is chosen', async () => {
    const { w } = setup()
    const follow = fieldByLabel(w, /follow the system/i)
    expect((follow.element as HTMLInputElement).checked).toBe(true)
    await follow.setValue(false)
    expect(controller.selection.value).toEqual({ kind: 'preset', id: 'retro-dark-soft' })
    await follow.setValue(true)
    expect(controller.selection.value).toBeNull()
  })

  it('says there is no custom theme yet', () => {
    expect(setup().w.text()).toContain('No custom themes')
  })

  it('duplicates a preset into the editor and saves it as a custom theme', async () => {
    const { w, button } = setup()
    await button(/^duplicate retro dark · soft$/i)!.trigger('click')
    expect(w.text()).toContain('New theme')
    await fieldByLabel(w, /^--accent-brand$/).setValue('#ffa07a')
    await button(/^save theme$/i)!.trigger('click')
    expect(controller.customThemes.value.map((t) => t.label)).toEqual(['Retro dark · soft copy'])
    expect(w.text()).not.toContain('New theme')
    expect(w.get('table').text()).toContain('Retro dark · soft copy')
  })

  it('uses, exports and deletes a custom theme', async () => {
    const { button } = setup()
    controller.saveCustomTheme(SEA)
    await flushPromises()
    await button(/^use sea$/i)!.trigger('click')
    expect(controller.selection.value).toEqual({ kind: 'custom', id: 'sea' })

    await button(/^export sea$/i)!.trigger('click')
    expect(download).toHaveBeenCalledWith('sea.theme.json', expect.stringContaining('"accent-brand": "#ffa07a"'), 'application/json')

    confirm.mockResolvedValueOnce(false)
    await button(/^delete sea$/i)!.trigger('click')
    await flushPromises()
    expect(controller.customThemes.value).toHaveLength(1)
    await button(/^delete sea$/i)!.trigger('click')
    await flushPromises()
    expect(controller.customThemes.value).toHaveLength(0)
    expect(controller.selection.value).toEqual({ kind: 'preset', id: 'retro-dark' })
  })

  it('imports a valid theme file', async () => {
    const { w } = setup()
    await importFile(w, JSON.stringify(SEA))
    expect(controller.customThemes.value.map((t) => t.id)).toEqual(['sea'])
    expect(toast.success).toHaveBeenCalled()
  })

  it('rejects a bad theme file with the reason, changing nothing', async () => {
    const { w } = setup()
    await importFile(w, JSON.stringify({ ...SEA, overrides: { canvas: 'url(https://example.com)' } }), 'evil.json')
    expect(w.get('[data-test="import-error"]').text()).toMatch(/evil\.json/)
    expect(w.get('[data-test="import-error"]').text()).toMatch(/not a valid colour/)
    expect(controller.customThemes.value).toHaveLength(0)

    await importFile(w, '{ nope')
    expect(w.get('[data-test="import-error"]').text()).toMatch(/not valid JSON/)
  })

  it('asks before an import replaces a theme with the same id', async () => {
    const { w } = setup()
    controller.saveCustomTheme(SEA)
    confirm.mockResolvedValueOnce(false)
    await importFile(w, JSON.stringify({ ...SEA, label: 'Sea 2' }))
    expect(controller.customThemes.value[0]!.label).toBe('Sea')
  })

  it('opens an imported theme with contrast failures in the editor for acknowledgement', async () => {
    const { w, button } = setup()
    await importFile(w, JSON.stringify(LOW_CONTRAST))
    expect(controller.customThemes.value).toHaveLength(0)
    expect(w.find('[data-test="contrast-warning"]').exists()).toBe(true)
    await fieldByLabel(w, /save anyway/i).setValue(true)
    await button(/^save theme$/i)!.trigger('click')
    expect(controller.customThemes.value.map((t) => t.id)).toEqual(['murky'])
  })

  it('reports an open editor with edits to the page guard', async () => {
    const { w, button, onDirty } = setup()
    controller.saveCustomTheme(SEA)
    await flushPromises()
    await button(/^edit sea$/i)!.trigger('click')
    await fieldByLabel(w, /^name$/i).setValue('Sea two')
    expect(onDirty).toHaveBeenLastCalledWith(true)
    await button(/^cancel$/i)!.trigger('click')
    expect(onDirty).toHaveBeenLastCalledWith(false)
  })
})
