import { describe, it, expect } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory } from 'vue-router'
import { createAppRouter } from '../../../router'
import AdminHeader from '../../../components/layout/AdminHeader.vue'
import { PRODUCT_NAME } from '../../../config/brand'

async function mountAt(path: string, mobile: boolean, drawerOpen = false) {
  const router = createAppRouter(createMemoryHistory())
  await router.push(path)
  const wrapper = mount(AdminHeader, {
    props: { mobile, drawerOpen },
    global: { plugins: [router], stubs: {
        Icon: true,
        RunActivityPill: { template: '<div data-test="run-pill" />' },
        HostStats: { template: '<div data-test="host-stats" />' },
      }, },
  })
  await flushPromises()
  return wrapper
}

describe('AdminHeader (top strip)', () => {
  it('shows the page title and the run pill on every destination', async () => {
    for (const [path, title] of [['/overview', 'Overview'], ['/workspaces/ws/files', 'Workspaces'], ['/nope', 'Not found']]) {
      const wrapper = await mountAt(path!, false)
      expect(wrapper.get('[aria-current="page"]').text()).toBe(title)
      expect(wrapper.find('[data-test="run-pill"]').exists()).toBe(true)
    }
  })

  it('shows the host stats on every destination, between the title and the shell actions', async () => {
    for (const path of ['/overview', '/workspaces/ws/assistant', '/automations']) {
      const wrapper = await mountAt(path, false)
      const order = ['[aria-current="page"]', '[data-test="host-stats"]', '[data-test="run-pill"]'].map((sel) => wrapper.html().indexOf(wrapper.get(sel).html()))
      expect(order).toEqual([...order].sort((a, b) => a - b))
    }
  })

  it('leaves the brand to the rail on desktop', async () => {
    const wrapper = await mountAt('/overview', false)
    expect(wrapper.find('[data-test="drawer-toggle"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain(PRODUCT_NAME)
  })

  it('on mobile, shows the brand and a labelled drawer toggle', async () => {
    const wrapper = await mountAt('/overview', true)
    expect(wrapper.text()).toContain(PRODUCT_NAME)
    const toggle = wrapper.get('[data-test="drawer-toggle"]')
    expect(toggle.attributes('aria-label')).toBe('Open navigation')
    expect(toggle.attributes('aria-expanded')).toBe('false')
    await toggle.trigger('click')
    expect(wrapper.emitted('toggle-drawer')).toHaveLength(1)
  })
})
