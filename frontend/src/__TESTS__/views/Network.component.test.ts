import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { ref } from 'vue'
import { createMemoryHistory } from 'vue-router'
import { createAppRouter } from '../../router'
import type { Automation } from '../../types/dispatcher'

const automations = ref<Automation[]>([])
vi.mock('../../composables/automation/useDispatcher', () => ({
  useDispatcher: () => ({ automations, fetchAutomations: vi.fn().mockResolvedValue(undefined) }),
}))

import NetworkView from '../../views/NetworkView.vue'

const STUBS = {
  StackModules: { props: ['stack'], template: '<div data-test="stub-modules" :data-stack="stack" />' },
  NetworkSettings: { template: '<div data-test="stub-settings" />' },
  ReadinessBanner: { props: ['here'], template: '<div data-test="stub-readiness" :data-here="here" />' },
  NetauditHelp: { template: '<div data-test="stub-help" />' },
}

async function mountAt(path: string) {
  const router = createAppRouter(createMemoryHistory())
  await router.push(path)
  const wrapper = mount(NetworkView, { global: { plugins: [router], stubs: STUBS } })
  await flushPromises()
  return { wrapper, router }
}

const tabLabels = (w: Awaited<ReturnType<typeof mountAt>>['wrapper']) => w.findAll('a.stack-tab').map((a) => a.text())

describe('NetworkView', () => {
  beforeEach(() => {
    automations.value = []
  })

  it('has only the tabs Modules and Settings: what is to be seen is on the dashboard', async () => {
    const { wrapper } = await mountAt('/network')
    expect(tabLabels(wrapper)).toEqual(['Modules', 'Settings'])
    expect(wrapper.findAll('a.stack-tab').map((a) => a.attributes('href'))).toEqual(['/network/modules', '/network/settings'])
  })

  it.each([
    ['/network', 'stub-modules'], // the page without a tab is Modules
    ['/network/modules', 'stub-modules'],
    ['/network/settings', 'stub-settings'],
  ])('shows one thing on %s', async (path, shown) => {
    const { wrapper } = await mountAt(path)
    const all = ['stub-modules', 'stub-settings']
    expect(all.filter((id) => wrapper.find(`[data-test="${id}"]`).exists())).toEqual([shown])
  })

  it.each([
    ['/network/devices', 'network-devices'],
    ['/network/findings', 'network-findings'],
    ['/network/traffic', 'network-traffic'],
    ['/network/audits', 'network-audits'],
  ])('sends the old address %s to its card on the Network board', async (path, card) => {
    const { router } = await mountAt(path)
    expect(router.currentRoute.value.name).toBe('dashboard')
    expect(router.currentRoute.value.query).toEqual({ board: 'network', card })
  })

  it('keeps the filters of an old findings link when it sends it to the card', async () => {
    const { router } = await mountAt('/network/findings?device=192.0.2.5')
    expect(router.currentRoute.value.query).toEqual({ board: 'network', card: 'network-findings', device: '192.0.2.5' })
  })

  it('keeps Simple/Advanced and Help in the page header, once, on every tab', async () => {
    for (const path of ['/network', '/network/modules', '/network/settings']) {
      const { wrapper } = await mountAt(path)
      expect(wrapper.get('header').findAll('[role="radio"], [role="tab"], button').some((b) => b.attributes('aria-label') === 'Help')).toBe(true)
      expect(wrapper.findAll('button').filter((b) => b.attributes('aria-label') === 'Help')).toHaveLength(1)
    }
  })

  it('tells the readiness banner it is on the settings tab, so it does not link to itself', async () => {
    expect((await mountAt('/network/settings')).wrapper.get('[data-test="stub-readiness"]').attributes('data-here')).toBe('network-scan')
    expect((await mountAt('/network/modules')).wrapper.get('[data-test="stub-readiness"]').attributes('data-here')).toBeUndefined()
  })

  it('links back to its board on the dashboard, where everything to be seen is', async () => {
    const { wrapper } = await mountAt('/network/modules')
    expect(wrapper.get('[data-test="open-board"]').attributes('href')).toBe('/dashboard?board=network')
  })
})
