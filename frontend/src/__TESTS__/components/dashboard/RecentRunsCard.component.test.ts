import { beforeEach, describe, expect, it, vi } from 'vitest'
import { RouterLinkStub, flushPromises, mount } from '@vue/test-utils'

const dispatcher = vi.hoisted(() => ({ fetchGlobalActivity: vi.fn() }))
vi.mock('../../../composables/automation/useDispatcher', () => ({ useDispatcher: () => dispatcher }))

import RecentRunsCard from '../../../components/dashboard/cards/RecentRunsCard.vue'

const run = (i: number) => ({ id: `r${i}`, automation_name: `auto-${i}`, workspace_id: 'ws', timestamp: new Date(Date.now() - i * 60_000).toISOString(), duration_ms: 1000, status: 'completed' })

beforeEach(() => vi.clearAllMocks())

describe('RecentRunsCard', () => {
  it('lists every kept run in a table that scrolls at the large size, so the card never hides the rest', async () => {
    dispatcher.fetchGlobalActivity.mockResolvedValue(Array.from({ length: 12 }, (_, i) => run(i)))
    const w = mount(RecentRunsCard, { global: { stubs: { RouterLink: RouterLinkStub } } })
    await flushPromises()
    expect(w.findAll('tbody tr')).toHaveLength(12)
    const box = w.get('[data-test="recent-runs-table"]')
    expect(box.classes()).toEqual(expect.arrayContaining(['overflow-y-auto']))
    expect(box.classes().some((c) => c.startsWith('max-h-'))).toBe(true)
  })
})
