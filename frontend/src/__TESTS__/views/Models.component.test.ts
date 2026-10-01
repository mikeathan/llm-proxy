import { describe, it, expect, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { ref } from 'vue'
import { createMemoryHistory } from 'vue-router'
import { createAppRouter } from '../../router'
import type { AdminState } from '../../types/admin'
import type { Model } from '../../types/model'

const models = {
  state: ref<AdminState | null>(null),
  error: ref<string | null>(null),
  refresh: vi.fn(),
  startModel: vi.fn(),
  stopModel: vi.fn(),
}
vi.mock('../../composables/models/useModels', () => ({ useModels: () => models }))

import Models from '../../views/ModelsView.vue'

const model = (m: Partial<Model>): Model => ({ name: 'm', provider: 'local', endpoint: '', active: false, ready: false, ...m })
const LOCAL_IDLE = model({ name: 'qwen', filename: 'qwen.gguf' })
const LOCAL_ACTIVE = model({ name: 'llama', filename: 'llama.gguf', active: true, ready: true })
const CLOUD = model({ name: 'gpt', provider: 'openai', model_id: 'gpt-5' })

function stateWith(list: Model[], config: Record<string, unknown> = {}): AdminState {
  return { models: list, config } as unknown as AdminState
}

async function mountModels() {
  const router = createAppRouter(createMemoryHistory())
  await router.push('/models')
  const w = mount(Models, { global: { plugins: [router], stubs: { Icon: true, BrandMark: true } } })
  await flushPromises()
  return w
}

// Characterised against Dashboard.vue first (plan D22), then updated to the
// redesigned page: the same controls and targets, laid out as two tables.
describe('Models', () => {
  beforeEach(() => {
    models.state.value = stateWith([LOCAL_IDLE, LOCAL_ACTIVE, CLOUD], { primary_model: 'gpt' })
    models.error.value = null
    Object.values(models).forEach((v) => typeof v === 'function' && v.mockReset())
  })

  const button = (w: Awaited<ReturnType<typeof mountModels>>, label: string) =>
    w.findAll('button').find((b) => b.attributes('aria-label') === label || b.text() === label)!

  it('refreshes the catalogue on mount', async () => {
    await mountModels()
    expect(models.refresh).toHaveBeenCalledTimes(1)
  })

  it('lists local and cloud models in separate tables', async () => {
    const w = await mountModels()
    const captions = w.findAll('caption').map((c) => c.text())
    expect(captions).toEqual(['Local models', 'Cloud models'])
    const [local, cloud] = w.findAll('table')
    expect(local!.text()).toContain('qwen')
    expect(local!.text()).toContain('Active')
    expect(cloud!.text()).toContain('gpt-5')
    expect(cloud!.text()).toContain('Primary')
  })

  it('stops the active local model, and blocks starting another while one is active', async () => {
    const w = await mountModels()
    await button(w, 'Stop llama').trigger('click')
    expect(models.stopModel).toHaveBeenCalledTimes(1)
    expect(button(w, 'Start qwen').attributes('disabled')).toBeDefined()
  })

  it('starts a local model when none is active', async () => {
    models.state.value = stateWith([LOCAL_IDLE, CLOUD])
    const w = await mountModels()
    await button(w, 'Start qwen').trigger('click')
    expect(models.startModel).toHaveBeenCalledWith('qwen')
  })

  it('links each model to its settings section', async () => {
    const w = await mountModels()
    const hrefs = w.findAll('a').map((a) => a.attributes('href'))
    expect(hrefs).toContain('/settings/local-models')
    expect(hrefs).toContain('/settings/openai')
  })

  it('offers adding a model or a provider when there are none', async () => {
    models.state.value = stateWith([])
    const w = await mountModels()
    expect(w.text()).toContain('No models configured')
    expect(w.find('a[href="/settings/local-models"]').exists()).toBe(true)
    expect(w.find('a[href="/settings/openai"]').exists()).toBe(true)
  })

  it('shows loading, then an error with a retry when the catalogue cannot be read', async () => {
    models.state.value = null
    const loading = await mountModels()
    expect(loading.text()).toContain('Loading models')

    models.error.value = 'permission denied'
    const failed = await mountModels()
    expect(failed.get('[role="alert"]').text()).toContain('permission denied')
    await button(failed, 'Retry').trigger('click')
    expect(models.refresh).toHaveBeenCalledTimes(3)
  })
})
