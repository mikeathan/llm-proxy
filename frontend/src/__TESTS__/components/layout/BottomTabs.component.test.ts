import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createMemoryHistory } from 'vue-router'
import { createAppRouter } from '../../../router'
import BottomTabs from '../../../components/layout/BottomTabs.vue'
import { useLastWorkspace } from '../../../composables/ui/useLastWorkspace'
import { memoryStorage } from '../../helpers/memoryStorage'
import { resetBriefUnread } from '../../../composables/newsroom/useBriefUnread'
import { resetAlertsUnread } from '../../../composables/newsroom/useAlertsUnread'

// The shell asks which briefs wait unread (a dot on More, since Newsroom sits under it); the tests answer it instead of reaching a server.
const briefs = vi.hoisted(() => ({ latest: [] as { edition_id: number; name: string; issue_id: number; made_at: string; read: boolean }[] }))
vi.mock('../../../services/newsroom/briefService', () => ({ BriefService: { latest: () => Promise.resolve(briefs.latest) } }))
// The sidebar also counts unread alerts; no server here, so the count is none.
vi.mock('../../../services/newsroom/newsroomService', () => ({ NewsroomService: { alertCounts: () => Promise.resolve({ unread: 0, leftOut: 0 }) } }))

const mounted: VueWrapper[] = []
async function mountAt(path: string, props: Record<string, unknown> = {}) {
  const router = createAppRouter(createMemoryHistory())
  await router.push(path)
  const wrapper = mount(BottomTabs, { props, global: { plugins: [router], stubs: { Icon: true } }, attachTo: document.body })
  mounted.push(wrapper)
  await flushPromises()
  return { wrapper, router }
}
const tabs = (w: VueWrapper) => w.findAll('nav[aria-label="Primary"] > a')
const more = (w: VueWrapper) => w.get('button[aria-haspopup="dialog"]')

describe('BottomTabs', () => {
  beforeEach(() => {
    vi.stubGlobal('localStorage', memoryStorage())
    useLastWorkspace().remember('demo')
  })
  afterEach(() => {
    mounted.splice(0).forEach((w) => w.unmount())
    resetBriefUnread()
    resetAlertsUnread()
    briefs.latest = []
    document.body.innerHTML = ''
    useLastWorkspace().forget('demo')
    vi.unstubAllGlobals()
  })

  it('has Dashboard, Assistant, Workspace and Automations, then More', async () => {
    const { wrapper } = await mountAt('/models')
    expect(tabs(wrapper).map((a) => a.attributes('aria-label'))).toEqual(['Dashboard', 'Assistant', 'Workspace', 'Automations'])
    expect(tabs(wrapper).map((a) => a.attributes('href'))).toEqual(['/dashboard?board=home', '/workspaces/demo/assistant', '/workspaces/demo/files', '/automations'])
    expect(more(wrapper).text()).toContain('More')
  })

  it('puts a dot on Dashboard while a brief waits unread, since the brief is read there (ADR-0089)', async () => {
    briefs.latest = [{ edition_id: 1, name: 'Morning', issue_id: 4, made_at: '2026-11-24T06:00:00Z', read: false }]
    const { wrapper } = await mountAt('/models')
    const dashboard = tabs(wrapper).find((a) => a.attributes('aria-label')?.startsWith('Dashboard'))!
    expect(dashboard.find('[data-test="nav-dot"]').exists()).toBe(true)
    expect(dashboard.attributes('aria-label')).toMatch(/^Dashboard, .*brief/i)
    expect(more(wrapper).find('[data-test="nav-dot"]').exists()).toBe(false)
  })

  it('marks the current tab', async () => {
    const { wrapper } = await mountAt('/workspaces/demo/assistant')
    expect(tabs(wrapper).filter((a) => a.attributes('aria-current') === 'page').map((a) => a.attributes('aria-label'))).toEqual(['Assistant'])
  })

  it('keeps the rest under a More sheet that is closed until asked for', async () => {
    const { wrapper } = await mountAt('/automations')
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false)
    expect(more(wrapper).attributes('aria-expanded')).toBe('false')
    await more(wrapper).trigger('click')
    const sheet = wrapper.get('[role="dialog"]')
    expect(sheet.findAll('a').map((a) => a.attributes('aria-label'))).toEqual(['Network', 'Agents', 'Radio', 'Reminders', 'Newsroom', 'Models', 'Activity', 'Settings'])
    expect(more(wrapper).attributes('aria-expanded')).toBe('true')
  })

  it('shows Network, Agents, Radio, Reminders and Newsroom under a Stacks heading in the More sheet', async () => {
    const { wrapper } = await mountAt('/automations')
    await more(wrapper).trigger('click')
    const sheet = wrapper.get('[role="dialog"]')
    expect(sheet.get('[data-test="sheet-heading-stacks"]').text()).toBe('Stacks')
    expect(sheet.findAll('[role="group"][aria-labelledby="sheet-heading-stacks"] a').map((a) => a.attributes('aria-label'))).toEqual(['Network', 'Agents', 'Radio', 'Reminders', 'Newsroom'])
  })

  it('closes the sheet on Escape, on the scrim and after choosing a destination', async () => {
    const { wrapper, router } = await mountAt('/automations')
    await more(wrapper).trigger('click')
    await wrapper.get('[role="dialog"]').trigger('keydown', { key: 'Escape' })
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false)

    await more(wrapper).trigger('click')
    await wrapper.get('[data-test="more-scrim"]').trigger('click')
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false)

    await more(wrapper).trigger('click')
    await wrapper.get('[role="dialog"] a[href="/settings"]').trigger('click')
    await vi.waitFor(() => expect(router.currentRoute.value.path).toBe('/settings'))
    await flushPromises()
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false)
  })

  it('shows More as current while a destination under it is open', async () => {
    const { wrapper } = await mountAt('/settings/local')
    expect(more(wrapper).attributes('data-current')).toBe('true')
    await more(wrapper).trigger('click')
    expect(wrapper.get('[role="dialog"] a[aria-current="page"]').attributes('aria-label')).toBe('Settings')
  })

  it('carries unread run badges, counting More for the entries under it', async () => {
    const { wrapper } = await mountAt('/models', { badges: { automations: 2, activity: 1 } })
    expect(tabs(wrapper)[3]!.attributes('aria-label')).toBe('Automations, 2 unread run notifications')
    expect(more(wrapper).attributes('aria-label')).toBe('More, 1 unread run notification')
  })
})
