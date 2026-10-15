import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory } from 'vue-router'
import { createAppRouter } from '../../../router'
import AppSidebar from '../../../components/layout/AppSidebar.vue'
import { useLastWorkspace } from '../../../composables/ui/useLastWorkspace'
import { memoryStorage } from '../../helpers/memoryStorage'

vi.mock('../../../services/admin/adminService', () => ({
  AdminApiService: { fetchVersion: () => Promise.resolve({ version: 'v1.2.3' }) },
}))

const DESTINATIONS = ['Assistant', 'Overview', 'Workspace', 'Automations', 'Reminders', 'Models', 'Activity', 'Settings']

async function mountAt(path: string, props: Record<string, unknown> = {}) {
  const router = createAppRouter(createMemoryHistory())
  await router.push(path)
  const wrapper = mount(AppSidebar, {
    props: { ...props },
    global: { plugins: [router], stubs: { Icon: true } },
  })
  await flushPromises()
  return { wrapper, router }
}

const navLinks = (wrapper: Awaited<ReturnType<typeof mountAt>>['wrapper']) => wrapper.findAll('nav a[data-destination]')

describe('AppSidebar', () => {
  beforeEach(() => vi.stubGlobal('localStorage', memoryStorage()))
  afterEach(() => {
    useLastWorkspace().forget('demo')
    vi.unstubAllGlobals()
  })

  it('links the destinations in order, Assistant first, under the versioned brand', async () => {
    const { wrapper } = await mountAt('/overview')
    expect(wrapper.text()).toContain('v1.2.3')
    expect(navLinks(wrapper).map((a) => a.attributes('aria-label'))).toEqual(DESTINATIONS)
    expect(navLinks(wrapper).every((a) => a.attributes('href'))).toBe(true)
  })

  it('opens the assistant and the files of the workspace last used, or the list while none is known', async () => {
    const none = await mountAt('/overview')
    expect(none.wrapper.get('a[data-destination="assistant"]').attributes('href')).toBe('/workspaces')
    useLastWorkspace().remember('demo')
    const known = await mountAt('/overview')
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

  it('links the brand to where the app opens', async () => {
    useLastWorkspace().remember('demo')
    const { wrapper } = await mountAt('/models')
    expect(wrapper.get('a[aria-label$="home"]').attributes('href')).toBe('/workspaces/demo/assistant')
  })

  it('collapses to icons that keep accessible names, and persists the choice', async () => {
    const { wrapper } = await mountAt('/overview')
    const toggle = wrapper.get('[data-test="sidebar-collapse"]')
    expect(toggle.attributes('aria-expanded')).toBe('true')
    await toggle.trigger('click')
    expect(wrapper.get('aside').attributes('data-collapsed')).toBe('true')
    expect(toggle.attributes('aria-expanded')).toBe('false')
    expect(navLinks(wrapper).map((a) => a.attributes('title'))).toEqual(DESTINATIONS)

    const again = await mountAt('/overview')
    expect(again.wrapper.get('aside').attributes('data-collapsed')).toBe('true')
  })

  it('badges entries with unread run notifications, in the accessible name too', async () => {
    const { wrapper } = await mountAt('/overview', { badges: { automations: 2, assistant: 1 } })
    const link = wrapper.get('nav a[href="/automations"]')
    expect(link.attributes('aria-label')).toBe('Automations, 2 unread run notifications')
    expect(link.find('.notif-count').text()).toBe('2')
    expect(wrapper.get('a[data-destination="assistant"]').attributes('aria-label')).toBe('Assistant, 1 unread run notification')
  })
})
