import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory } from 'vue-router'
import { createAppRouter } from '../../router'

const service = vi.hoisted(() => ({ list: vi.fn() }))
vi.mock('../../services/stacks/stacksService', () => ({ StacksService: service }))

import StacksView from '../../views/StacksView.vue'

const summary = (id: string, title: string, on: number) => ({ id, title, purpose: `${title} purpose`, on, total: 4, stopped: 0, blocked: 0 })

async function show() {
  const router = createAppRouter(createMemoryHistory())
  await router.push('/stacks')
  const w = mount(StacksView, { global: { plugins: [router] } })
  await flushPromises()
  return { w, router }
}

describe('StacksView', () => {
  beforeEach(() => vi.clearAllMocks())

  it('lists every stack with whether anything of it is on, and links to its modules', async () => {
    service.list.mockResolvedValue([summary('network', 'Network', 0), summary('agents', 'Agents', 2), summary('radio', 'Radio', 1)])
    const { w } = await show()
    expect(w.get('[data-test="stack-network"]').text()).toContain('Off: none of its 4 modules is on')
    expect(w.get('[data-test="stack-agents"]').text()).toContain('On: 2 of 4 modules')
    const links = w.findAll('[data-test^="stack-"] a').map((a) => a.attributes('href'))
    expect(links).toEqual(['/network/modules', '/agents/modules', '/radio/modules'])
  })

  it('says when the stacks could not be read', async () => {
    service.list.mockRejectedValue(new Error('down'))
    const { w } = await show()
    expect(w.get('[role="alert"]').text()).not.toBe('')
  })
})
