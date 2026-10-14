import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createMemoryHistory } from 'vue-router'
import { createAppRouter } from '../../../router'
import BottomTabs from '../../../components/layout/BottomTabs.vue'
import { useLastWorkspace } from '../../../composables/ui/useLastWorkspace'
import { memoryStorage } from '../../helpers/memoryStorage'

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
    document.body.innerHTML = ''
    useLastWorkspace().forget('demo')
    vi.unstubAllGlobals()
  })

  it('has Assistant, Workspace, Automations and Reminders, then More', async () => {
    const { wrapper } = await mountAt('/models')
    expect(tabs(wrapper).map((a) => a.attributes('aria-label'))).toEqual(['Assistant', 'Workspace', 'Automations', 'Reminders'])
    expect(tabs(wrapper).map((a) => a.attributes('href'))).toEqual(['/workspaces/demo/assistant', '/workspaces/demo/files', '/automations', '/reminders'])
    expect(more(wrapper).text()).toContain('More')
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
    expect(sheet.findAll('a').map((a) => a.attributes('aria-label'))).toEqual(['Overview', 'Models', 'Activity', 'Settings'])
    expect(more(wrapper).attributes('aria-expanded')).toBe('true')
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
    expect(tabs(wrapper)[2]!.attributes('aria-label')).toBe('Automations, 2 unread run notifications')
    expect(more(wrapper).attributes('aria-label')).toBe('More, 1 unread run notification')
  })
})
