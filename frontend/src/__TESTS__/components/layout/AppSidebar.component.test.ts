import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory } from 'vue-router'
import { createAppRouter } from '../../../router'
import AppSidebar from '../../../components/layout/AppSidebar.vue'
import { memoryStorage } from '../../helpers/memoryStorage'

vi.mock('../../../services/admin/adminService', () => ({
  AdminApiService: { fetchVersion: () => Promise.resolve({ version: 'v1.2.3' }) },
}))

const DESTINATIONS = ['Overview', 'Workspaces', 'Automations', 'Models', 'Activity', 'Settings']

async function mountAt(path: string, props: Record<string, unknown> = {}) {
  const router = createAppRouter(createMemoryHistory())
  await router.push(path)
  const wrapper = mount(AppSidebar, {
    props: { mobile: false, open: false, ...props },
    global: { plugins: [router], stubs: { Icon: true } },
  })
  await flushPromises()
  return { wrapper, router }
}

const navLinks = (wrapper: Awaited<ReturnType<typeof mountAt>>['wrapper']) => wrapper.findAll('nav a')

describe('AppSidebar', () => {
  beforeEach(() => vi.stubGlobal('localStorage', memoryStorage()))
  afterEach(() => vi.unstubAllGlobals())

  it('links the six destinations in order under the versioned brand', async () => {
    const { wrapper } = await mountAt('/overview')
    expect(wrapper.text()).toContain('v1.2.3')
    expect(navLinks(wrapper).map((a) => a.attributes('aria-label'))).toEqual(DESTINATIONS)
    expect(navLinks(wrapper).every((a) => a.attributes('href'))).toBe(true)
  })

  it('marks the destination owning a nested route as current', async () => {
    const { wrapper } = await mountAt('/workspaces/ws/files/a.md')
    const current = navLinks(wrapper).filter((a) => a.attributes('aria-current') === 'page')
    expect(current.map((a) => a.attributes('aria-label'))).toEqual(['Workspaces'])
  })

  it('marks nothing current on the not-found page', async () => {
    const { wrapper } = await mountAt('/missing')
    expect(navLinks(wrapper).some((a) => a.attributes('aria-current'))).toBe(false)
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

  it('as a mobile drawer, closes on Escape, on the scrim and after navigating', async () => {
    const { wrapper, router } = await mountAt('/overview', { mobile: true, open: true })
    expect(wrapper.find('[data-test="sidebar-collapse"]').exists()).toBe(false)

    await wrapper.get('aside').trigger('keydown', { key: 'Escape' })
    await wrapper.get('[data-test="drawer-scrim"]').trigger('click')
    await router.push('/models')
    await flushPromises()
    expect(wrapper.emitted('update:open')).toEqual([[false], [false], [false]])
  })

  it('hides the closed drawer from assistive tech', async () => {
    const { wrapper } = await mountAt('/overview', { mobile: true, open: false })
    expect(wrapper.get('aside').attributes('inert')).toBeDefined()
    expect(wrapper.find('[data-test="drawer-scrim"]').exists()).toBe(false)
  })

  it('badges destinations with unread run notifications, in the accessible name too', async () => {
    const { wrapper } = await mountAt('/overview', { badges: { automations: 2 } })
    const link = wrapper.get('nav a[href="/automations"]')
    expect(link.attributes('aria-label')).toBe('Automations, 2 unread run notifications')
    expect(link.find('.notif-count').text()).toBe('2')
  })
})
