import { describe, it, expect, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { ref } from 'vue'
import { createMemoryHistory, createRouter } from 'vue-router'
import type { ModelStatsReport } from '../../../../types/modelStats'

const MIN = 60_000
const activeModel = ref<unknown>(null)
vi.mock('../../../../composables/models/useModels', () => ({ useModels: () => ({ activeModel }) }))
const metrics = ref<unknown>({ load_percent: 3, mem_used_mb: 8192, mem_total_mb: 49152, llm_tokens_per_sec: 34.5 })
vi.mock('../../../../composables/system/useMetrics', () => ({ useMetrics: () => ({ metrics, history: ref([]) }) }))
const report = ref<ModelStatsReport | null>(null)
vi.mock('../../../../composables/newsroom/useModelUse', () => ({ useModelUse: () => ({ report, failed: ref(false) }) }))
const size = ref('lg')
vi.mock('../../../../composables/dashboard/useCardSize', () => ({ useCardSize: () => size }))

import HostHealthCard from '../../../../components/dashboard/cards/HostHealthCard.vue'

async function mountCard() {
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/:p(.*)*', name: 'models', component: { render: () => null } }] })
  const w = mount(HostHealthCard, { global: { plugins: [router], stubs: { Icon: true } } })
  await flushPromises()
  return w
}

describe('HostHealthCard', () => {
  beforeEach(() => {
    size.value = 'lg'
    activeModel.value = { name: 'Qwen3.6 35B A3B', ready: true, port: 8081, started_at: new Date(Date.now() - 43 * MIN).toISOString(), last_used_at: new Date(Date.now() - 2 * MIN).toISOString() }
    report.value = {
      days: [],
      today: [
        { model: 'Qwen3.6 35B A3B', local: true, calls: 398, failed: 0, prompt_tokens: 1, completion_tokens: 1, tokens_per_sec: 37.3 },
        { model: 'deepseek-v4.1-flash', local: false, calls: 1, failed: 1, prompt_tokens: 12000, completion_tokens: 800 },
      ],
    }
  })

  it('names the running model, how long it has run and its last call', async () => {
    const w = await mountCard()
    expect(w.get('[data-test="host-model"]').text()).toContain('Qwen3.6 35B A3B')
    expect(w.get('[data-test="host-model-running"]').text()).toBe('up 43m 0s · last call 2 minutes ago')
  })

  it('lists today by model: calls, failures, a local speed and a cloud model\'s tokens', async () => {
    const rows = (await mountCard()).get('[data-test="host-today"]').findAll('li').map((li) => li.text().replace(/\s+/g, ' '))
    expect(rows[0]).toContain('Qwen3.6 35B A3B')
    expect(rows[0]).toContain('398 calls')
    expect(rows[0]).toContain('37 tok/s')
    expect(rows[1]).toContain('1 call · 1 failed')
    expect(rows[1]).toContain('12.8K tokens')
  })

  it('says when no model was called today, and leaves the list out at the small sizes', async () => {
    report.value = { days: [], today: [] }
    expect((await mountCard()).get('[data-test="host-today"]').text()).toContain('No model calls today yet.')
    size.value = 'sm'
    expect((await mountCard()).find('[data-test="host-today"]').exists()).toBe(false)
  })
})
