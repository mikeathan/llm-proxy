import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { ref, shallowRef } from 'vue'
import { createMemoryHistory, type Router } from 'vue-router'
import { createAppRouter } from '../../router'
import type { WorkspaceTree } from '../../types/workspace'

const TREE: WorkspaceTree = { entries: [{ path: 'plan.md', type: 'file' }], truncated: false }
const d = {
  metrics: ref(null),
  workspaces: ref([{ id: 'demo' }, { id: 'lab' }]),
  workspaceTrees: shallowRef<Record<string, WorkspaceTree>>({ demo: TREE, lab: TREE }),
  loading: ref(false),
  fetchMetrics: vi.fn(),
  fetchWorkspaces: vi.fn(),
  fetchWorkspaceTree: vi.fn(),
  fetchWorkspaceState: vi.fn().mockResolvedValue({ history: [] }),
  fetchGlobalActivity: vi.fn().mockResolvedValue([]),
  createWorkspace: vi.fn(),
  deleteWorkspaceFile: vi.fn(),
  deleteWorkspace: vi.fn(),
  deleteRun: vi.fn(),
  deleteAutomationRuns: vi.fn(),
}
vi.mock('../../composables/automation/useDispatcher', () => ({ useDispatcher: () => d }))
vi.mock('../../composables/models/useModels', () => ({ useModels: () => ({ state: ref({ config: { guardrails: {} } }) }) }))
vi.mock('../../composables/assistant/useAssistant', () => ({
  useAssistant: () => ({ runningSessions: ref([]), reconcileRunning: vi.fn(), reconcileRunningConversation: vi.fn() }),
}))
vi.mock('../../composables/assistant/useRunningActivity', () => ({
  useRunningActivity: () => ({ assistantRunning: ref(false), assistantConversationId: ref(''), assistantQueued: ref(false) }),
}))
vi.mock('../../composables/assistant/useGlobalRunActivity', () => ({ useGlobalRunActivity: () => ({ laneHolders: ref([]) }) }))
vi.mock('../../services/automation/dispatcherService', () => ({
  DispatcherService: {
    getAllWorkspaceConfigs: vi.fn().mockResolvedValue({}),
    readWorkspaceFile: vi.fn().mockResolvedValue('# Plan'),
    writeWorkspaceFile: vi.fn(),
  },
}))

import WorkspacesView from '../../views/WorkspacesView.vue'

const STUBS = {
  Icon: true,
  BrandMark: true,
  AssistantChat: { template: '<div data-test="chat" />' },
  WorkspaceSettings: { template: '<div data-test="settings" />' },
  MemoryPanel: { template: '<div data-test="memory" />' },
  TemplateLibrary: { props: ['appendTarget'], template: '<div data-test="playbooks">{{ appendTarget }}</div>' },
  MonitorPanel: true,
}

const mounted: VueWrapper[] = []
async function mountAt(path: string): Promise<{ w: VueWrapper; router: Router }> {
  const router = createAppRouter(createMemoryHistory())
  await router.push(path)
  const w = mount(WorkspacesView, { global: { plugins: [router], stubs: STUBS }, attachTo: document.body })
  mounted.push(w)
  await flushPromises()
  return { w, router }
}

describe('Workspaces', () => {
  beforeEach(() => {
    d.createWorkspace.mockReset()
    d.fetchWorkspaces.mockReset().mockResolvedValue(undefined)
  })
  afterEach(() => {
    mounted.splice(0).forEach((w) => w.unmount())
    document.body.innerHTML = ''
  })

  it('lists the workspaces at /workspaces', async () => {
    const { w } = await mountAt('/workspaces')
    expect(w.get('h1').text()).toBe('Workspaces')
    expect(w.findAll('tbody a').map((a) => a.text())).toEqual(['demo', 'lab'])
  })

  it('opens a new workspace once it has been created', async () => {
    d.createWorkspace.mockResolvedValue(undefined)
    const { w, router } = await mountAt('/workspaces')
    await w.get('input[name="workspace-name"]').setValue('research')
    await w.get('form').trigger('submit')
    await flushPromises()
    expect(d.createWorkspace).toHaveBeenCalledWith('research')
    expect(router.currentRoute.value.path).toBe('/workspaces/research/files')
  })

  it('stays on the list when creating fails', async () => {
    d.createWorkspace.mockImplementation(async () => {
      throw new Error('exists')
    })
    const { w, router } = await mountAt('/workspaces')
    await w.get('input[name="workspace-name"]').setValue('demo')
    await w.get('form').trigger('submit')
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/workspaces')
  })

  it('says when the workspace in the URL does not exist', async () => {
    const { w } = await mountAt('/workspaces/ghost/files')
    expect(w.get('[role="alert"]').text()).toContain('No workspace ghost')
  })

  it('shows the tree and the editor for a file link', async () => {
    const { w } = await mountAt('/workspaces/demo/files/plan.md')
    expect(w.find('[role="tree"]').exists()).toBe(true)
    expect((w.get('textarea').element as HTMLTextAreaElement).value).toBe('# Plan')
    const save = w.findAll('button').find((b) => b.text() === 'Save')!
    expect(save.attributes('disabled')).toBeDefined()
    await w.get('textarea').setValue('# Plan v2')
    expect(w.text()).toContain('Unsaved')
    expect(save.attributes('disabled')).toBeUndefined()
  })

  it('switches workspace within the same section', async () => {
    const { w, router } = await mountAt('/workspaces/demo/memory')
    expect(w.find('[data-test="memory"]').exists()).toBe(true)
    await w.get('select[aria-label="Workspace"]').setValue('lab')
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/workspaces/lab/memory')
  })

  it('offers appending playbooks to the open file of this workspace', async () => {
    const { w, router } = await mountAt('/workspaces/demo/files/plan.md')
    await router.push('/workspaces/demo/playbooks')
    await flushPromises()
    expect(w.get('[data-test="playbooks"]').text()).toBe('plan.md')
  })
})
