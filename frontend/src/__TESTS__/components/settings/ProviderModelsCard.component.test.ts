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

import { DEFAULT_CONFIG } from '../../../composables/models/useConfig'
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
    models.state.value = { config: {} }
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

  describe('adding a cloud model: API key default and model loading', () => {
    const TWO_KEYS: APIKeyItem[] = [
      { id: 'k1', name: 'Personal', key: 'sk-...1111' },
      { id: 'k2', name: 'Work', key: 'sk-...2222' },
    ]
    const reload = (w: ReturnType<typeof mountCard>['w']) =>
      w.findAll('button').find((b) => /reload the model list/i.test(b.attributes('aria-label') ?? ''))
    const scan = (w: ReturnType<typeof mountCard>['w']) =>
      w.findAll('button').find((b) => /scan endpoint/i.test(b.text()))

    it('preselects the first key and fetches once with it', async () => {
      const { w, button } = mountCard({ provider: 'openai', apiKeys: TWO_KEYS, models: [] })
      await button(/^add model$/i)!.trigger('click')
      await flushPromises()
      expect((fieldByLabel(w, /api key/i).element as HTMLSelectElement).value).toBe('Personal')
      expect(models.fetchProviderModels).toHaveBeenCalledTimes(1)
      expect(models.fetchProviderModels).toHaveBeenCalledWith('openai', 'Personal')
    })

    it('does not fetch and keeps the placeholder when there are no keys', async () => {
      const { w, button } = mountCard({ provider: 'openai', apiKeys: [], models: [] })
      await button(/^add model$/i)!.trigger('click')
      await flushPromises()
      expect((fieldByLabel(w, /api key/i).element as HTMLSelectElement).value).toBe('')
      expect(models.fetchProviderModels).not.toHaveBeenCalled()
      expect(scan(w)!.attributes('disabled')).toBeDefined()
    })

    it('fetches again with the newly chosen key', async () => {
      const { w, button } = mountCard({ provider: 'openai', apiKeys: TWO_KEYS, models: [] })
      await button(/^add model$/i)!.trigger('click')
      await flushPromises()
      await fieldByLabel(w, /api key/i).setValue('Work')
      await flushPromises()
      expect(models.fetchProviderModels).toHaveBeenCalledTimes(2)
      expect(models.fetchProviderModels).toHaveBeenLastCalledWith('openai', 'Work')
    })

    it('does not fetch while the key is cleared, and disables Reload and Scan', async () => {
      models.fetchProviderModels.mockResolvedValue([{ id: 'gpt-5' }])
      const { w, button } = mountCard({ provider: 'openai', apiKeys: TWO_KEYS, models: [] })
      await button(/^add model$/i)!.trigger('click')
      await flushPromises()
      expect(reload(w)!.attributes('disabled')).toBeUndefined()
      models.fetchProviderModels.mockClear()
      await fieldByLabel(w, /api key/i).setValue('')
      await flushPromises()
      expect(models.fetchProviderModels).not.toHaveBeenCalled()
      expect(reload(w)).toBeUndefined()
      expect(scan(w)!.attributes('disabled')).toBeDefined()
    })

    it('falls back to the first remaining key when the selected key is removed', async () => {
      const { w, button } = mountCard({ provider: 'openai', apiKeys: TWO_KEYS, models: [] })
      await button(/^add model$/i)!.trigger('click')
      await fieldByLabel(w, /api key/i).setValue('Work')
      await flushPromises()
      models.fetchProviderModels.mockClear()
      await w.setProps({ apiKeys: TWO_KEYS.slice(0, 1) })
      await flushPromises()
      expect((fieldByLabel(w, /api key/i).element as HTMLSelectElement).value).toBe('Personal')
      expect(models.fetchProviderModels).toHaveBeenCalledWith('openai', 'Personal')
    })

    it('falls back to an empty key when the last key is removed', async () => {
      const { w, button } = mountCard({ provider: 'openai', apiKeys: KEYS, models: [] })
      await button(/^add model$/i)!.trigger('click')
      await flushPromises()
      models.fetchProviderModels.mockClear()
      await w.setProps({ apiKeys: [] })
      await flushPromises()
      expect((fieldByLabel(w, /api key/i).element as HTMLSelectElement).value).toBe('')
      expect(models.fetchProviderModels).not.toHaveBeenCalled()
    })

    it('shows the model list as a multi-row listbox', async () => {
      models.fetchProviderModels.mockResolvedValue([{ id: 'gpt-5' }, { id: 'gpt-5-mini' }])
      const { w, button } = mountCard({ provider: 'openai', apiKeys: KEYS, models: [] })
      await button(/^add model$/i)!.trigger('click')
      await flushPromises()
      expect(fieldByLabel(w, /model id/i).attributes('size')).toBe('8')
    })
  })

  describe('temperature normalisation', () => {
    it('saves a cleared temperature as 0 when editing', async () => {
      const { w, button } = mountCard({
        provider: 'openai',
        apiKeys: KEYS,
        models: [{ ...CLOUD[0], temperature: 0.1, workload_class: 'cloud' } as Model],
      })
      await button(/^edit gpt$/i)!.trigger('click')
      expect(fieldByLabel(w, /temperature/i).attributes('placeholder')).toBe('Provider default')
      await fieldByLabel(w, /temperature/i).setValue('')
      await w.get('form').trigger('submit')
      await flushPromises()
      expect(models.updateModel).toHaveBeenCalledWith(expect.objectContaining({ name: 'gpt', temperature: 0 }))
    })

    it('saves a cleared temperature as 0 when adding', async () => {
      models.fetchProviderModels.mockResolvedValue([{ id: 'gpt-5-mini' }])
      const { w, button } = mountCard({ provider: 'openai', apiKeys: KEYS, models: [] })
      await button(/^add model$/i)!.trigger('click')
      await flushPromises()
      await fieldByLabel(w, /model id/i).setValue('gpt-5-mini')
      await fieldByLabel(w, /temperature/i).setValue('')
      await w.get('form').trigger('submit')
      await flushPromises()
      expect(models.addModel).toHaveBeenCalledWith(expect.objectContaining({ model_id: 'gpt-5-mini', temperature: 0 }))
    })

    it('keeps an explicit temperature', async () => {
      const { w, button } = mountCard({ provider: 'openai', apiKeys: KEYS, models: CLOUD })
      await button(/^edit gpt$/i)!.trigger('click')
      await fieldByLabel(w, /temperature/i).setValue('0.4')
      await w.get('form').trigger('submit')
      await flushPromises()
      expect(models.updateModel).toHaveBeenCalledWith(expect.objectContaining({ temperature: 0.4 }))
    })

    it('shows a blank temperature with the "Server default" placeholder for an unset local model', async () => {
      const { w, button } = mountCard({ provider: 'local', apiKeys: [], models: [{ ...LOCAL[0], temperature: 0, workload_class: 'local' } as Model] })
      await button(/^edit qwen$/i)!.trigger('click')
      const field = fieldByLabel(w, /temperature/i)
      expect(field.attributes('placeholder')).toBe('Server default')
      expect((field.element as HTMLInputElement).value).not.toBe('0.1')
    })

    it('sends temperature 0 from provider_defaults when a cloud add form is left untouched', async () => {
      // Use a non-zero global fallback so the test proves the provider tier wins.
      models.state.value = {
        config: {
          agent_defaults: { ...DEFAULT_CONFIG.agent_defaults, temperature: 0.1 },
          provider_defaults: { openai: { ...DEFAULT_CONFIG.agent_defaults, temperature: 0 } },
        },
      } as never
      models.fetchProviderModels.mockResolvedValue([{ id: 'gpt-5-mini' }])
      const { w, button } = mountCard({ provider: 'openai', apiKeys: KEYS, models: [] })
      await button(/^add model$/i)!.trigger('click')
      await flushPromises()
      await fieldByLabel(w, /model id/i).setValue('gpt-5-mini')
      await w.get('form').trigger('submit')
      await flushPromises()
      expect(models.addModel).toHaveBeenCalledWith(expect.objectContaining({ model_id: 'gpt-5-mini', temperature: 0 }))
    })
  })

  describe('blank numeric tuning fields', () => {
    const FIELDS: [RegExp, string][] = [
      [/^max steps/i, 'max_steps'],
      [/context budget/i, 'context_budget'],
      [/^max tokens/i, 'max_tokens'],
      [/reasoning budget/i, 'reasoning_budget'],
      [/^temperature/i, 'temperature'],
      [/^timeout \(min\)/i, 'timeout_minutes'],
      [/per-tool timeout/i, 'tool_timeout_seconds'],
      [/filesystem timeout/i, 'filesystem_tool_timeout_seconds'],
      [/max plan duration/i, 'max_plan_duration_minutes'],
      [/max plan steps/i, 'max_plan_steps'],
      [/guardrail timeout \(sec\)/i, 'guardrail_timeout_seconds'],
      [/guardrail approval timeout/i, 'guardrail_approval_timeout_seconds'],
    ]
    // Distinctive provider-tier values so a pass proves the tier wins, not a fallback.
    const tier = Object.fromEntries(FIELDS.map(([, key], i) => [key, key === 'temperature' ? 0 : 100 + i]))
    const expectedCloud = { ...tier }
    const setProviderTier = () => {
      models.state.value = {
        config: { provider_defaults: { openai: { ...DEFAULT_CONFIG.agent_defaults, ...tier } } },
      } as never
    }

    it('saves a cleared cloud field as the provider tier value (temperature stays 0) when editing', async () => {
      setProviderTier()
      const filled = Object.fromEntries(FIELDS.map(([, key]) => [key, 7]))
      const { w, button } = mountCard({
        provider: 'openai',
        apiKeys: KEYS,
        models: [{ ...CLOUD[0], ...filled, workload_class: 'cloud' } as Model],
      })
      await button(/^edit gpt$/i)!.trigger('click')
      for (const [label] of FIELDS) await fieldByLabel(w, label).setValue('')
      await w.get('form').trigger('submit')
      await flushPromises()
      expect(models.updateModel).toHaveBeenCalledWith(expect.objectContaining({ name: 'gpt', ...expectedCloud }))
    })

    it('saves cleared max_tokens and context_budget as the provider tier, not 0', async () => {
      models.state.value = {
        config: { provider_defaults: { openai: { ...DEFAULT_CONFIG.agent_defaults, max_tokens: 8192, context_budget: 50000 } } },
      } as never
      const { w, button } = mountCard({
        provider: 'openai',
        apiKeys: KEYS,
        models: [{ ...CLOUD[0], max_tokens: 1000, context_budget: 2000, workload_class: 'cloud' } as Model],
      })
      await button(/^edit gpt$/i)!.trigger('click')
      await fieldByLabel(w, /^max tokens/i).setValue('')
      await fieldByLabel(w, /context budget/i).setValue('')
      await w.get('form').trigger('submit')
      await flushPromises()
      expect(models.updateModel).toHaveBeenCalledWith(expect.objectContaining({ max_tokens: 8192, context_budget: 50000 }))
    })

    it('saves 0 for a cleared cloud field when the tier value is not positive', async () => {
      models.state.value = {
        config: { provider_defaults: { openai: { ...DEFAULT_CONFIG.agent_defaults, max_tokens: 0 } } },
      } as never
      const { w, button } = mountCard({
        provider: 'openai',
        apiKeys: KEYS,
        models: [{ ...CLOUD[0], max_tokens: 1000, workload_class: 'cloud' } as Model],
      })
      await button(/^edit gpt$/i)!.trigger('click')
      await fieldByLabel(w, /^max tokens/i).setValue('')
      await w.get('form').trigger('submit')
      await flushPromises()
      expect(models.updateModel).toHaveBeenCalledWith(expect.objectContaining({ max_tokens: 0 }))
    })

    it('saves every cleared numeric field as 0 for a local model when editing', async () => {
      setProviderTier()
      const local = FIELDS.filter(([, key]) => key !== 'context_budget' && key !== 'max_tokens')
      const filled = Object.fromEntries(local.map(([, key]) => [key, 7]))
      const { w, button } = mountCard({
        provider: 'local',
        apiKeys: [],
        models: [{ ...LOCAL[0], ...filled, workload_class: 'local' } as Model],
      })
      await button(/^edit qwen$/i)!.trigger('click')
      for (const [label] of local) await fieldByLabel(w, label).setValue('')
      await w.get('form').trigger('submit')
      await flushPromises()
      const zeros = Object.fromEntries(local.map(([, key]) => [key, 0]))
      expect(models.updateModel).toHaveBeenCalledWith(expect.objectContaining({ name: 'qwen', ...zeros }))
    })

    it('saves every cleared numeric field as 0 when adding a local model', async () => {
      const local = FIELDS.filter(([, key]) => key !== 'context_budget' && key !== 'max_tokens')
      const { w, button } = mountCard({ provider: 'local', apiKeys: [], models: [], availableModels: DISCOVERED })
      await button(/^add model$/i)!.trigger('click')
      await flushPromises()
      await fieldByLabel(w, /filename|gguf/i).setValue('llama.gguf')
      for (const [label] of local) await fieldByLabel(w, label).setValue('')
      await w.get('form').trigger('submit')
      await flushPromises()
      const zeros = Object.fromEntries(local.map(([, key]) => [key, 0]))
      expect(models.addModel).toHaveBeenCalledWith(expect.objectContaining({ filename: 'llama.gguf', ...zeros }))
    })

    it('saves a cleared cloud field as the provider tier value when adding', async () => {
      setProviderTier()
      models.fetchProviderModels.mockResolvedValue([{ id: 'gpt-5-mini' }])
      const { w, button } = mountCard({ provider: 'openai', apiKeys: KEYS, models: [] })
      await button(/^add model$/i)!.trigger('click')
      await flushPromises()
      await fieldByLabel(w, /model id/i).setValue('gpt-5-mini')
      for (const [label] of FIELDS) await fieldByLabel(w, label).setValue('')
      await w.get('form').trigger('submit')
      await flushPromises()
      expect(models.addModel).toHaveBeenCalledWith(expect.objectContaining({ model_id: 'gpt-5-mini', ...expectedCloud }))
    })
  })
})
