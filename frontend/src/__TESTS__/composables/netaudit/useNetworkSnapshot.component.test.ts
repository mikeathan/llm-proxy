import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { defineComponent, h } from 'vue'
import { useNetworkSnapshot } from '../../../composables/netaudit/useNetworkSnapshot'
import { NetauditService } from '../../../services/netaudit/netauditService'

vi.mock('../../../services/netaudit/netauditService', () => ({ NetauditService: { overview: vi.fn() } }))
vi.mock('../../../composables/automation/useDispatcher', async () => {
  const { ref } = await import('vue')
  const automations = ref([])
  return { useDispatcher: () => ({ automations, fetchAutomations: vi.fn().mockResolvedValue(undefined) }) }
})

const overview = vi.mocked(NetauditService.overview)
const Card = defineComponent({ setup() { useNetworkSnapshot(); return () => h('div') } })

beforeEach(() => {
  vi.clearAllMocks()
  overview.mockResolvedValue({ posture: 'quiet' } as never)
})

describe('useNetworkSnapshot', () => {
  it('reads once for several cards, and reads again for the next board after all were removed', async () => {
    const a = mount(Card)
    const b = mount(Card)
    await Promise.resolve()
    expect(overview).toHaveBeenCalledTimes(1)
    a.unmount()
    b.unmount()
    const c = mount(Card)
    await Promise.resolve()
    expect(overview).toHaveBeenCalledTimes(2)
    c.unmount()
  })
})
