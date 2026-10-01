import { describe, it, expect, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { ref } from 'vue'
import { fieldByLabel } from '../../helpers/fieldByLabel'
import type { APIKeyItem } from '../../../types/admin'
import type { AvailableModel, Model } from '../../../types/model'

const models = {
  state: ref({ config: {} }),
  addModel: vi.fn(),
  updateModel: vi.fn(),
  removeModel: vi.fn(),
  removeAllModels: vi.fn(),
  fetchProviderModels: vi.fn(),
}
vi.mock('../../../composables/models/useModels', () => ({ useModels: () => models }))
const confirm = vi.fn()
vi.mock('../../../composables/ui/useConfirm', () => ({ useConfirm: () => ({ confirm }) }))

import ProviderModelsCard from '../../../components/settings/ProviderModelsCard.vue'

const KEYS: APIKeyItem[] = [{ id: 'k1', name: 'Personal', key: 'sk-...1111' }]
const CLOUD: Model[] = [
  { name: 'gpt', provider: 'openai', model_id: 'gpt-5', endpoint: '', active: false, ready: false, provider_config: { api_key_name: 'Personal' } } as Model,
]
const LOCAL: Model[] = [{ name: 'qwen', provider: 'local', filename: 'qwen.gguf', port: 8081, endpoint: '', active: false, ready: false } as Model]
const DISCOVERED: AvailableModel[] = [
  { name: 'llama', filename: 'llama.gguf', size_bytes: 4_000_000_000, metadata: { name: 'Llama 3', architecture: 'llama', quantization: 'Q4_K_M' } } as AvailableModel,
  { name: 'qwen', filename: 'qwen.gguf', size_bytes: 3_000_000_000 } as AvailableModel,
]

function mountCard(props: Record<string, unknown>) {
  const onRefresh = vi.fn()
  const w = mount(ProviderModelsCard, { props: props as never, attrs: { onRefresh }, global: { stubs: { Icon: true } } })
  const button = (name: RegExp) =>
    w.findAll('button').find((b) => name.test(b.text()) || name.test(b.attributes('aria-label') ?? '') || name.test(b.attributes('title') ?? ''))
  return { w, onRefresh, button }
}

// Characterised before the Settings redesign (plan D22), then moved to the
// redesigned card: the model list, the add / edit form and the removal flow.
describe('ProviderModelsCard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    confirm.mockResolvedValue(true)
    models.fetchProviderModels.mockResolvedValue([])
  })

  it('lists cloud models under the key they use', () => {
    const { w } = mountCard({ provider: 'openai', apiKeys: KEYS, models: CLOUD })
    expect(w.text()).toContain('Personal')
    expect(w.text()).toContain('gpt')
    expect(w.text()).toContain('gpt-5')
  })

  it('adds a cloud model once a key and a model ID are chosen', async () => {
    const { w, button } = mountCard({ provider: 'openai', apiKeys: KEYS, models: CLOUD })
    await button(/^add model$/i)!.trigger('click')
    const submit = () => w.findAll('button').find((b) => /^add model$/i.test(b.text()))!
    expect(submit().attributes('disabled')).toBeDefined()
    await fieldByLabel(w, /api key/i).setValue('Personal')
    await fieldByLabel(w, /model id/i).setValue('gpt-5-mini')
    expect(submit().attributes('disabled')).toBeUndefined()
  })

  it('edits a model in place', async () => {
    const { w, button } = mountCard({ provider: 'openai', apiKeys: KEYS, models: CLOUD })
    await button(/^edit gpt$/i)!.trigger('click')
    expect((fieldByLabel(w, /^name$/i).element as HTMLInputElement).value).toBe('gpt')
    await fieldByLabel(w, /^name$/i).setValue('gpt-main')
    await w.get('form').trigger('submit')
    await flushPromises()
    expect(models.updateModel).toHaveBeenCalledWith(expect.objectContaining({ name: 'gpt-main' }))
  })

  it('removes a model after confirmation', async () => {
    const { button, onRefresh } = mountCard({ provider: 'openai', apiKeys: KEYS, models: CLOUD })
    await button(/^remove gpt$/i)!.trigger('click')
    await flushPromises()
    expect(models.removeModel).toHaveBeenCalledWith('gpt')
    expect(onRefresh).toHaveBeenCalled()
  })

  it('offers GGUF files found on disk for a local model', async () => {
    const { w, button } = mountCard({ provider: 'local', apiKeys: [], models: LOCAL, availableModels: DISCOVERED })
    expect(w.text()).toContain('Llama 3')
    expect(w.text()).toContain('Q4_K_M')
    // A file that is already configured cannot be added twice.
    expect(button(/^add qwen$/i)).toBeUndefined()
    expect(w.text()).toContain('Added')
    await button(/^add llama 3$/i)!.trigger('click')
    expect((fieldByLabel(w, /filename/i).element as HTMLInputElement).value).toBe('llama.gguf')
  })

  it('says when the local engine has no model yet', () => {
    const { w } = mountCard({ provider: 'local', apiKeys: [], models: [] })
    expect(w.text()).toContain('No local models')
  })
})
