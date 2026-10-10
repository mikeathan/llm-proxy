import { describe, it, expect, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { fieldByLabel } from '../../helpers/fieldByLabel'
import type { APIKeyItem } from '../../../types/admin'

const confirm = vi.fn()
vi.mock('../../../composables/ui/useConfirm', () => ({ useConfirm: () => ({ confirm }) }))

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
  const onClearAll = vi.fn()
  const w = mount(ApiKeySettings, {
    props: { apiKeys, testLoading: false, modelCounts: { Work: 2 }, ...props },
    attrs: { 'onUpdate:apiKeys': onKeys, onTestKey: onTest, onClearAll },
    global: { stubs: { Icon: true } },
  })
  const button = (name: RegExp) =>
    w.findAll('button').find((b) => name.test(b.text()) || name.test(b.attributes('aria-label') ?? ''))!
  return { w, onKeys, onTest, onClearAll, button }
}

describe('ApiKeySettings', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    confirm.mockResolvedValue(true)
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

  it('adds a key, named after its position when no name is given', async () => {
    const { w, onKeys, button } = mountKeys()
    expect(button(/^add key$/i).attributes('disabled')).toBeDefined()
    await fieldByLabel(w, /^new key value/i).setValue('sk-new-3333')
    expect(button(/^add key$/i).attributes('disabled')).toBeUndefined()
    await button(/^add key$/i).trigger('click')
    const added = onKeys.mock.lastCall![0] as APIKeyItem[]
    expect(added).toHaveLength(3)
    expect(added[2]).toMatchObject({ name: 'Key 3', key: 'sk-new-3333' })
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

  it('removes every key only after confirmation', async () => {
    const { onClearAll, button } = mountKeys()
    confirm.mockResolvedValueOnce(false)
    await button(/remove all keys/i).trigger('click')
    await flushPromises()
    expect(onClearAll).not.toHaveBeenCalled()
    await button(/remove all keys/i).trigger('click')
    await flushPromises()
    expect(onClearAll).toHaveBeenCalledTimes(1)
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
