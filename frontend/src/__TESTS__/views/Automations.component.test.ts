import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { ref } from 'vue'
import { createMemoryHistory, type Router } from 'vue-router'
import { createAppRouter } from '../../router'
import { useConfirm } from '../../composables/ui/useConfirm'
import type { Automation } from '../../types/dispatcher'

const AUTO: Automation = {
  id: 'demo/nightly', workspace: 'demo', name: 'nightly', task_file: 'jobs/nightly.md', strategy: 'persistent',
  trigger: 'manual', trigger_value: '', model: '', history: [],
}
const d = {
  automations: ref<Automation[]>([AUTO]),
  metrics: ref(null),
  workspaces: ref([{ id: 'demo' }]),
  workspaceFiles: ref<Record<string, string[]>>({}),
  loading: ref(false),
  fetchAutomations: vi.fn(),
  fetchMetrics: vi.fn(),
  triggerAutomation: vi.fn(),
  fetchWorkspaces: vi.fn(),
  fetchWorkspaceTree: vi.fn(),
  fetchWorkspaceState: vi.fn().mockResolvedValue({ history: [] }),
  fetchGlobalActivity: vi.fn().mockResolvedValue([]),
  createAutomation: vi.fn(),
  updateAutomation: vi.fn(),
  deleteAutomation: vi.fn(),
  stopAutomation: vi.fn(),
  cancelQueued: vi.fn(),
  deleteRun: vi.fn(),
  deleteAutomationRuns: vi.fn(),
}
vi.mock('../../composables/automation/useDispatcher', () => ({ useDispatcher: () => d }))
vi.mock('../../services/automation/dispatcherService', () => ({
  DispatcherService: { getRecordingStatus: vi.fn().mockResolvedValue({ enabled: true, dir: '/rec' }) },
}))

import AutomationsView from '../../views/AutomationsView.vue'

const STUBS = {
  Icon: true,
  BrandMark: true,
  AutomationDetails: { props: ['automation'], template: '<div data-test="details">{{ automation.name }}</div>' },
  AutomationForm: { props: ['editAutomation'], template: '<div data-test="form">{{ editAutomation?.name ?? "new" }}</div>' },
  RecordingsPanel: { template: '<div data-test="recordings" />' },
  MonitorPanel: true,
}

const mounted: VueWrapper[] = []
async function mountAt(path: string): Promise<{ w: VueWrapper; router: Router }> {
  const router = createAppRouter(createMemoryHistory())
  await router.push(path)
  const w = mount(AutomationsView, { global: { plugins: [router], stubs: STUBS }, attachTo: document.body })
  mounted.push(w)
  await flushPromises()
  return { w, router }
}

// One page per route (plan Phase 5): list, one automation, new / edit, recordings.
describe('Automations', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    d.fetchAutomations.mockResolvedValue(undefined)
    d.automations.value = [AUTO]
  })
  afterEach(() => {
    mounted.splice(0).forEach((w) => w.unmount())
    document.body.innerHTML = ''
  })

  it('lists the fleet at /automations with links to create and to recordings', async () => {
    const { w } = await mountAt('/automations')
    expect(w.get('h1').text()).toBe('Automations')
    expect(w.text()).toContain('nightly')
    expect(w.find('a[href="/automations/new"]').exists()).toBe(true)
    expect(w.find('a[href="/automations/recordings"]').exists()).toBe(true)
  })

  it('shows one automation with its actions', async () => {
    const { w } = await mountAt('/automations/demo%2Fnightly')
    expect(w.get('h1').text()).toBe('nightly')
    expect(w.get('[data-test="details"]').text()).toBe('nightly')
    expect(w.find('a[href="/automations/demo%2Fnightly/edit"]').exists()).toBe(true)
  })

  it('deletes the automation only after confirming, then returns to the list', async () => {
    d.deleteAutomation.mockResolvedValue(undefined)
    const { w, router } = await mountAt('/automations/demo%2Fnightly')
    await w.get('button[aria-label="Delete nightly"]').trigger('click')
    await flushPromises()
    expect(d.deleteAutomation).not.toHaveBeenCalled()
    useConfirm().handleConfirm()
    await flushPromises()
    expect(d.deleteAutomation).toHaveBeenCalledWith('demo', 'nightly')
    expect(router.currentRoute.value.path).toBe('/automations')
  })

  it('runs an automation from the list and opens it', async () => {
    d.triggerAutomation.mockResolvedValue({ status: 'started' })
    const { w, router } = await mountAt('/automations')
    await w.get('button[aria-label="Run nightly"]').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/automations/demo%2Fnightly')
    expect(d.triggerAutomation).toHaveBeenCalledWith('demo', 'nightly')
  })

  it('opens the form for new and edit', async () => {
    expect((await mountAt('/automations/new')).w.get('[data-test="form"]').text()).toBe('new')
    expect((await mountAt('/automations/demo%2Fnightly/edit')).w.get('[data-test="form"]').text()).toBe('nightly')
  })

  it('says when an automation does not exist', async () => {
    const { w } = await mountAt('/automations/demo%2Fgone')
    expect(w.text()).toContain('No automation demo/gone')
    expect(w.find('a[href="/automations"]').exists()).toBe(true)
  })

  it('shows recordings at /automations/recordings', async () => {
    const { w } = await mountAt('/automations/recordings')
    expect(w.find('[data-test="recordings"]').exists()).toBe(true)
  })
})
