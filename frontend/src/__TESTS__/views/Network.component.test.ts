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
vi.mock('../../composables/netaudit/useNetworkOverview', () => ({
  useNetworkOverview: () => ({ overview: ref({ posture: 'quiet' }), error: ref(''), load: vi.fn(), start: vi.fn(), stop: vi.fn() }),
}))

import NetworkView from '../../views/NetworkView.vue'

const STUBS = {
  NetworkOverviewPanel: { emits: ['open-tab'], template: '<div data-test="stub-overview" @click="$emit(\'open-tab\', \'devices\')" />' },
  DevicesCard: { template: '<div data-test="stub-devices" />' },
  FindingsReport: { template: '<div data-test="stub-findings" />' },
  TimelineTab: { props: ['audit'], template: '<div data-test="stub-timeline" />' },
  AuditsPanel: { template: '<div data-test="stub-audits" />' },
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

  it('has the tabs Overview, Devices, Findings, Timeline, Audits, Modules and Settings', async () => {
    const { wrapper } = await mountAt('/network')
    expect(tabLabels(wrapper)).toEqual(['Overview', 'Devices', 'Findings', 'Timeline', 'Audits', 'Modules', 'Settings'])
    expect(wrapper.findAll('a.stack-tab').map((a) => a.attributes('href'))).toEqual(['/network', '/network/devices', '/network/findings', '/network/timeline', '/network/audits', '/network/modules', '/network/settings'])
  })

  it.each([
    ['/network', 'stub-overview'],
    ['/network/devices', 'stub-devices'],
    ['/network/timeline', 'stub-timeline'],
    ['/network/audits', 'stub-audits'],
    ['/network/modules', 'stub-modules'],
    ['/network/settings', 'stub-settings'],
  ])('shows one thing on %s', async (path, shown) => {
    const { wrapper } = await mountAt(path)
    const all = ['stub-overview', 'stub-devices', 'stub-findings', 'stub-timeline', 'stub-audits', 'stub-modules', 'stub-settings']
    expect(all.filter((id) => wrapper.find(`[data-test="${id}"]`).exists())).toEqual([shown])
  })

  it('keeps Simple/Advanced and Help in the page header, once, on every tab', async () => {
    for (const path of ['/network', '/network/devices', '/network/findings', '/network/timeline', '/network/audits', '/network/modules', '/network/settings']) {
      const { wrapper } = await mountAt(path)
      expect(wrapper.get('header').findAll('[role="radio"], [role="tab"], button').some((b) => b.attributes('aria-label') === 'Help')).toBe(true)
      expect(wrapper.findAll('button').filter((b) => b.attributes('aria-label') === 'Help')).toHaveLength(1)
    }
  })

  it('tells the readiness banner it is on the settings tab, so it does not link to itself', async () => {
    expect((await mountAt('/network/settings')).wrapper.get('[data-test="stub-readiness"]').attributes('data-here')).toBe('network-scan')
    expect((await mountAt('/network/devices')).wrapper.get('[data-test="stub-readiness"]').attributes('data-here')).toBeUndefined()
  })

  it('sends findings without an audit to the Audits tab, not to Settings', async () => {
    const { wrapper, router } = await mountAt('/network/findings')
    await wrapper.findAll('button').find((b) => b.text() === 'Set up a network audit')!.trigger('click')
    await vi.waitFor(() => expect(router.currentRoute.value.path).toBe('/network/audits'))
  })

  it('opens the tab the verdict card points to', async () => {
    const { wrapper, router } = await mountAt('/network')
    await wrapper.get('[data-test="stub-overview"]').trigger('click')
    await vi.waitFor(() => expect(router.currentRoute.value.path).toBe('/network/devices'))
  })
})
