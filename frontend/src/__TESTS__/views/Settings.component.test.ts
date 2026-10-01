import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, h, ref } from 'vue'
import { createMemoryHistory, RouterView } from 'vue-router'
import { createAppRouter } from '../../router'
import { AdminApiService } from '../../services/admin/adminService'
import { DEFAULT_CONFIG, useConfig } from '../../composables/models/useConfig'
import type { GlobalConfig } from '../../types/admin'

vi.mock('../../services/admin/adminService', () => ({
  AdminApiService: {
    fetchState: vi.fn(),
    updateConfig: vi.fn(),
    fetchProviderManifests: vi.fn(),
    fetchProviderKeys: vi.fn(),
    saveProviderKeys: vi.fn(),
    deleteAllProviderKeys: vi.fn(),
    testConnection: vi.fn(),
    restartSystem: vi.fn(),
  },
}))

const models = { state: ref(null), availableModels: ref([]), refresh: vi.fn() }
vi.mock('../../composables/models/useModels', () => ({ useModels: () => models }))
vi.mock('../../composables/system/useMcpServers', () => ({
  useMcpServers: () => ({ mcpServers: ref([]), addMCPServer: vi.fn(), toggleMCPServer: vi.fn(), removeMCPServer: vi.fn() }),
}))
const logLevel = { logLevel: ref('info'), updateLogLevel: vi.fn() }
vi.mock('../../composables/system/useMetrics', () => ({ useLogLevel: () => logLevel }))
const toast = { success: vi.fn(), error: vi.fn(), info: vi.fn(), show: vi.fn() }
vi.mock('../../composables/useToast', () => ({ useToast: () => toast }))
const confirm = vi.fn()
vi.mock('../../composables/ui/useConfirm', () => ({ useConfirm: () => ({ confirm }) }))

import { fieldByLabel } from '../helpers/fieldByLabel'

// Sections with their own data layers are characterised in their own tests.
const STUBS = {
  Icon: true,
  BrandMark: true,
  SecuritySettings: true,
  ProviderModelsCard: true,
  InfrastructurePanel: true,
  CommunicationSettings: true,
  SearchSettings: true,
  McpServers: true,
  GuardrailSettings: true,
  AppearanceSettings: true,
}

const LOADED: GlobalConfig = { ...structuredClone(DEFAULT_CONFIG), primary_model: 'qwen', model_host: '10.0.0.2' }

// Through a RouterView, as the app renders it, so route guards take part.
async function mountSettings(path = '/settings/local') {
  const router = createAppRouter(createMemoryHistory())
  await router.push(path)
  const w = mount(defineComponent({ render: () => h(RouterView) }), { global: { plugins: [router], stubs: STUBS }, attachTo: document.body })
  await vi.waitFor(() => expect(w.find('h1').exists()).toBe(true))
  await flushPromises()
  return { w, router }
}

type Mounted = Awaited<ReturnType<typeof mountSettings>>['w']
const buttonByText = (w: Mounted, text: RegExp) => w.findAll('button').find((b) => text.test(b.text()))!
const modelHost = (w: Mounted) => fieldByLabel(w, /model host/i)

// Characterised against Settings.vue first (plan D22), then moved to the
// redesigned page: the same loads, saves and targets, plus the save model.
describe('Settings (global)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    useConfig().config.value = structuredClone(DEFAULT_CONFIG)
    vi.mocked(AdminApiService.fetchState).mockResolvedValue({ config: structuredClone(LOADED) } as never)
    vi.mocked(AdminApiService.fetchProviderManifests).mockResolvedValue([])
    vi.mocked(AdminApiService.fetchProviderKeys).mockResolvedValue([])
    vi.mocked(AdminApiService.updateConfig).mockResolvedValue()
    vi.mocked(AdminApiService.restartSystem).mockResolvedValue()
    confirm.mockResolvedValue(true)
  })
  afterEach(() => {
    document.body.innerHTML = ''
  })

  it('loads the configuration and the model catalogue on mount', async () => {
    const { w } = await mountSettings()
    expect(AdminApiService.fetchState).toHaveBeenCalledTimes(1)
    expect(models.refresh).toHaveBeenCalled()
    expect((modelHost(w).element as HTMLInputElement).value).toBe('10.0.0.2')
  })

  it('links every category, grouped, and marks the current one', async () => {
    const { w } = await mountSettings('/settings/mcp')
    const nav = w.get('nav[aria-label="Settings categories"]')
    for (const group of [/system/i, /cloud providers/i, /extensions/i]) expect(nav.text()).toMatch(group)
    expect(nav.findAll('a[aria-current="page"]').map((a) => a.attributes('href'))).toEqual(['/settings/mcp'])
    expect(nav.findAll('a').map((a) => a.attributes('href'))).toEqual(
      expect.arrayContaining(['/settings/appearance', '/settings/local', '/settings/guardrails', '/settings/openai', '/settings/search']),
    )
  })

  it('offers the categories as a picker on small screens', async () => {
    const { w, router } = await mountSettings('/settings/local')
    const picker = fieldByLabel(w, /^category$/i)
    expect(picker.findAll('optgroup').map((g) => g.attributes('label'))).toEqual(['System', 'Cloud providers', 'Extensions'])
    await picker.setValue('search')
    await vi.waitFor(() => expect(router.currentRoute.value.fullPath).toBe('/settings/search'))
  })

  it('offers Save only once something changed, then saves the whole configuration', async () => {
    const { w } = await mountSettings()
    expect(buttonByText(w, /^save settings$/i).attributes('disabled')).toBeDefined()
    await modelHost(w).setValue('0.0.0.0')
    expect(w.text()).toContain('Unsaved changes')
    await buttonByText(w, /^save settings$/i).trigger('click')
    await flushPromises()
    expect(AdminApiService.updateConfig).toHaveBeenCalledWith(expect.objectContaining({ primary_model: 'qwen', model_host: '0.0.0.0' }))
    expect(toast.success).toHaveBeenCalled()
    expect(models.refresh).toHaveBeenCalledTimes(2)
    expect(w.text()).not.toContain('Unsaved changes')
  })

  it('shows a failed save next to Save and keeps the edit', async () => {
    vi.mocked(AdminApiService.updateConfig).mockRejectedValue(new Error('disk full'))
    const { w } = await mountSettings()
    await modelHost(w).setValue('0.0.0.0')
    await buttonByText(w, /^save settings$/i).trigger('click')
    await flushPromises()
    expect(w.get('[role="alert"]').text()).toContain('disk full')
    expect((modelHost(w).element as HTMLInputElement).value).toBe('0.0.0.0')
  })

  it('discards edits back to the loaded values', async () => {
    const { w } = await mountSettings()
    await modelHost(w).setValue('0.0.0.0')
    await buttonByText(w, /discard changes/i).trigger('click')
    await flushPromises()
    expect((modelHost(w).element as HTMLInputElement).value).toBe('10.0.0.2')
    expect(w.text()).not.toContain('Unsaved changes')
  })

  it('keeps edits across categories and asks before leaving Settings with them', async () => {
    const { w, router } = await mountSettings()
    await modelHost(w).setValue('0.0.0.0')
    await router.push('/settings/search')
    expect(confirm).not.toHaveBeenCalled()
    await router.push('/settings/local')
    await flushPromises()
    expect((modelHost(w).element as HTMLInputElement).value).toBe('0.0.0.0')

    confirm.mockResolvedValueOnce(false)
    await router.push('/overview')
    expect(confirm).toHaveBeenCalledTimes(1)
    expect(router.currentRoute.value.fullPath).toBe('/settings/local')
  })

  it('offers a retry when the configuration cannot be loaded', async () => {
    vi.mocked(AdminApiService.fetchState).mockRejectedValueOnce(new Error('offline'))
    const { w } = await mountSettings()
    expect(w.text()).toContain('Could not load settings')
    expect(w.text()).toContain('offline')
    await buttonByText(w, /retry/i).trigger('click')
    await flushPromises()
    expect(modelHost(w).exists()).toBe(true)
  })

  it('loads the API keys of the provider section it opens on', async () => {
    await mountSettings('/settings/openai')
    expect(AdminApiService.fetchProviderKeys).toHaveBeenCalledWith('openai')
  })

  it('shows the Vertex AI fields on a Gemini deep link even when the stored config has no Gemini entry', async () => {
    const { w } = await mountSettings('/settings/gemini')
    expect(LOADED.providers.gemini).toBeUndefined()
    expect(fieldByLabel(w, /project id/i).exists()).toBe(true)
    expect(w.text()).not.toContain('Unsaved changes')
  })

  it('opens Appearance as a section, first in the list', async () => {
    const { w } = await mountSettings('/settings/appearance')
    const links = w.get('nav[aria-label="Settings categories"]').findAll('a')
    expect(links[0]!.attributes('href')).toBe('/settings/appearance')
    expect(links[0]!.attributes('aria-current')).toBe('page')
    expect(w.findComponent({ name: 'AppearanceSettings' }).isVisible()).toBe(true)
  })

  it('restarts the backend only after confirmation', async () => {
    vi.useFakeTimers({ toFake: ['setTimeout'] })
    const { w } = await mountSettings()
    confirm.mockResolvedValueOnce(false)
    await buttonByText(w, /restart backend/i).trigger('click')
    await flushPromises()
    expect(AdminApiService.restartSystem).not.toHaveBeenCalled()
    await buttonByText(w, /restart backend/i).trigger('click')
    await flushPromises()
    expect(AdminApiService.restartSystem).toHaveBeenCalledTimes(1)
    vi.useRealTimers()
  })
})
