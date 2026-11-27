import { describe, it, expect, vi, beforeEach } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createMemoryHistory } from 'vue-router'
import { createAppRouter } from '../../../router'
import { PRODUCT_NAME } from '../../../config/brand'

const fetchMetrics = vi.fn()
const fetchFootprint = vi.fn()
vi.mock('../../../services/monitoring/metricsService', () => ({
  MetricsApiService: { fetchMetrics: () => fetchMetrics(), fetchFootprint: () => fetchFootprint(), fetchLogLevel: vi.fn().mockResolvedValue('INFO'), updateLogLevel: vi.fn() },
}))
const fetchState = vi.fn()
vi.mock('../../../services/admin/adminService', () => ({ AdminApiService: { fetchState: () => fetchState() } }))

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

const FOOTPRINT = {
  processes: [
    { role: 'service', pid: 10, cpu_percent: null, rss_mb: 512, threads: 21, open_files: 34, uptime_seconds: 3725 },
    { role: 'model', pid: 20, cpu_percent: 12.5, rss_mb: 20480, threads: 40, uptime_seconds: 90, gpu_vram_mb: 18432 },
  ],
  go: { heap_mb: 48.2, total_mb: 90.7, goroutines: 120, gc_cycles: 33 },
  timestamp: '2026-11-27T10:00:00Z',
}

// The composable keeps module-level state, so each test gets a fresh module.
async function mountStrip(sample: unknown = SAMPLE, state: unknown = { models: [], config: {}, next_port: 9000 }) {
  vi.resetModules()
  fetchMetrics.mockReset()
  fetchState.mockReset()
  fetchState.mockResolvedValue(state)
  fetchFootprint.mockReset()
  fetchFootprint.mockResolvedValue(FOOTPRINT)
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
    for (const name of ['CPU load', 'Memory used', 'GPU load', 'VRAM used']) expect(panel.find(`[role="meter"][aria-label="${name}"]`).exists()).toBe(true)
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

  describe('running model', () => {
    const MIN = 60_000
    const active = (over: Record<string, unknown> = {}) => ({
      models: [],
      config: {},
      next_port: 9000,
      active: { name: 'Qwen3.6 35B A3B', provider: 'local', endpoint: 'http://127.0.0.1:8081', port: 8081, ready: true, started_at: new Date(Date.now() - 43 * MIN).toISOString(), last_used_at: new Date(Date.now() - 2 * MIN).toISOString(), ...over },
    })
    const section = (w: VueWrapper) => panelOf(w).get('[data-test="running-model"]')

    it('reads which model is loaded only when the panel opens', async () => {
      const w = await mountStrip(SAMPLE, active())
      expect(fetchState).not.toHaveBeenCalled()
      await stripOf(w).trigger('click')
      await flushPromises()
      expect(fetchState).toHaveBeenCalledTimes(1)
      w.unmount()
    })

    it('names the running model, its port, how long it has run and its last call', async () => {
      const w = await mountStrip(SAMPLE, active())
      await stripOf(w).trigger('click')
      await flushPromises()
      const text = section(w).text()
      expect(text).toContain('Running model')
      expect(text).toContain('Qwen3.6 35B A3B')
      expect(text).toContain(':8081')
      expect(text).toContain('ready')
      expect(text).toContain('43m')
      expect(text).toContain('2 minutes ago')
      w.unmount()
    })

    it('says a model is still loading', async () => {
      const w = await mountStrip(SAMPLE, active({ ready: false }))
      await stripOf(w).trigger('click')
      await flushPromises()
      expect(section(w).text()).toContain('loading')
      w.unmount()
    })

    it('says when no local model is running', async () => {
      const w = await mountStrip()
      await stripOf(w).trigger('click')
      await flushPromises()
      expect(section(w).text()).toContain('No local model running')
      w.unmount()
    })
  })

  describe('this service (footprint)', () => {
    const section = (w: VueWrapper) => panelOf(w).get('[data-test="service-footprint"]')

    it('reads the footprint only while the panel is open', async () => {
      const w = await mountStrip()
      expect(fetchFootprint).not.toHaveBeenCalled()
      await stripOf(w).trigger('click')
      await flushPromises()
      expect(fetchFootprint).toHaveBeenCalledTimes(1)
      w.unmount()
    })

    it('shows the service and the model server with CPU, memory, GPU memory and the Go runtime', async () => {
      const w = await mountStrip()
      await stripOf(w).trigger('click')
      await flushPromises()
      const text = section(w).text()
      expect(text).toContain('This service')
      expect(text).toContain(`${PRODUCT_NAME} service`)
      expect(text).toContain('Measuring…')
      expect(text).toContain('512 MB')
      expect(text).toContain('12.5%')
      expect(text).toContain('18.0 GB')
      expect(text).toContain('Not reported')
      expect(section(w).get('[data-test="go-runtime"]').text()).toContain('120 goroutines')
      w.unmount()
    })

    it('says so when the footprint cannot be read', async () => {
      const w = await mountStrip()
      fetchFootprint.mockRejectedValue(new Error('down'))
      await stripOf(w).trigger('click')
      await flushPromises()
      expect(section(w).text()).toContain('Could not read the service figures')
      w.unmount()
    })
  })

  it('links from the panel to the Overview for the full picture', async () => {
    const w = await mountStrip()
    await stripOf(w).trigger('click')
    expect(panelOf(w).get('a').attributes('href')).toBe('/dashboard')
    w.unmount()
  })
})
