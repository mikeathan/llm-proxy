import { describe, expect, it } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory } from 'vue-router'
import { createAppRouter } from '../../router'
import AgentsView from '../../views/AgentsView.vue'

const STUBS = {
  StackModules: { props: ['stack'], template: '<div data-test="stub-modules" />' },
  AgentLockdown: { template: '<div data-test="stub-lockdown" />' },
  AgentTools: { template: '<div data-test="stub-tools" />' },
  StackBoardLink: { props: ['stack'], template: '<div data-test="stub-board-link" :data-stack="stack" />' },
}

async function mountAt(path: string) {
  const router = createAppRouter(createMemoryHistory())
  await router.push(path)
  const wrapper = mount(AgentsView, { global: { plugins: [router], stubs: STUBS } })
  await flushPromises()
  return wrapper
}

describe('AgentsView', () => {
  it('has Lock down, Tools and Modules, and links to its board', async () => {
    const w = await mountAt('/agents')
    expect(w.findAll('a.stack-tab').map((a) => a.text())).toEqual(['Lock down', 'Tools', 'Modules'])
    expect(w.get('[data-test="stub-board-link"]').attributes('data-stack')).toBe('agents')
    expect(w.find('[data-test="stub-lockdown"]').exists()).toBe(true)
  })

  it('shows the MCP servers on the Tools tab and neither the switch nor the modules', async () => {
    const w = await mountAt('/agents/tools')
    expect(w.find('[data-test="stub-tools"]').exists()).toBe(true)
    expect(w.find('[data-test="stub-lockdown"]').exists()).toBe(false)
    expect(w.find('[data-test="stub-modules"]').exists()).toBe(false)
  })

  it('sends an old Settings link to the Tools tab', async () => {
    const router = createAppRouter(createMemoryHistory())
    await router.push('/settings/mcp')
    expect(router.currentRoute.value.fullPath).toBe('/agents/tools')
  })

  it('shows Modules on its tab and not the lock down switch', async () => {
    const w = await mountAt('/agents/modules')
    expect(w.find('[data-test="stub-modules"]').exists()).toBe(true)
    expect(w.find('[data-test="stub-lockdown"]').exists()).toBe(false)
  })
})
