import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, h, ref } from 'vue'
import { createMemoryHistory, RouterView } from 'vue-router'
import { createAppRouter } from '../../router'
import { AdminApiService } from '../../services/admin/adminService'
import { DEFAULT_CONFIG, useConfig } from '../../composables/models/useConfig'
import { fieldByLabel } from '../helpers/fieldByLabel'
import { memoryStorage } from '../helpers/memoryStorage'
import type { AdminState, GlobalConfig } from '../../types/admin'
import type { Model } from '../../types/model'

vi.mock('../../services/admin/adminService', () => ({
  AdminApiService: {
    fetchState: vi.fn(),
    updateConfig: vi.fn(),
    fetchProviderManifests: vi.fn(),
    fetchProviderKeys: vi.fn(),
    saveProviderKeys: vi.fn(),
    deleteAllProviderKeys: vi.fn(),
    testConnection: vi.fn(),
    fetchProcesses: vi.fn(),
  },
}))

const models = {
  state: ref<AdminState | null>(null),
  error: ref<string | null>(null),
  availableModels: ref([] as unknown[]),
  agentDefaults: ref(DEFAULT_CONFIG.agent_defaults),
  refresh: vi.fn(),
  startModel: vi.fn(),
  stopModel: vi.fn(),
  removeAllModels: vi.fn(),
  addModel: vi.fn(),
  updateModel: vi.fn(),
  removeModel: vi.fn(),
  fetchProviderModels: vi.fn(),
}
vi.mock('../../composables/models/useModels', () => ({ useModels: () => models }))
const toast = { success: vi.fn(), error: vi.fn(), info: vi.fn(), show: vi.fn() }
vi.mock('../../composables/useToast', () => ({ useToast: () => toast }))
const confirm = vi.fn()
vi.mock('../../composables/ui/useConfirm', () => ({ useConfirm: () => ({ confirm }) }))
// The provider rail is always shown on a desktop and is a screen of its own on a phone.
const mobile = ref(false)
vi.mock('../../composables/ui/useResponsiveLayout', () => ({ useResponsiveLayout: () => ({ isMobile: mobile }) }))

const model = (m: Partial<Model>): Model => ({ name: 'm', provider: 'local', endpoint: '', active: false, ready: false, ...m })
const LOCAL_IDLE = model({ name: 'qwen', filename: 'qwen.gguf', port: 8082, metadata: { quantization: 'Q4_K_M', context_length: 262144 } as Model['metadata'] })
const LOCAL_ACTIVE = model({ name: 'llama', filename: 'llama.gguf', port: 8081, active: true, ready: true })
const CLOUD = model({ name: 'gpt', provider: 'openai', model_id: 'gpt-5', workload_class: 'cloud', provider_config: { api_key_name: 'Work' } })
const LOADED: GlobalConfig = { ...structuredClone(DEFAULT_CONFIG), primary_model: 'gpt', model_host: '10.0.0.2' }

function stateWith(list: Model[], config: Partial<GlobalConfig> = LOADED): AdminState {
  return { models: list, config, next_port: 9000, available: [] } as unknown as AdminState
}

async function mountModels(path = '/models') {
  const router = createAppRouter(createMemoryHistory())
  await router.push(path)
  const w = mount(defineComponent({ render: () => h(RouterView) }), { global: { plugins: [router], stubs: { Icon: true, BrandMark: true, InfrastructurePanel: true, teleport: true } }, attachTo: document.body })
  await vi.waitFor(() => expect(w.find('h1').exists()).toBe(true))
  await flushPromises()
  return { w, router }
}
type Mounted = Awaited<ReturnType<typeof mountModels>>['w']
const button = (w: Mounted, label: string | RegExp) =>
  w.findAll('button').find((b) => (typeof label === 'string' ? b.attributes('aria-label') === label || b.text() === label : label.test(b.text()) || label.test(b.attributes('aria-label') ?? '')))!
const cardNames = (w: Mounted) => w.get('[data-test="model-cards"]').findAll('article').map((a) => a.attributes('data-model'))

describe('Models page', () => {
  beforeEach(() => {
    vi.stubGlobal('localStorage', memoryStorage())
    vi.clearAllMocks()
    useConfig().config.value = structuredClone(DEFAULT_CONFIG)
    models.state.value = stateWith([LOCAL_IDLE, CLOUD, LOCAL_ACTIVE])
    models.error.value = null
    models.availableModels.value = []
    vi.mocked(AdminApiService.fetchState).mockResolvedValue({ config: structuredClone(LOADED) } as never)
    vi.mocked(AdminApiService.fetchProviderManifests).mockResolvedValue([])
    vi.mocked(AdminApiService.fetchProviderKeys).mockResolvedValue([])
    vi.mocked(AdminApiService.updateConfig).mockResolvedValue()
    models.fetchProviderModels.mockResolvedValue([])
    confirm.mockResolvedValue(true)
    mobile.value = false
  })
  afterEach(() => {
    document.body.innerHTML = ''
    vi.unstubAllGlobals()
  })

  it('loads the catalogue, the model settings and every cloud provider\'s keys on mount', async () => {
    await mountModels()
    expect(models.refresh).toHaveBeenCalled()
    expect(AdminApiService.fetchState).toHaveBeenCalledTimes(1)
    for (const p of ['gemini', 'openai', 'openrouter', 'nvidia', 'opencode-go']) expect(AdminApiService.fetchProviderKeys).toHaveBeenCalledWith(p)
  })

  it('has three tabs, Models first, each its own address; providers are part of Models', async () => {
    const { w } = await mountModels()
    const tabs = w.get('nav[aria-label="Models"]').findAll('a')
    expect(tabs.map((a) => a.text())).toEqual(['Models', 'Engine', 'Routing'])
    expect(tabs.map((a) => a.attributes('href'))).toEqual(['/models', '/models/engine', '/models/routing'])
    const onProvider = await mountModels('/models/providers/openai')
    expect(onProvider.w.get('nav[aria-label="Models"]').findAll('a')[0]!.attributes('aria-current')).toBe('page')
  })

  it('shows the models as cards by default, the running one first, with its routing role', async () => {
    const { w } = await mountModels()
    expect(cardNames(w)).toEqual(['llama', 'qwen', 'gpt'])
    const gpt = w.get('article[data-model="gpt"]')
    expect(gpt.text()).toContain('Primary')
    expect(gpt.text()).toContain('OpenAI / Compatible')
    expect(w.get('article[data-model="qwen"]').text()).toContain('262.1K context')
    expect(w.find('[data-test="model-list"]').exists()).toBe(false)
  })

  it('switches to a list and remembers it on this device', async () => {
    const first = await mountModels()
    await button(first.w, 'List').trigger('click')
    expect(first.w.find('[data-test="model-list"]').exists()).toBe(true)
    first.w.unmount()
    const again = await mountModels()
    expect(again.w.find('[data-test="model-list"]').exists()).toBe(true)
  })

  it('narrows to local or cloud models, and by a typed filter', async () => {
    const { w } = await mountModels()
    await button(w, /^Local 2$/).trigger('click')
    expect(cardNames(w)).toEqual(['llama', 'qwen'])
    await button(w, /^Cloud 1$/).trigger('click')
    expect(cardNames(w)).toEqual(['gpt'])
    await button(w, /^All 3$/).trigger('click')
    await w.get('input[aria-label="Filter models"]').setValue('qw')
    expect(cardNames(w)).toEqual(['qwen'])
  })

  it('stops the running local model, and blocks starting another while one runs', async () => {
    const { w } = await mountModels()
    await button(w, 'Stop llama').trigger('click')
    expect(models.stopModel).toHaveBeenCalledTimes(1)
    expect(button(w, 'Start qwen').attributes('disabled')).toBeDefined()
  })

  it('starts a local model when none runs', async () => {
    models.state.value = stateWith([LOCAL_IDLE, CLOUD])
    const { w } = await mountModels()
    await button(w, 'Start qwen').trigger('click')
    expect(models.startModel).toHaveBeenCalledWith('qwen')
  })

  it('edits a model in a side panel over the list', async () => {
    const { w } = await mountModels()
    await button(w, 'Edit qwen').trigger('click')
    await flushPromises()
    const panel = w.get('[role="dialog"]')
    expect(panel.text()).toContain('Edit qwen')
    expect((fieldByLabel(w, /^name$/i).element as HTMLInputElement).value).toBe('qwen')
    expect(w.find('[data-test="model-cards"]').exists()).toBe(true)
  })

  it('adds a model by first saying where it runs', async () => {
    const { w } = await mountModels()
    await button(w, 'Add model').trigger('click')
    await flushPromises()
    expect(w.get('[role="dialog"]').text()).toContain('Add a model')
    const where = fieldByLabel(w, /where it runs/i)
    expect(where.findAll('option').map((o) => o.text())).toEqual(['This computer (llama.cpp)', 'Google Gemini', 'OpenAI / Compatible', 'OpenRouter', 'NVIDIA NIM', 'OpenCode Go'])
    expect(() => fieldByLabel(w, /model filename/i)).not.toThrow()
    await where.setValue('openai')
    await flushPromises()
    expect(() => fieldByLabel(w, /api key/i)).not.toThrow()
  })

  it('offers adding a model or connecting a provider when there are none', async () => {
    models.state.value = stateWith([])
    const { w } = await mountModels()
    expect(w.text()).toContain('No models yet')
    expect(w.find('a[href="/models/providers"]').exists()).toBe(true)
  })

  it('shows loading, then an error with a retry when the catalogue cannot be read', async () => {
    models.state.value = null
    const loading = await mountModels()
    expect(loading.w.text()).toContain('Loading models')
    loading.w.unmount()

    models.error.value = 'permission denied'
    const failed = await mountModels()
    expect(failed.w.findAll('[role="alert"]').some((a) => a.text().includes('permission denied'))).toBe(true)
    const before = models.refresh.mock.calls.length
    await button(failed.w, 'Retry').trigger('click')
    expect(models.refresh.mock.calls.length).toBe(before + 1)
  })

  it('links to the dashboard tab where the running model is read', async () => {
    const { w } = await mountModels()
    expect(w.get('[data-test="open-board"]').attributes('href')).toContain('board=host')
  })

  describe('Providers', () => {
    const OPENAI_KEY = [{ id: 'k1', name: 'Work', key: '••••1111' }]
    const withKey = () => vi.mocked(AdminApiService.fetchProviderKeys).mockImplementation(async (p) => (p === 'openai' ? OPENAI_KEY : []))
    const providerLinks = (w: Mounted) => w.get('nav[aria-label="Providers"]').findAll('a')
    const menuButton = (w: Mounted, label: RegExp) => button(w, label)

    it('lists All models, This computer and every cloud provider by name in a rail, with what each holds', async () => {
      withKey()
      const { w } = await mountModels()
      await vi.waitFor(() => expect(w.get('nav[aria-label="Providers"]').text()).toContain('1 key · 1 model'))
      const links = providerLinks(w)
      expect(links.map((a) => a.attributes('href'))).toEqual(['/models', '/models/providers/local', '/models/providers/gemini', '/models/providers/nvidia', '/models/providers/openai', '/models/providers/opencode-go', '/models/providers/openrouter'])
      expect(links[0]!.text()).toContain('All models')
      expect(links[1]!.text()).toContain('3 models'.replace('3', '2'))
      expect(links[4]!.text()).toContain('1 key · 1 model')
      expect(links[2]!.text()).toContain('Not set up')
    })

    it('shows a provider\'s keys and, under each key, the models it serves', async () => {
      withKey()
      const { w } = await mountModels('/models/providers/openai')
      await vi.waitFor(() => expect(w.get('[data-test="provider-pane"]').text()).toContain('Work'))
      const pane = w.get('[data-test="provider-pane"]')
      expect(pane.get('h2').text()).toBe('OpenAI / Compatible')
      expect(pane.get('[data-key-group="Work"]').text()).toContain('gpt')
      expect(pane.text()).not.toContain('qwen')
    })

    it('keeps Add key closed until it is pressed', async () => {
      withKey()
      const { w } = await mountModels('/models/providers/openai')
      await vi.waitFor(() => expect(w.get('[data-test="provider-pane"]').text()).toContain('Work'))
      expect(() => fieldByLabel(w, /^key value/i)).toThrow()
      await button(w, /^add key$/i).trigger('click')
      expect(() => fieldByLabel(w, /^key value/i)).not.toThrow()
    })

    it('suggests the provider\'s own endpoint in the base URL, not another provider\'s', async () => {
      vi.mocked(AdminApiService.fetchProviderManifests).mockResolvedValue([{ id: 'openrouter', name: 'OpenRouter', default_base_url: 'https://openrouter.ai/api/v1', archetype: 'openai' }])
      const config = { ...structuredClone(LOADED), provider_defaults: { openrouter: { ...DEFAULT_CONFIG.agent_defaults, supports_base_url: true } } }
      vi.mocked(AdminApiService.fetchState).mockResolvedValue({ config } as never)
      const { w } = await mountModels('/models/providers/openrouter')
      await vi.waitFor(() => expect(button(w, /^add key$/i)).toBeDefined())
      await button(w, /^add key$/i).trigger('click')
      expect(fieldByLabel(w, /base url/i).attributes('placeholder')).toBe('https://openrouter.ai/api/v1')
    })

    it('adds a model of the provider under a key from that key\'s group', async () => {
      withKey()
      const { w } = await mountModels('/models/providers/openai')
      await vi.waitFor(() => expect(w.find('[data-key-group="Work"]').exists()).toBe(true))
      await w.get('[data-key-group="Work"]').findAll('button').find((b) => /add a model/i.test(b.text()))!.trigger('click')
      await flushPromises()
      expect((fieldByLabel(w, /where it runs/i).element as HTMLSelectElement).value).toBe('openai')
      expect(models.fetchProviderModels).toHaveBeenCalledWith('openai', 'Work')
    })

    it('puts the remove-all actions in a menu, and asks before each', async () => {
      withKey()
      const { w } = await mountModels('/models/providers/openai')
      await vi.waitFor(() => expect(w.get('[data-test="provider-pane"]').text()).toContain('Work'))
      expect(w.text()).not.toMatch(/remove all/i)
      await button(w, 'More actions for OpenAI / Compatible').trigger('click')
      confirm.mockResolvedValueOnce(false)
      await menuButton(w, /remove all openai \/ compatible models/i).trigger('click')
      await flushPromises()
      expect(models.removeAllModels).not.toHaveBeenCalled()
      await button(w, 'More actions for OpenAI / Compatible').trigger('click')
      await menuButton(w, /remove all openai \/ compatible models/i).trigger('click')
      await flushPromises()
      expect(models.removeAllModels).toHaveBeenCalledWith('openai')

      await button(w, 'More actions for OpenAI / Compatible').trigger('click')
      await menuButton(w, /remove all keys/i).trigger('click')
      await flushPromises()
      expect(confirm).toHaveBeenLastCalledWith(expect.objectContaining({ title: expect.stringMatching(/remove all keys/i) }))
      expect(AdminApiService.deleteAllProviderKeys).toHaveBeenCalledWith('openai')
    })

    it('shows Gemini\'s Vertex AI fields with the Save row, and switching provider does not ask to discard them', async () => {
      const { w, router } = await mountModels('/models/providers/gemini')
      expect(w.text()).not.toContain('Unsaved changes')
      await fieldByLabel(w, /vertex ai project id/i).setValue('proj-1')
      expect(w.text()).toContain('Unsaved changes')
      await router.push('/models/providers/openai')
      await flushPromises()
      expect(confirm).not.toHaveBeenCalled()
      await router.push('/models/providers/gemini')
      await flushPromises()
      await button(w, /^save model settings$/i).trigger('click')
      await flushPromises()
      expect(AdminApiService.updateConfig).toHaveBeenCalledWith(expect.objectContaining({ providers: expect.objectContaining({ gemini: expect.objectContaining({ project_id: 'proj-1' }) }) }))
    })

    it('shows This computer with the local models, the files found on disk and a link to Engine', async () => {
      models.availableModels.value = [{ name: 'gemma', filename: 'gemma.gguf', size_bytes: 1 }]
      const { w } = await mountModels('/models/providers/local')
      expect(cardNames(w)).toEqual(['llama', 'qwen'])
      expect(w.get('[data-test="provider-pane"]').text()).toContain('gemma')
      expect(w.find('[data-test="provider-pane"] a[href="/models/engine"]').exists()).toBe(true)
    })

    it('says there is no local model yet, instead of a filter that matched nothing', async () => {
      models.state.value = stateWith([CLOUD])
      const { w } = await mountModels('/models/providers/local')
      expect(w.get('[data-test="provider-pane"]').text()).toContain('No local model yet')
      expect(w.get('[data-test="provider-pane"]').text()).not.toContain('No model matches')
    })

    it('asks nothing of the configuration to show every model', async () => {
      vi.mocked(AdminApiService.fetchState).mockRejectedValue(new Error('no config'))
      const { w } = await mountModels()
      expect(cardNames(w)).toEqual(['llama', 'qwen', 'gpt'])
    })

    describe('on a phone', () => {
      beforeEach(() => {
        mobile.value = true
      })

      it('opens on All models with one Providers row that leads to the list', async () => {
        const { w } = await mountModels()
        expect(w.find('nav[aria-label="Providers"]').exists()).toBe(false)
        expect(cardNames(w)).toEqual(['llama', 'qwen', 'gpt'])
        expect(w.get('[data-test="providers-link"]').attributes('href')).toBe('/models/providers')
      })

      it('shows the providers as a screen of their own', async () => {
        const { w } = await mountModels('/models/providers')
        expect(providerLinks(w).length).toBe(7)
        expect(w.find('[data-test="model-cards"]').exists()).toBe(false)
      })

      it('shows one provider with a way back to the list, and no rail', async () => {
        withKey()
        const { w } = await mountModels('/models/providers/openai')
        expect(w.find('nav[aria-label="Providers"]').exists()).toBe(false)
        expect(w.get('[data-test="provider-pane"]').text()).toContain('OpenAI / Compatible')
        expect(w.get('[data-test="back-to-providers"]').attributes('href')).toBe('/models/providers')
      })
    })
  })

  describe('Engine and Routing', () => {
    it('saves the engine settings with the shared configuration', async () => {
      const { w } = await mountModels('/models/engine')
      expect((fieldByLabel(w, /model host/i).element as HTMLInputElement).value).toBe('10.0.0.2')
      await fieldByLabel(w, /model host/i).setValue('0.0.0.0')
      await button(w, /^save model settings$/i).trigger('click')
      await flushPromises()
      expect(AdminApiService.updateConfig).toHaveBeenCalledWith(expect.objectContaining({ primary_model: 'gpt', model_host: '0.0.0.0' }))
      expect(toast.success).toHaveBeenCalled()
    })

    it('keeps an edit across tabs and asks before leaving the page with it', async () => {
      const { w, router } = await mountModels('/models/routing')
      await fieldByLabel(w, /local runs/i).setValue('2')
      await router.push('/models/engine')
      expect(confirm).not.toHaveBeenCalled()
      confirm.mockResolvedValueOnce(false)
      await router.push('/dashboard')
      expect(confirm).toHaveBeenCalledTimes(1)
      expect(router.currentRoute.value.fullPath).toBe('/models/engine')
    })

    it('lists the files found on disk that are not added yet, on This computer', async () => {
      models.availableModels.value = [
        { name: 'qwen', filename: 'qwen.gguf', size_bytes: 1 },
        { name: 'gemma', filename: 'gemma.gguf', size_bytes: 1, metadata: { name: 'Gemma 4' } },
      ]
      const { w } = await mountModels('/models/providers/local')
      expect(button(w, 'Add Gemma 4')).toBeDefined()
      expect(w.findAll('button').some((b) => b.text() === 'Add qwen')).toBe(false)
      expect(w.get('[data-test="added-count"]').text()).toContain('1 more file is added already')
    })
  })
})
