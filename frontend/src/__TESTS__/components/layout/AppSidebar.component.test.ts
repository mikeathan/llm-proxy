import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory } from 'vue-router'
import { createAppRouter } from '../../../router'
import AppSidebar from '../../../components/layout/AppSidebar.vue'
import { useLastWorkspace } from '../../../composables/ui/useLastWorkspace'
import { memoryStorage } from '../../helpers/memoryStorage'
import { resetBriefUnread } from '../../../composables/newsroom/useBriefUnread'

// The shell asks which briefs wait unread (a dot on Newsroom); the tests answer it instead of reaching a server.
const briefs = vi.hoisted(() => ({ latest: [] as { edition_id: number; name: string; issue_id: number; made_at: string; read: boolean }[] }))
vi.mock('../../../services/newsroom/briefService', () => ({ BriefService: { latest: () => Promise.resolve(briefs.latest) } }))

vi.mock('../../../services/admin/adminService', () => ({
  AdminApiService: { fetchVersion: () => Promise.resolve({ version: 'v1.2.3' }) },
}))

const DESTINATIONS = ['Dashboard', 'Assistant', 'Workspace', 'Automations', 'Network', 'Agents', 'Radio', 'Reminders', 'Newsroom', 'Models', 'Activity', 'Settings']

const mounted: { unmount(): void }[] = []
async function mountAt(path: string, props: Record<string, unknown> = {}) {
  const router = createAppRouter(createMemoryHistory())
  await router.push(path)
  const wrapper = mount(AppSidebar, {
    props: { ...props },
    global: { plugins: [router], stubs: { Icon: true } },
  })
  mounted.push(wrapper)
  await flushPromises()
  return { wrapper, router }
}

const navLinks = (wrapper: Awaited<ReturnType<typeof mountAt>>['wrapper']) => wrapper.findAll('nav a[data-destination]')

describe('AppSidebar', () => {
  beforeEach(() => vi.stubGlobal('localStorage', memoryStorage()))
  afterEach(() => {
    mounted.splice(0).forEach((w) => w.unmount())
    resetBriefUnread()
    briefs.latest = []
    useLastWorkspace().forget('demo')
    vi.unstubAllGlobals()
  })

  it('links the destinations in order, Dashboard first, under the versioned brand', async () => {
    const { wrapper } = await mountAt('/dashboard')
    expect(wrapper.text()).toContain('v1.2.3')
    expect(navLinks(wrapper).map((a) => a.attributes('aria-label'))).toEqual(DESTINATIONS)
    expect(navLinks(wrapper).every((a) => a.attributes('href'))).toBe(true)
  })

  it('shows a plain dot on Newsroom while a brief waits unread, apart from the run-notification count', async () => {
    briefs.latest = [{ edition_id: 1, name: 'Morning', issue_id: 4, made_at: '2026-11-24T06:00:00Z', read: false }]
    const { wrapper } = await mountAt('/dashboard')
    const link = wrapper.get('[data-destination="newsroom"]')
    expect(link.find('[data-test="nav-dot"]').exists()).toBe(true)
    expect(link.attributes('aria-label')).toMatch(/^Newsroom, .*brief/i)
    expect(wrapper.get('[data-destination="dashboard"]').find('[data-test="nav-dot"]').exists()).toBe(false)
  })

  it('shows no dot once every brief is read', async () => {
    briefs.latest = [{ edition_id: 1, name: 'Morning', issue_id: 4, made_at: '2026-11-24T06:00:00Z', read: true }]
    const { wrapper } = await mountAt('/dashboard')
    expect(wrapper.find('[data-test="nav-dot"]').exists()).toBe(false)
    expect(wrapper.get('[data-destination="newsroom"]').attributes('aria-label')).toBe('Newsroom')
  })

  it('keeps Network, Agents and Radio under an expandable Stacks group, open by default and remembered when closed', async () => {
    const { wrapper } = await mountAt('/dashboard')
    const toggle = wrapper.get('[data-test="group-stacks"]')
    expect(toggle.attributes('aria-expanded')).toBe('true')
    expect(wrapper.get('#group-stacks a[data-destination="network"]').attributes('href')).toBe('/network')
    expect(wrapper.get('#group-stacks a[data-destination="agents"]').attributes('href')).toBe('/agents')
    await toggle.trigger('click')
    expect(toggle.attributes('aria-expanded')).toBe('false')
    expect(wrapper.find('a[data-destination="network"]').exists()).toBe(false)

    const again = await mountAt('/dashboard')
    expect(again.wrapper.get('[data-test="group-stacks"]').attributes('aria-expanded')).toBe('false')
  })

  it('opens Stacks and marks it current while a child is open, even if it was closed: its own page', async () => {
    const closed = await mountAt('/dashboard')
    await closed.wrapper.get('[data-test="group-stacks"]').trigger('click')
    const { wrapper } = await mountAt('/network/modules')
    expect(wrapper.get('[data-test="group-stacks"]').attributes('aria-expanded')).toBe('true')
    expect(wrapper.get('a[data-destination="network"]').attributes('aria-current')).toBe('page')
    const board = await mountAt('/network/devices') // the old address opens the card on the dashboard, so the Dashboard is current
    expect(board.wrapper.get('a[data-destination="dashboard"]').attributes('aria-current')).toBe('page')  })

  it('opens the assistant and the files of the workspace last used, or the list while none is known', async () => {
    const none = await mountAt('/dashboard')
    expect(none.wrapper.get('a[data-destination="assistant"]').attributes('href')).toBe('/workspaces')
    useLastWorkspace().remember('demo')
    const known = await mountAt('/dashboard')
    expect(known.wrapper.get('a[data-destination="assistant"]').attributes('href')).toBe('/workspaces/demo/assistant')
    expect(known.wrapper.get('a[data-destination="workspace"]').attributes('href')).toBe('/workspaces/demo/files')
  })

  it('marks the entry owning a nested route as current, telling Assistant from Workspace', async () => {
    const files = await mountAt('/workspaces/ws/files/a.md')
    expect(navLinks(files.wrapper).filter((a) => a.attributes('aria-current') === 'page').map((a) => a.attributes('aria-label'))).toEqual(['Workspace'])
    const chat = await mountAt('/workspaces/ws/assistant/c1')
    expect(navLinks(chat.wrapper).filter((a) => a.attributes('aria-current') === 'page').map((a) => a.attributes('aria-label'))).toEqual(['Assistant'])
  })

  it('marks nothing current on the not-found page', async () => {
    const { wrapper } = await mountAt('/missing')
    expect(navLinks(wrapper).some((a) => a.attributes('aria-current'))).toBe(false)
  })

  it('links the brand to where the app opens: the dashboard, whichever workspace was used last', async () => {
    useLastWorkspace().remember('demo')
    const { wrapper } = await mountAt('/models')
    expect(wrapper.get('a[aria-label$="home"]').attributes('href')).toBe('/dashboard')
  })

  it('collapses to icons that keep accessible names, and persists the choice', async () => {
    const { wrapper } = await mountAt('/dashboard')
    const toggle = wrapper.get('[data-test="sidebar-collapse"]')
    expect(toggle.attributes('aria-expanded')).toBe('true')
    await toggle.trigger('click')
    expect(wrapper.get('aside').attributes('data-collapsed')).toBe('true')
    expect(toggle.attributes('aria-expanded')).toBe('false')
    expect(navLinks(wrapper).map((a) => a.attributes('title'))).toEqual(DESTINATIONS)

    const again = await mountAt('/dashboard')
    expect(again.wrapper.get('aside').attributes('data-collapsed')).toBe('true')
  })

  it('badges entries with unread run notifications, in the accessible name too', async () => {
    const { wrapper } = await mountAt('/dashboard', { badges: { automations: 2, assistant: 1 } })
    const link = wrapper.get('nav a[href="/automations"]')
    expect(link.attributes('aria-label')).toBe('Automations, 2 unread run notifications')
    expect(link.find('.notif-count').text()).toBe('2')
    expect(wrapper.get('a[data-destination="assistant"]').attributes('aria-label')).toBe('Assistant, 1 unread run notification')
  })
})
