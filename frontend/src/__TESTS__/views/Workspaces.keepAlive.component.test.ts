import { describe, it, expect, vi, afterEach } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { defineComponent, h, KeepAlive, onMounted, onUnmounted, ref, shallowRef } from 'vue'
import { createMemoryHistory, RouterView } from 'vue-router'
import { createAppRouter } from '../../router'
import type { WorkspaceTree } from '../../types/workspace'

// The Workspaces destination is kept alive when the operator moves elsewhere. Its
// assistant chat owns a live connection to the run (each useAssistant() call does),
// so leaving must close it — otherwise the ask panel, which opens a chat of its own
// on any other page, would run a second one against the same conversation.

const TREE: WorkspaceTree = { entries: [], truncated: false }
const d = {
  metrics: ref(null),
  workspaces: ref([{ id: 'demo' }]),
  workspaceTrees: shallowRef<Record<string, WorkspaceTree>>({ demo: TREE }),
  loading: ref(false),
  fetchMetrics: vi.fn(),
  fetchWorkspaces: vi.fn().mockResolvedValue(undefined),
  fetchWorkspaceTree: vi.fn(),
  fetchWorkspaceState: vi.fn().mockResolvedValue({ history: [] }),
  fetchGlobalActivity: vi.fn().mockResolvedValue([]),
  createWorkspace: vi.fn(),
  deleteWorkspacePaths: vi.fn(async () => []),
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
  DispatcherService: { getAllWorkspaceConfigs: vi.fn().mockResolvedValue({}), readWorkspaceFile: vi.fn().mockResolvedValue(''), writeWorkspaceFile: vi.fn() },
}))

const lifecycle = { mounted: 0, unmounted: 0 }
const Chat = defineComponent({
  setup() {
    onMounted(() => lifecycle.mounted++)
    onUnmounted(() => lifecycle.unmounted++)
    return () => h('div', { 'data-test': 'chat' })
  },
})

const STUBS = { Icon: true, BrandMark: true, AssistantChat: Chat, MonitorPanel: true }
const openChats = () => lifecycle.mounted - lifecycle.unmounted

// The shell's own arrangement: the routed view inside a KeepAlive that keeps Workspaces.
const Shell = defineComponent({
  setup: () => () => h(RouterView, null, { default: ({ Component }: { Component: never }) => h(KeepAlive, { include: ['WorkspacesView'] }, () => (Component ? h(Component) : null)) }),
})

const mounted: VueWrapper[] = []
async function mountShell(path: string) {
  const router = createAppRouter(createMemoryHistory())
  router.addRoute({ path: '/elsewhere', component: { render: () => h('div', 'elsewhere') }, meta: { destination: 'reminders' } })
  await router.push(path)
  const w = mount(Shell, { global: { plugins: [router], stubs: STUBS }, attachTo: document.body })
  mounted.push(w)
  await flushPromises()
  await vi.waitFor(() => expect(w.find('[data-test="chat"]').exists()).toBe(true))
  return { w, router }
}

describe('Workspaces assistant chat across navigation', () => {
  afterEach(() => {
    mounted.splice(0).forEach((w) => w.unmount())
    document.body.innerHTML = ''
    lifecycle.mounted = 0
    lifecycle.unmounted = 0
  })

  it('closes the chat when the operator leaves, and opens a fresh one on return', async () => {
    const { w, router } = await mountShell('/workspaces/demo/assistant')
    expect(openChats()).toBe(1)

    await router.push('/elsewhere')
    await flushPromises()
    expect(openChats()).toBe(0)
    expect(w.find('[data-test="chat"]').exists()).toBe(false)

    await router.push('/workspaces/demo/assistant')
    await flushPromises()
    await vi.waitFor(() => expect(openChats()).toBe(1))
  })

  it('never has two chats open when moving between sections of the same workspace', async () => {
    const { router } = await mountShell('/workspaces/demo/assistant')
    await router.push('/workspaces/demo/files')
    await flushPromises()
    expect(openChats()).toBe(0)
    await router.push('/workspaces/demo/assistant')
    await flushPromises()
    expect(openChats()).toBe(1)
  })
})
