import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { ref } from 'vue'
import { fieldByLabel } from '../../helpers/fieldByLabel'
import type { Model, ModelEditorOpen } from '../../../types/model'

const models = {
  state: ref({ config: {} }),
  addModel: vi.fn(),
  updateModel: vi.fn(),
  removeModel: vi.fn(),
  removeAllModels: vi.fn(),
  fetchProviderModels: vi.fn().mockResolvedValue([]),
}
vi.mock('../../../composables/models/useModels', () => ({ useModels: () => models }))
const confirm = vi.fn()
vi.mock('../../../composables/ui/useConfirm', () => ({ useConfirm: () => ({ confirm }) }))

import ModelEditorPanel from '../../../components/models/ModelEditorPanel.vue'

const QWEN = { name: 'qwen', provider: 'local', filename: 'qwen.gguf', port: 8081, endpoint: '', active: false, ready: false, workload_class: 'local' } as Model

function mountPanel(open: ModelEditorOpen | null) {
  const onOpen = vi.fn()
  const w = mount(ModelEditorPanel, {
    props: { open, models: [QWEN], availableModels: [], providers: [{ id: 'openai', label: 'OpenAI / Compatible' }], apiKeysOf: () => [] },
    attrs: { 'onUpdate:open': onOpen },
    global: { stubs: { Icon: true, teleport: true } },
    attachTo: document.body,
  })
  return { w, onOpen }
}
const pressEscape = (w: ReturnType<typeof mountPanel>['w']) => w.get('[role="dialog"]').trigger('keydown', { key: 'Escape' })

describe('ModelEditorPanel', () => {
  beforeEach(() => vi.clearAllMocks())
  afterEach(() => {
    document.body.innerHTML = ''
  })

  it('closes at once when nothing was changed', async () => {
    const { w, onOpen } = mountPanel({ kind: 'edit', model: QWEN })
    await flushPromises()
    await pressEscape(w)
    await flushPromises()
    expect(confirm).not.toHaveBeenCalled()
    expect(onOpen).toHaveBeenCalledWith(null)
  })

  it('asks before throwing away an edit, and stays open when told to keep editing', async () => {
    const { w, onOpen } = mountPanel({ kind: 'edit', model: QWEN })
    await flushPromises()
    await fieldByLabel(w, /^name$/i).setValue('qwen-2')
    confirm.mockResolvedValueOnce(false)
    await pressEscape(w)
    await flushPromises()
    expect(confirm).toHaveBeenCalledTimes(1)
    expect(onOpen).not.toHaveBeenCalled()
    confirm.mockResolvedValueOnce(true)
    await pressEscape(w)
    await flushPromises()
    expect(onOpen).toHaveBeenCalledWith(null)
  })

  it('names what it edits, and asks where a new model runs', async () => {
    expect(mountPanel({ kind: 'edit', model: QWEN }).w.get('[role="dialog"]').text()).toContain('Edit qwen')
    const add = mountPanel({ kind: 'add', provider: 'local' })
    await flushPromises()
    const where = fieldByLabel(add.w, /where it runs/i)
    await where.setValue('openai')
    expect(add.onOpen).toHaveBeenCalledWith({ kind: 'add', provider: 'openai' })
  })
})
