import { describe, it, expect, vi, beforeEach } from 'vitest'
import { ref } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { fieldByLabel } from '../../helpers/fieldByLabel'
import type { APIKeyItem } from '../../../types/admin'

const confirm = vi.fn()
vi.mock('../../../composables/ui/useConfirm', () => ({ useConfirm: () => ({ confirm }) }))
const isMobile = ref(false)
vi.mock('../../../composables/ui/useResponsiveLayout', () => ({ useResponsiveLayout: () => ({ isMobile }) }))

import ApiKeySettings from '../../../components/settings/ApiKeySettings.vue'

const KEYS: APIKeyItem[] = [
  { id: 'k1', name: 'Personal', key: 'sk-...1111' },
  { id: 'k2', name: 'Work', key: 'sk-...2222', base_url: 'https://gw.example/v1' },
]

// The page saves keys as soon as they change, so every change is an
// update:apiKeys with the whole list; the listeners are the contract.
function mountKeys(apiKeys: APIKeyItem[] = KEYS, props: Record<string, unknown> = {}) {
  const onKeys = vi.fn()
  const onTest = vi.fn()
  const w = mount(ApiKeySettings, {
    props: { apiKeys, testLoading: false, modelCounts: { Work: 2 }, ...props },
    attrs: { 'onUpdate:apiKeys': onKeys, onTestKey: onTest },
    global: { stubs: { Icon: true } },
  })
  const button = (name: RegExp) =>
    w.findAll('button').find((b) => name.test(b.text()) || name.test(b.attributes('aria-label') ?? ''))!
  return { w, onKeys, onTest, button }
}

describe('ApiKeySettings', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    confirm.mockResolvedValue(true)
    isMobile.value = false
  })

  // On a phone the form is a drawer over the page (it opens full screen), not a block that pushes the keys down.
  it('opens the add-key form in a drawer on a phone, and Close dismisses it', async () => {
    isMobile.value = true
    const { w, button } = mountKeys()
    await button(/^add key$/i).trigger('click')
    const drawer = document.body.querySelector('[role="dialog"]')
    expect(drawer?.textContent).toContain('New key')
    expect(drawer?.querySelector('input[type="password"]')).not.toBeNull()
    ;(document.body.querySelector('[data-test="drawer-scrim"]') as HTMLElement).click()
    await flushPromises()
    expect(document.body.querySelector('[role="dialog"]')).toBeNull()
    w.unmount()
  })

  it('lists keys by name with only their last four characters', () => {
    const { w } = mountKeys()
    expect(w.text()).toContain('Personal')
    expect(w.text()).toContain('••••••••1111')
    expect(w.text()).not.toContain('sk-...1111')
  })

  it('says when there is no key yet', () => {
    expect(mountKeys([]).w.text()).toMatch(/no api keys/i)
  })

  it('keeps the add-key form closed until Add key is pressed, and closes it again on Cancel', async () => {
    const { w, button } = mountKeys()
    expect(() => fieldByLabel(w, /^key value/i)).toThrow()
    await button(/^add key$/i).trigger('click')
    expect(() => fieldByLabel(w, /^key value/i)).not.toThrow()
    expect(button(/^add key$/i).attributes('disabled')).toBeDefined() // now the form's own, waiting for a value
    await button(/^cancel$/i).trigger('click')
    expect(() => fieldByLabel(w, /^key value/i)).toThrow()
    expect(button(/^add key$/i).attributes('disabled')).toBeUndefined()
  })

  it('adds a key, named after its position when no name is given, and closes the form', async () => {
    const { w, onKeys, button } = mountKeys()
    await button(/^add key$/i).trigger('click')
    expect(button(/^add key$/i).attributes('disabled')).toBeDefined()
    await fieldByLabel(w, /^key value/i).setValue('sk-new-3333')
    expect(button(/^add key$/i).attributes('disabled')).toBeUndefined()
    await button(/^add key$/i).trigger('click')
    const added = onKeys.mock.lastCall![0] as APIKeyItem[]
    expect(added).toHaveLength(3)
    expect(added[2]).toMatchObject({ name: 'Key 3', key: 'sk-new-3333' })
    expect(() => fieldByLabel(w, /^key value/i)).toThrow()
  })

  it('suggests the provider\'s own endpoint as the base URL, and none when it has none', async () => {
    const own = mountKeys(KEYS, { showBaseUrl: true, baseUrlPlaceholder: 'https://openrouter.ai/api/v1' })
    await own.button(/^add key$/i).trigger('click')
    expect(fieldByLabel(own.w, /base url/i).attributes('placeholder')).toBe('https://openrouter.ai/api/v1')
    await own.button(/^cancel$/i).trigger('click')
    await own.button(/^work/i).trigger('click')
    expect(fieldByLabel(own.w, /^base url/i).attributes('placeholder')).toBe('https://openrouter.ai/api/v1')

    const none = mountKeys(KEYS, { showBaseUrl: true })
    await none.button(/^add key$/i).trigger('click')
    expect(fieldByLabel(none.w, /base url/i).attributes('placeholder')).toBeUndefined()
  })

  it('edits the selected key and tests it with the typed values', async () => {
    const { w, onKeys, onTest, button } = mountKeys(KEYS, { showBaseUrl: true })
    await button(/^work/i).trigger('click')
    expect(button(/^work/i).attributes('aria-expanded')).toBe('true')
    await fieldByLabel(w, /^key name$/i).setValue('Office')
    await button(/test connection/i).trigger('click')
    expect(onTest).toHaveBeenCalledWith({ key: 'sk-...2222', name: 'Office', id: 'k2', base_url: 'https://gw.example/v1' })
    await button(/save key/i).trigger('click')
    expect((onKeys.mock.lastCall![0] as APIKeyItem[])[1]).toMatchObject({ id: 'k2', name: 'Office' })
  })

  it('removes a key only after confirmation, warning about the models that use it', async () => {
    const { onKeys, button } = mountKeys()
    await button(/^work/i).trigger('click')
    confirm.mockResolvedValueOnce(false)
    await button(/^remove key$/i).trigger('click')
    await flushPromises()
    expect(onKeys).not.toHaveBeenCalled()
    expect(confirm).toHaveBeenLastCalledWith(expect.objectContaining({ message: expect.stringContaining('2 models') }))

    await button(/^remove key$/i).trigger('click')
    await flushPromises()
    expect(onKeys.mock.lastCall![0]).toEqual([KEYS[0]])
  })

  it('leaves removing every key to the provider\'s menu', () => {
    expect(mountKeys().w.text()).not.toMatch(/remove all/i)
  })

  it('announces the connection test result', async () => {
    const ok = mountKeys(KEYS, { testSuccess: 'Connected: 12 models' })
    await ok.button(/^personal/i).trigger('click')
    expect(ok.w.get('[role="status"]').text()).toContain('Connected: 12 models')

    const failed = mountKeys(KEYS, { testError: '401 Unauthorized' })
    await failed.button(/^personal/i).trigger('click')
    expect(failed.w.get('[role="alert"]').text()).toContain('401 Unauthorized')
  })
})
