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
  CommunicationSettings: true,
  SearchSettings: true,
  GuardrailSettings: true,
  AppearanceSettings: true,
  AgentsSettings: true,
}

const LOADED: GlobalConfig = { ...structuredClone(DEFAULT_CONFIG), primary_model: 'qwen', workspaces_dir: '/srv/ws' }

// Through a RouterView, as the app renders it, so route guards take part.
async function mountSettings(path = '/settings/storage') {
  const router = createAppRouter(createMemoryHistory())
  await router.push(path)
  const w = mount(defineComponent({ render: () => h(RouterView) }), { global: { plugins: [router], stubs: STUBS }, attachTo: document.body })
  await vi.waitFor(() => expect(w.find('h1').exists()).toBe(true))
  await flushPromises()
  return { w, router }
}

type Mounted = Awaited<ReturnType<typeof mountSettings>>['w']
const buttonByText = (w: Mounted, text: RegExp) => w.findAll('button').find((b) => text.test(b.text()))!
const workspacesDir = (w: Mounted) => fieldByLabel(w, /workspaces directory/i)

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

  it('loads the configuration on mount', async () => {
    const { w } = await mountSettings()
    expect(AdminApiService.fetchState).toHaveBeenCalledTimes(1)
    expect((workspacesDir(w).element as HTMLInputElement).value).toBe('/srv/ws')
  })

  it('shows the four tabs, marks the current one, and lists the sections of a group', async () => {
    const { w } = await mountSettings('/settings/search')
    const tabs = w.get('nav[aria-label="Settings"]')
    expect(tabs.findAll('a').map((a) => a.text())).toEqual(['General', 'Safety', 'Connectors', 'Appearance'])
    expect(tabs.findAll('a[aria-current="page"]').map((a) => a.text())).toEqual(['Connectors'])
    const sections = w.get('nav[aria-label="Connectors sections"]')
    expect(sections.findAll('a').map((a) => a.attributes('href'))).toEqual(['/settings/communication', '/settings/search'])
    expect(sections.findAll('a[aria-current="page"]').map((a) => a.attributes('href'))).toEqual(['/settings/search'])
  })

  it('splits General into short sections, each its own page', async () => {
    const { w } = await mountSettings('/settings/memory')
    const row = w.get('nav[aria-label="General sections"]')
    expect(row.findAll('a').map((a) => a.text())).toEqual(['Agents', 'Memory & reminders', 'Storage & logging'])
    expect(row.get('a[aria-current="page"]').attributes('href')).toBe('/settings/memory')
    const visible = w.findAll('section').filter((p) => p.isVisible()).map((p) => p.text())
    expect(visible.some((t) => /reminders/i.test(t))).toBe(true)
    expect(visible.some((t) => /workspace storage/i.test(t))).toBe(false)
  })

  it('keeps every section reachable from the tabs, and nothing about models', async () => {
    const { w } = await mountSettings('/settings/memory')
    const hrefs = w.findAll('nav a').map((a) => a.attributes('href'))
    expect(hrefs).toEqual(expect.arrayContaining(['/settings/agents', '/settings/memory', '/settings/storage', '/settings/security', '/settings/communication', '/settings/appearance']))
    for (const moved of ['/settings/local', '/settings/runs', '/settings/local-models', '/settings/processes', '/settings/openai']) expect(hrefs).not.toContain(moved)
  })

  // Everything about models moved to the Models page (Constitution III.8 as amended 2026-11-26); an old link lands on the tab that holds it.
  it.each([
    ['/settings/local-models', '/models'],
    ['/settings/local', '/models/engine'],
    ['/settings/processes', '/models/engine'],
    ['/settings/runs', '/models/routing'],
    ['/settings/openai', '/models/providers'],
    ['/settings/nvidia', '/models/providers'],
  ])('sends %s to %s', async (from, to) => {
    const router = createAppRouter(createMemoryHistory())
    await router.push(from)
    expect(router.currentRoute.value.fullPath).toBe(to)
  })

  it('offers Save only once something changed, then saves the whole configuration', async () => {
    const { w } = await mountSettings()
    expect(buttonByText(w, /^save settings$/i).attributes('disabled')).toBeDefined()
    await workspacesDir(w).setValue('/data/ws')
    expect(w.text()).toContain('Unsaved changes')
    await buttonByText(w, /^save settings$/i).trigger('click')
    await flushPromises()
    expect(AdminApiService.updateConfig).toHaveBeenCalledWith(expect.objectContaining({ primary_model: 'qwen', workspaces_dir: '/data/ws' }))
    expect(toast.success).toHaveBeenCalled()
    expect(w.text()).not.toContain('Unsaved changes')
  })

  it('shows a failed save next to Save and keeps the edit', async () => {
    vi.mocked(AdminApiService.updateConfig).mockRejectedValue(new Error('disk full'))
    const { w } = await mountSettings()
    await workspacesDir(w).setValue('/data/ws')
    await buttonByText(w, /^save settings$/i).trigger('click')
    await flushPromises()
    expect(w.findAll('[role="alert"]').some((a) => a.text().includes('disk full'))).toBe(true)
    expect((workspacesDir(w).element as HTMLInputElement).value).toBe('/data/ws')
  })

  it('discards edits back to the loaded values', async () => {
    const { w } = await mountSettings()
    await workspacesDir(w).setValue('/data/ws')
    await buttonByText(w, /discard changes/i).trigger('click')
    await flushPromises()
    expect((workspacesDir(w).element as HTMLInputElement).value).toBe('/srv/ws')
    expect(w.text()).not.toContain('Unsaved changes')
  })

  it('keeps edits across categories and asks before leaving Settings with them', async () => {
    const { w, router } = await mountSettings()
    await workspacesDir(w).setValue('/data/ws')
    await router.push('/settings/search')
    expect(confirm).not.toHaveBeenCalled()
    await router.push('/settings/storage')
    await flushPromises()
    expect((workspacesDir(w).element as HTMLInputElement).value).toBe('/data/ws')

    confirm.mockResolvedValueOnce(false)
    await router.push('/dashboard')
    expect(confirm).toHaveBeenCalledTimes(1)
    expect(router.currentRoute.value.fullPath).toBe('/settings/storage')
  })

  it('offers a retry when the configuration cannot be loaded', async () => {
    vi.mocked(AdminApiService.fetchState).mockRejectedValueOnce(new Error('offline'))
    const { w } = await mountSettings()
    expect(w.text()).toContain('Could not load settings')
    expect(w.text()).toContain('offline')
    await buttonByText(w, /retry/i).trigger('click')
    await flushPromises()
    expect(workspacesDir(w).exists()).toBe(true)
  })

  it('opens Appearance as a tab of its own', async () => {
    const { w } = await mountSettings('/settings/appearance')
    const current = w.get('nav[aria-label="Settings"] a[aria-current="page"]')
    expect(current.text()).toBe('Appearance')
    expect(current.attributes('href')).toBe('/settings/appearance')
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
