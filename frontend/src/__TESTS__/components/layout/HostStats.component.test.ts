import { describe, it, expect, vi, beforeEach } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createMemoryHistory } from 'vue-router'
import { createAppRouter } from '../../../router'

const fetchMetrics = vi.fn()
vi.mock('../../../services/monitoring/metricsService', () => ({
  MetricsApiService: { fetchMetrics: () => fetchMetrics(), fetchLogLevel: vi.fn().mockResolvedValue('INFO'), updateLogLevel: vi.fn() },
}))

const GPU = {
  name: 'Apple GPU',
  vendor: 'apple',
  memory_used_mb: 16281.6,
  memory_total_mb: 16384,
  memory_utilization_percent: 99.4,
  utilization_percent: 4.9,
  temperature_c: 37,
}
const SAMPLE = { load_percent: 0.2, mem_used_mb: 7475.2, mem_total_mb: 47923.2, llm_tokens_per_sec: 33.2, gpu: GPU }

// The composable keeps module-level state, so each test gets a fresh module.
async function mountStrip(sample: unknown = SAMPLE) {
  vi.resetModules()
  fetchMetrics.mockReset()
  if (sample instanceof Error) fetchMetrics.mockRejectedValue(sample)
  else fetchMetrics.mockResolvedValue(sample)
  const { default: HostStats } = await import('../../../components/layout/HostStats.vue')
  const router = createAppRouter(createMemoryHistory())
  await router.push('/workspaces/ws/assistant')
  const w = mount(HostStats, { global: { plugins: [router], stubs: { Icon: true } }, attachTo: document.body })
  await flushPromises()
  return w
}

const stripOf = (w: VueWrapper) => w.find('button[aria-controls="host-stats-panel"]')
const figure = (w: VueWrapper, name: string) => w.get(`[data-figure="${name}"]`)
const panelOf = (w: VueWrapper) => w.find('#host-stats-panel')

describe('HostStats (header strip)', () => {
  beforeEach(() => vi.spyOn(console, 'error').mockImplementation(() => {}))

  it('renders nothing until the first sample arrives, and stays empty if it never does', async () => {
    const w = await mountStrip(new Error('down'))
    expect(stripOf(w).exists()).toBe(false)
    expect(w.text()).toBe('')
    w.unmount()
  })

  it('shows CPU, memory, GPU and throughput at a glance', async () => {
    const w = await mountStrip()
    expect(figure(w, 'cpu').text()).toContain('0.2%')
    expect(figure(w, 'mem').text()).toContain('16%')
    expect(figure(w, 'gpu').text()).toContain('4.9%')
    expect(figure(w, 'tps').text()).toContain('33.2')
    expect(figure(w, 'tps').text()).toContain('t/s')
    w.unmount()
  })

  it('leaves the GPU figure out when there is no GPU', async () => {
    const { gpu: _gpu, ...noGpu } = SAMPLE
    const w = await mountStrip(noGpu)
    expect(w.find('[data-figure="gpu"]').exists()).toBe(false)
    expect(w.find('[data-figure="cpu"]').exists()).toBe(true)
    w.unmount()
  })

  it.each([
    [10, 'ok'],
    [60, 'busy'],
    [85, 'high'],
  ])('marks a %d%% CPU load as %s', async (load, level) => {
    const w = await mountStrip({ ...SAMPLE, load_percent: load })
    expect(figure(w, 'cpu').attributes('data-level')).toBe(level)
    w.unmount()
  })

  it('is framed as a raised plate with a hairline border, darkening on hover', async () => {
    const w = await mountStrip()
    const classes = stripOf(w).classes()
    expect(classes).toEqual(expect.arrayContaining(['bg-surface-raised', 'border-hairline', 'hover:border-strong', 'hover:bg-surface-hover']))
    expect(classes).not.toContain('border-transparent')
    w.unmount()
  })

  it('does not flag an ordinary 72% memory use, but does flag 92%', async () => {
    const used = (percent: number) => ({ ...SAMPLE, mem_used_mb: percent * 10, mem_total_mb: 1000 })
    const calm = await mountStrip(used(72))
    expect(figure(calm, 'mem').attributes('data-level')).toBe('ok')
    calm.unmount()
    const hot = await mountStrip(used(92))
    expect(figure(hot, 'mem').attributes('data-level')).toBe('high')
    hot.unmount()
  })

  it('opens a labelled details panel on click and closes it on Escape', async () => {
    const w = await mountStrip()
    expect(stripOf(w).attributes('aria-expanded')).toBe('false')
    expect(panelOf(w).exists()).toBe(false)

    await stripOf(w).trigger('click')
    expect(stripOf(w).attributes('aria-expanded')).toBe('true')
    const panel = panelOf(w)
    expect(panel.attributes('role')).toBe('dialog')
    expect(panel.attributes('aria-label')).toBe('System stats')
    for (const name of ['CPU load', 'Memory used', 'VRAM used']) expect(panel.find(`[role="meter"][aria-label="${name}"]`).exists()).toBe(true)
    expect(panel.get('[role="meter"][aria-label="Memory used"]').attributes('aria-valuetext')).toBe('7.3 / 46.8 GB')
    expect(panel.get('[role="meter"][aria-label="VRAM used"]').attributes('aria-valuetext')).toBe('15.9 / 16.0 GB')
    expect(panel.text()).toContain('Apple GPU')
    expect(panel.text()).toContain('4.9%')
    expect(panel.text()).toContain('37°C')
    expect(panel.text()).toContain('33.2')

    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await flushPromises()
    expect(panelOf(w).exists()).toBe(false)
    expect(stripOf(w).attributes('aria-expanded')).toBe('false')
    w.unmount()
  })

  it('says so when the GPU reports no temperature or there is no GPU at all', async () => {
    const w = await mountStrip({ ...SAMPLE, gpu: { ...GPU, temperature_c: Number.NaN } })
    await stripOf(w).trigger('click')
    expect(panelOf(w).text()).toContain('Not reported')
    w.unmount()

    const { gpu: _gpu, ...noGpu } = SAMPLE
    const w2 = await mountStrip({ ...noGpu, gpu_error: 'rocm-smi not found' })
    await stripOf(w2).trigger('click')
    expect(panelOf(w2).text()).toContain('No GPU detected')
    expect(panelOf(w2).text()).toContain('rocm-smi not found')
    w2.unmount()
  })

  it('links from the panel to the Overview for the full picture', async () => {
    const w = await mountStrip()
    await stripOf(w).trigger('click')
    expect(panelOf(w).get('a').attributes('href')).toBe('/overview')
    w.unmount()
  })
})
