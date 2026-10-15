import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { ref } from 'vue'
import { createMemoryHistory, createRouter, type Router } from 'vue-router'
import { memoryStorage } from '../../helpers/memoryStorage'
import { ROUTE_NAMES } from '../../../types/routes'

const workspaces = ref<{ id: string }[]>([])
const fetchWorkspaces = vi.fn()
vi.mock('../../../composables/automation/useDispatcher', () => ({ useDispatcher: () => ({ workspaces, fetchWorkspaces }) }))

import WorkspaceSwitcher from '../../../components/layout/WorkspaceSwitcher.vue'
import { useActiveWorkspace } from '../../../composables/ui/useActiveWorkspace'
import { useLastWorkspace } from '../../../composables/ui/useLastWorkspace'

const empty = { render: () => null }
const ws = { destination: 'workspaces' } as const
function makeRouter(): Router {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/overview', name: ROUTE_NAMES.overview, component: empty, meta: { destination: 'overview' } },
      { path: '/workspaces', name: ROUTE_NAMES.workspaces, component: empty, meta: ws },
      { path: '/workspaces/:ws/files/:path(.*)*', name: ROUTE_NAMES.workspaceFiles, component: empty, meta: ws },
      { path: '/workspaces/:ws/assistant/:conversationId?', name: ROUTE_NAMES.workspaceAssistant, component: empty, meta: ws },
      { path: '/workspaces/:ws/:section(memory|playbooks|heartbeat|settings)', name: ROUTE_NAMES.workspaceSection, component: empty, meta: ws },
      { path: '/settings/:section?', name: ROUTE_NAMES.settings, component: empty, meta: { destination: 'settings', workspaceScope: 'all' } },
    ],
  })
}

const mounted: VueWrapper[] = []
async function mountAt(path: string, mobile = false) {
  const router = makeRouter()
  await router.push(path)
  const w = mount(WorkspaceSwitcher, { props: { mobile }, global: { plugins: [router], stubs: { Icon: true } }, attachTo: document.body })
  mounted.push(w)
  await flushPromises()
  return { w, router }
}
const trigger = (w: VueWrapper) => w.get('button[aria-haspopup="menu"]')
const choices = (w: VueWrapper) => w.findAll('[role="menu"] button')

describe('WorkspaceSwitcher', () => {
  beforeEach(async () => {
    vi.stubGlobal('localStorage', memoryStorage())
    workspaces.value = [{ id: 'alpha' }, { id: 'beta' }]
    fetchWorkspaces.mockReset().mockResolvedValue(undefined)
    useLastWorkspace().forget('alpha')
    useLastWorkspace().forget('beta')
    await useActiveWorkspace().refresh()
  })
  afterEach(() => {
    mounted.splice(0).forEach((w) => w.unmount())
    document.body.innerHTML = ''
    useLastWorkspace().forget('alpha')
    useLastWorkspace().forget('beta')
    vi.unstubAllGlobals()
  })

  it('names the workspace the shell is in', async () => {
    const { w } = await mountAt('/overview')
    expect(trigger(w).text()).toContain('alpha')
    expect(trigger(w).attributes('aria-expanded')).toBe('false')
  })

  it('lists every workspace, marks the current one and links to managing them', async () => {
    const { w } = await mountAt('/overview')
    await trigger(w).trigger('click')
    expect(choices(w).map((b) => b.text())).toEqual(['alpha', 'beta'])
    expect(choices(w).filter((b) => b.attributes('aria-current') === 'true').map((b) => b.text())).toEqual(['alpha'])
    expect(w.get('[role="dialog"] a[href="/workspaces"]').text()).toMatch(/manage workspaces/i)
  })

  it('switches workspace without leaving a page that has nothing to do with it', async () => {
    const { w, router } = await mountAt('/overview')
    await trigger(w).trigger('click')
    await choices(w)[1]!.trigger('click')
    expect(useLastWorkspace().lastWorkspace.value).toBe('beta')
    expect(router.currentRoute.value.path).toBe('/overview')
    expect(w.find('[role="menu"]').exists()).toBe(false)
  })

  it.each([
    ['/workspaces/alpha/files/notes/a.md', '/workspaces/beta/files'],
    ['/workspaces/alpha/assistant/c1', '/workspaces/beta/assistant'],
    ['/workspaces/alpha/memory', '/workspaces/beta/memory'],
  ])('keeps the section when it switches workspace on %s', async (from, to) => {
    const { w, router } = await mountAt(from)
    await trigger(w).trigger('click')
    await choices(w)[1]!.trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.path).toBe(to)
  })

  it('closes on Escape', async () => {
    const { w } = await mountAt('/overview')
    await trigger(w).trigger('click')
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await flushPromises()
    expect(w.find('[role="menu"]').exists()).toBe(false)
  })

  it('says "All workspaces" where the page is not about one, and offers no choice', async () => {
    const { w } = await mountAt('/settings/local')
    expect(w.text()).toContain('All workspaces')
    expect(w.find('button[aria-haspopup="menu"]').exists()).toBe(false)
  })

  it('is absent while there are no workspaces', async () => {
    workspaces.value = []
    useLastWorkspace().forget('alpha')
    const { w } = await mountAt('/overview')
    expect(w.text()).toBe('')
  })
})
