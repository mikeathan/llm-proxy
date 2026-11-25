import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const service = vi.hoisted(() => ({ get: vi.fn() }))
vi.mock('../../../services/modelStatsService', () => ({ ModelStatsService: service }))

import NewsroomEditionModelUse from '../../../components/newsroom/NewsroomEditionModelUse.vue'
import { resetModelUse } from '../../../composables/newsroom/useModelUse'
import type { ModelStatsReport } from '../../../types/modelStats'

const week = (today: { local: number; cloud: number }): ModelStatsReport['days'] =>
  [90, 120, 80, 150, 170, 200].map((n, i) => ({ date: `2026-11-${18 + i}`, local: n, cloud: Math.round(n / 5) })).concat([{ date: '2026-11-24', ...today }])
const report = (over: Partial<ModelStatsReport> = {}): ModelStatsReport => ({
  days: week({ local: 214, cloud: 38 }),
  today: [
    { model: 'Qwen3.5 35B A3B', local: true, calls: 128, failed: 2, prompt_tokens: 400000, completion_tokens: 12000, tokens_per_sec: 41.2 },
    { model: 'deepseek-v4.1-flash', local: false, calls: 22, failed: 0, prompt_tokens: 100000, completion_tokens: 40000 },
  ],
  ...over,
})
const mounted: { unmount(): void }[] = []
afterEach(() => {
  mounted.splice(0).forEach((w) => w.unmount())
  resetModelUse()
})
const mountIt = async () => {
  const w = mount(NewsroomEditionModelUse)
  mounted.push(w)
  await flushPromises()
  return w
}
beforeEach(() => { vi.clearAllMocks(); service.get.mockResolvedValue(report()) })

describe('NewsroomEditionModelUse', () => {
  it('asks for a week, and is folded to one line: the day\'s calls and a small trend', async () => {
    const w = await mountIt()
    expect(service.get).toHaveBeenCalledWith(7)
    expect(w.get('[data-test="model-use-total"]').text()).toBe('150 calls today')
    expect(w.findAll('[data-test="spark-bar"]')).toHaveLength(7)
    expect(w.get('[data-test="model-use-toggle"]').attributes('aria-expanded')).toBe('false')
    expect(w.find('[data-test="model-use-body"]').exists()).toBe(false)
  })

  it('opens into a stacked week and a table of today by model, with tokens, a local speed and failures', async () => {
    const w = await mountIt()
    await w.get('[data-test="model-use-toggle"]').trigger('click')
    expect(w.findAll('[data-test="model-use-column"]')).toHaveLength(7)
    expect(w.get('[data-test="model-use-chart"]').attributes('aria-label')).toContain('local')
    expect(w.get('[data-test="model-use-legend"]').text()).toContain('Local')
    expect(w.get('[data-test="model-use-legend"]').text()).toContain('Cloud')
    const rows = w.findAll('[data-test="model-use-row"]')
    expect(rows).toHaveLength(2)
    expect(rows[0]!.text()).toContain('Qwen3.5 35B A3B')
    expect(rows[0]!.text()).toContain('128')
    expect(rows[0]!.text()).toContain('412.0K')
    expect(rows[0]!.text()).toContain('41 tok/s')
    expect(rows[0]!.text()).toContain('2 failed')
    expect(rows[1]!.text()).toContain('—')
    expect(rows[1]!.text()).not.toContain('failed')
    expect(rows[1]!.attributes('data-kind')).toBe('cloud')
  })

  it('draws today by the hour the calls ended, and draws nothing for a server that does not count hours or a day with no calls', async () => {
    const hours = Array.from({ length: 24 }, (_, i) => ({ local: i === 9 ? 6 : 0, cloud: i === 14 ? 3 : 0 }))
    service.get.mockResolvedValue(report({ hours }))
    const w = await mountIt()
    await w.get('[data-test="model-use-toggle"]').trigger('click')
    const cols = w.findAll('[data-test="model-use-hour"]')
    expect(cols).toHaveLength(24)
    expect(cols[9]!.attributes('title')).toBe('09:00: 6 local, 0 cloud')
    expect(w.get('[data-test="model-use-hours"]').text()).toContain('12')
    w.unmount()
    service.get.mockResolvedValue(report())
    const old = await mountIt()
    await old.get('[data-test="model-use-toggle"]').trigger('click')
    expect(old.find('[data-test="model-use-hours"]').exists()).toBe(false)
    old.unmount()
    service.get.mockResolvedValue(report({ hours: Array.from({ length: 24 }, () => ({ local: 0, cloud: 0 })) }))
    const quiet = await mountIt()
    await quiet.get('[data-test="model-use-toggle"]').trigger('click')
    expect(quiet.find('[data-test="model-use-hours"]').exists()).toBe(false)
  })

  it('says one call in the singular, and says so when nothing was called today', async () => {
    service.get.mockResolvedValue(report({ today: [{ model: 'm', local: true, calls: 1, failed: 0, prompt_tokens: 1, completion_tokens: 1 }] }))
    expect((await mountIt()).get('[data-test="model-use-total"]').text()).toBe('1 call today')
    // The numbers are shared by every block on screen; a fresh page asks again.
    mounted.splice(0).forEach((m) => m.unmount())
    resetModelUse()
    service.get.mockResolvedValue(report({ today: [] }))
    const w = await mountIt()
    expect(w.get('[data-test="model-use-total"]').text()).toBe('No calls today')
    await w.get('[data-test="model-use-toggle"]').trigger('click')
    expect(w.get('[data-test="model-use-none"]').text()).toBe('No model calls today yet.')
  })

  it('says tokens were not reported, rather than showing zero, for a model whose provider (or an outside caller) gave none', async () => {
    service.get.mockResolvedValue(report({ today: [{ model: 'flash', local: false, calls: 5, failed: 0, prompt_tokens: 0, completion_tokens: 0 }] }))
    const w = await mountIt()
    await w.get('[data-test="model-use-toggle"]').trigger('click')
    expect(w.get('[data-test="model-use-row"]').text()).toContain('tokens not reported')
    expect(w.get('[data-test="model-use-row"]').text()).not.toContain('0 tokens')
  })

  it('shows model names as text', async () => {
    service.get.mockResolvedValue(report({ today: [{ model: '<img src=x onerror=alert(1)>', local: true, calls: 1, failed: 0, prompt_tokens: 0, completion_tokens: 0 }] }))
    const w = await mountIt()
    await w.get('[data-test="model-use-toggle"]').trigger('click')
    expect(w.find('img').exists()).toBe(false)
    expect(w.text()).toContain('<img src=x onerror=alert(1)>')
  })

  it('says it could not be read, and offers no fold, when the server cannot answer', async () => {
    service.get.mockRejectedValue(new Error('down'))
    const w = await mountIt()
    expect(w.get('[data-test="model-use-unavailable"]').text()).toBe('The model use could not be read.')
    expect(w.find('[data-test="model-use-toggle"]').exists()).toBe(false)
  })
})
