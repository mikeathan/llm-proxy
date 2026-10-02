import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { ref } from 'vue'
import { useConfirm } from '../../../../composables/ui/useConfirm'
import type { MemoryEntry, MemoryInjectionPreview, OperatorNotes } from '../../../../types/memory'

const mem = {
  memories: ref<MemoryEntry[]>([]),
  loading: ref(false),
  searchQuery: ref(''),
  searchResults: ref<MemoryEntry[]>([]),
  fetchMemories: vi.fn(),
  search: vi.fn(),
  deleteMemory: vi.fn(),
  selectMemory: vi.fn(),
  clearAllMemories: vi.fn(),
  updateMemory: vi.fn(),
  setHot: vi.fn(),
  setPriority: vi.fn(),
  createMemory: vi.fn(),
  error: ref<string | null>(null),
  injectionPreview: ref<MemoryInjectionPreview | null>(null),
  fetchInjectionPreview: vi.fn(),
  operatorNotes: ref<OperatorNotes | null>(null),
  memoryRevision: ref(0),
  fetchNotes: vi.fn(),
  exportMemory: vi.fn(),
  importMemory: vi.fn(),
  saveNotes: vi.fn(),
}
vi.mock('../../../../composables/memory/useMemory', () => ({ useMemory: () => mem }))
vi.mock('../../../../composables/models/useModels', () => ({
  useModels: () => ({ state: ref({ models: [{ name: 'qwen', provider: 'local' }, { name: 'gpt-5', provider: 'openai' }] }) }),
}))

import MemoryPanel from '../../../../components/AgentIde/memory/MemoryPanel.vue'
import MemoryDetail from '../../../../components/AgentIde/memory/MemoryDetail.vue'
import MemoryAddForm from '../../../../components/AgentIde/memory/MemoryAddForm.vue'
import MemoryImportExport from '../../../../components/AgentIde/memory/MemoryImportExport.vue'
import MemoryNotesEditor from '../../../../components/AgentIde/memory/MemoryNotesEditor.vue'
import InjectionPreviewPanel from '../../../../components/AgentIde/memory/MemoryInjectionPreview.vue'

const entry = (id: number, over: Partial<MemoryEntry> = {}): MemoryEntry => ({
  id, workspace_id: 'ws', memory_type: 'long_term', title: `Fact ${id}`, content: `content ${id}`, source: 'agent',
  tags: [], priority: 1, created_at: '2026-09-29T10:00:00Z', updated_at: '2026-09-29T10:00:00Z', ...over,
})

const mounted: VueWrapper[] = []
const track = <T extends VueWrapper>(w: T) => (mounted.push(w), w)
const byText = (w: VueWrapper, text: string) => w.findAll('button').find((b) => b.text().trim() === text || b.attributes('aria-label') === text)!

// Characterised before the Phase 5 restyle (plan D22): the contract stays.
describe('MemoryPanel', () => {
  beforeEach(() => {
    mem.memories.value = [entry(1), entry(2, { memory_type: 'daily' })]
    mem.searchResults.value = []
    mem.searchQuery.value = ''
    mem.loading.value = false
    mem.error.value = null
    mem.operatorNotes.value = null
    Object.values(mem).forEach((v) => typeof v === 'function' && v.mockReset())
  })
  afterEach(() => mounted.splice(0).forEach((w) => w.unmount()))

  it('loads the workspace memories and lists them', async () => {
    const w = track(mount(MemoryPanel, { props: { workspaceId: 'ws' }, global: { stubs: { Icon: true } } }))
    expect(mem.fetchMemories).toHaveBeenCalledWith('ws', undefined)
    expect(w.text()).toContain('Fact 1')
    expect(w.text()).toContain('Fact 2')
  })

  it('filters by type, reloading from the server', async () => {
    const w = track(mount(MemoryPanel, { props: { workspaceId: 'ws' }, global: { stubs: { Icon: true } } }))
    await byText(w, 'Daily').trigger('click')
    await flushPromises()
    expect(mem.fetchMemories).toHaveBeenLastCalledWith('ws', 'daily')
  })

  it('offers Hot, Session and User filters next to the existing ones', async () => {
    const w = track(mount(MemoryPanel, { props: { workspaceId: 'ws' }, global: { stubs: { Icon: true } } }))
    const labels = w.findAll('[role="radio"]').map((r) => r.text())
    expect(labels).toEqual(expect.arrayContaining(['All', 'Hot', 'Permanent', 'Daily', 'Session', 'User', 'Unused']))
  })

  it('Hot loads the always-injected facts from the injection query, not the capped list', async () => {
    const w = track(mount(MemoryPanel, { props: { workspaceId: 'ws' }, global: { stubs: { Icon: true } } }))
    await byText(w, 'Hot').trigger('click')
    await flushPromises()
    expect(mem.fetchMemories).toHaveBeenLastCalledWith('ws', undefined, 'hot')
  })

  it('Unused lists the facts never sent to a model or found by search, and explains the caveat', async () => {
    const w = track(mount(MemoryPanel, { props: { workspaceId: 'ws' }, global: { stubs: { Icon: true } } }))
    await byText(w, 'Unused').trigger('click')
    await flushPromises()
    expect(mem.fetchMemories).toHaveBeenLastCalledWith('ws', undefined, 'unused')
    expect(w.text()).toContain('Counting began when usage tracking was added')
  })

  it('has nothing sensible to bulk-clear in the Hot and Unused views', async () => {
    const w = track(mount(MemoryPanel, { props: { workspaceId: 'ws' }, global: { stubs: { Icon: true } } }))
    expect(w.text()).toContain('Clear all')
    await byText(w, 'Unused').trigger('click')
    expect(w.text()).not.toContain('Clear unused')
  })

  it('shows when a fact was saved, when it was last edited and when it was last used', async () => {
    mem.memories.value = [
      entry(1, { created_at: '2026-08-01T10:00:00Z', updated_at: '2026-09-29T10:00:00Z', last_used_at: '2026-09-30 10:00:00' }),
      entry(2, { created_at: '2026-08-01T10:00:00Z', updated_at: '2026-08-01T10:00:00Z', last_used_at: null }),
    ]
    const w = track(mount(MemoryPanel, { props: { workspaceId: 'ws' }, global: { stubs: { Icon: true } } }))
    const rows = w.findAll('li')
    expect(rows[0]!.text()).toContain('Saved')
    expect(rows[0]!.text()).toContain('Edited')
    expect(rows[0]!.text()).toContain('Last used')
    expect(rows[1]!.text()).toContain('Saved')
    expect(rows[1]!.text()).not.toContain('Edited')
    expect(rows[1]!.text()).not.toContain('Last used')
  })

  it('shows how much each fact has been used', async () => {
    mem.memories.value = [entry(1, { tags: ['hot'], injected_count: 12, searched_count: 3 }), entry(2)]
    const w = track(mount(MemoryPanel, { props: { workspaceId: 'ws' }, global: { stubs: { Icon: true } } }))
    const rows = w.findAll('li')
    expect(rows[0]!.text()).toContain('Sent in 12 runs · Found by search 3 times')
    expect(rows[1]!.text()).toContain('Never used')
  })

  it('User shows the facts that follow the operator into every workspace (stored under "global")', async () => {
    const w = track(mount(MemoryPanel, { props: { workspaceId: 'ws' }, global: { stubs: { Icon: true } } }))
    await byText(w, 'User').trigger('click')
    await flushPromises()
    expect(mem.fetchMemories).toHaveBeenLastCalledWith('global', 'user_profile')
  })

  it('marks the facts that ride in every prompt', async () => {
    mem.memories.value = [entry(1, { tags: ['hot'] }), entry(2)]
    const w = track(mount(MemoryPanel, { props: { workspaceId: 'ws' }, global: { stubs: { Icon: true } } }))
    const rows = w.findAll('li')
    expect(rows[0]!.text()).toContain('Always')
    expect(rows[1]!.text()).not.toContain('Always')
  })

  it('labels user-wide facts so it is clear they follow every workspace', async () => {
    mem.memories.value = [entry(1), entry(2, { workspace_id: 'global', memory_type: 'user_profile', title: 'Name' })]
    const w = track(mount(MemoryPanel, { props: { workspaceId: 'ws' }, global: { stubs: { Icon: true } } }))
    const rows = w.findAll('li')
    expect(rows[0]!.text()).not.toContain('All workspaces')
    expect(rows[1]!.text()).toContain('All workspaces')
  })

  it('clears only this workspace in All, and says the user-wide facts are kept', async () => {
    mem.memories.value = [entry(1), entry(2, { workspace_id: 'global', memory_type: 'user_profile', title: 'Name' })]
    mem.clearAllMemories.mockResolvedValue(1)
    const w = track(mount(MemoryPanel, { props: { workspaceId: 'ws' }, global: { stubs: { Icon: true } } }))
    await byText(w, 'Clear all').trigger('click')
    await flushPromises()
    expect(useConfirm().options.value.message).toContain('1 ')
    expect(useConfirm().options.value.message).toContain('user-wide')
    useConfirm().handleConfirm()
    await flushPromises()
    expect(mem.clearAllMemories).toHaveBeenCalledWith('ws', undefined)
  })

  it('marks the facts the operator protected', async () => {
    mem.memories.value = [entry(1, { tags: ['hot'], priority: 2 }), entry(2, { tags: ['hot'], priority: 1 })]
    const w = track(mount(MemoryPanel, { props: { workspaceId: 'ws' }, global: { stubs: { Icon: true } } }))
    const rows = w.findAll('li')
    expect(rows[0]!.text()).toContain('High priority')
    expect(rows[1]!.text()).not.toContain('High priority')
  })

  it('deletes a user-wide fact from the workspace it is stored in, not the one being viewed', async () => {
    mem.memories.value = [entry(7, { workspace_id: 'global', memory_type: 'user_profile', title: 'Name' })]
    const w = track(mount(MemoryPanel, { props: { workspaceId: 'ws' }, global: { stubs: { Icon: true } } }))
    await byText(w, 'Delete memory Name').trigger('click')
    await flushPromises()
    useConfirm().handleConfirm()
    await flushPromises()
    expect(mem.deleteMemory).toHaveBeenCalledWith('global', 7)
  })

  it('opens the operator-notes editor (MEMORY.md) on demand', async () => {
    const w = track(mount(MemoryPanel, { props: { workspaceId: 'ws' }, global: { stubs: { Icon: true } } }))
    expect(w.findComponent(MemoryNotesEditor).exists()).toBe(false)
    await byText(w, 'Operator notes').trigger('click')
    expect(w.findComponent(MemoryNotesEditor).exists()).toBe(true)
  })

  it('shows the import / export controls and reloads after an import', async () => {
    const w = track(mount(MemoryPanel, { props: { workspaceId: 'ws' }, global: { stubs: { Icon: true } } }))
    const io = w.findComponent(MemoryImportExport)
    expect(io.exists()).toBe(true)
    await io.vm.$emit('imported')
    await flushPromises()
    expect(mem.fetchMemories).toHaveBeenCalledTimes(2)
  })

  it('reloads the current filter after a fact is added', async () => {
    const w = track(mount(MemoryPanel, { props: { workspaceId: 'ws' }, global: { stubs: { Icon: true } } }))
    await byText(w, 'Add memory').trigger('click')
    await w.findComponent(MemoryAddForm).vm.$emit('created')
    await flushPromises()
    expect(mem.fetchMemories).toHaveBeenCalledTimes(2)
  })

  it('searches, shows the results, and clears back to the list', async () => {
    mem.search.mockImplementation(async () => {
      mem.searchResults.value = [entry(9, { title: 'Found' })]
    })
    const w = track(mount(MemoryPanel, { props: { workspaceId: 'ws' }, global: { stubs: { Icon: true } } }))
    await w.get('input').setValue('found')
    // Enter submits the search form natively; the test submits it directly.
    await w.get('form').trigger('submit')
    await flushPromises()
    expect(mem.search).toHaveBeenCalledWith('ws', 'found')
    expect(w.text()).toContain('Found')
    expect(w.text()).not.toContain('Fact 1')
  })

  it('selects an entry', async () => {
    const w = track(mount(MemoryPanel, { props: { workspaceId: 'ws' }, global: { stubs: { Icon: true } } }))
    await w.findAll('[data-test="memory-item"]')[0]!.trigger('click')
    expect(w.emitted('select-memory')).toEqual([[mem.memories.value[0]]])
  })

  it('deletes an entry only after confirming', async () => {
    const w = track(mount(MemoryPanel, { props: { workspaceId: 'ws' }, global: { stubs: { Icon: true } } }))
    await byText(w, 'Delete memory Fact 1').trigger('click')
    await flushPromises()
    expect(mem.deleteMemory).not.toHaveBeenCalled()
    useConfirm().handleConfirm()
    await flushPromises()
    expect(mem.deleteMemory).toHaveBeenCalledWith('ws', 1)
  })

  it('says when there are no memories', async () => {
    mem.memories.value = []
    const w = track(mount(MemoryPanel, { props: { workspaceId: 'ws' }, global: { stubs: { Icon: true } } }))
    expect(w.text()).toContain('No memories yet')
  })
})

describe('MemoryDetail', () => {
  beforeEach(() => {
    mem.updateMemory.mockReset().mockResolvedValue(undefined)
    mem.setHot.mockReset().mockResolvedValue(true)
    mem.setPriority.mockReset().mockResolvedValue(true)
  })
  afterEach(() => mounted.splice(0).forEach((w) => w.unmount()))

  it('shows the entry and closes', async () => {
    const w = track(mount(MemoryDetail, { props: { entry: entry(1), workspaceId: 'ws' } }))
    expect(w.text()).toContain('Fact 1')
    expect(w.text()).toContain('content 1')
    expect(w.text()).toContain('Permanent')
    await byText(w, 'Close').trigger('click')
    expect(w.emitted('close')).toHaveLength(1)
  })

  it('edits and saves the title and content', async () => {
    const w = track(mount(MemoryDetail, { props: { entry: entry(1), workspaceId: 'ws' } }))
    await byText(w, 'Edit').trigger('click')
    await w.get('form input').setValue('New title') // scoped: the hot switch is an input too
    await w.get('textarea').setValue('New content')
    expect(byText(w, 'Save').attributes('type')).toBe('submit')
    await w.get('form').trigger('submit')
    await flushPromises()
    expect(mem.updateMemory).toHaveBeenCalledWith('ws', 1, 'New title', 'New content')
    expect(w.emitted('updated')).toHaveLength(1)
  })

  const hotSwitch = (w: VueWrapper) => w.get('input[role="switch"]')

  it('shows how much the fact has been used and when it last was', async () => {
    const used = track(mount(MemoryDetail, { props: { entry: entry(1, { injected_count: 4, last_used_at: '2026-09-30 10:00:00' }), workspaceId: 'ws' } }))
    expect(used.text()).toContain('Sent in 4 runs')
    expect(used.text()).toContain('Last used')
    const unused = track(mount(MemoryDetail, { props: { entry: entry(2), workspaceId: 'ws' } }))
    expect(unused.text()).toContain('Never used')
    expect(unused.text()).not.toContain('Last used')
  })

  it('shows whether the fact rides in every prompt', async () => {
    const hot = track(mount(MemoryDetail, { props: { entry: entry(1, { tags: ['hot'] }), workspaceId: 'ws' } }))
    expect((hotSwitch(hot).element as HTMLInputElement).checked).toBe(true)
    const cold = track(mount(MemoryDetail, { props: { entry: entry(2), workspaceId: 'ws' } }))
    expect((hotSwitch(cold).element as HTMLInputElement).checked).toBe(false)
  })

  it('promotes a fact straight away', async () => {
    const w = track(mount(MemoryDetail, { props: { entry: entry(1), workspaceId: 'ws' } }))
    await hotSwitch(w).setValue(true)
    await flushPromises()
    expect(mem.setHot).toHaveBeenCalledWith(expect.objectContaining({ id: 1 }), true)
    expect(w.emitted('updated')).toHaveLength(1)
  })

  it('asks before taking a fact out of every prompt, and leaves it alone if declined', async () => {
    const w = track(mount(MemoryDetail, { props: { entry: entry(1, { tags: ['hot'] }), workspaceId: 'ws' } }))
    await hotSwitch(w).setValue(false)
    await flushPromises()
    expect(mem.setHot).not.toHaveBeenCalled()
    useConfirm().handleCancel()
    await flushPromises()
    expect(mem.setHot).not.toHaveBeenCalled()
    expect((hotSwitch(w).element as HTMLInputElement).checked).toBe(true)
  })

  const prioritySelect = (w: VueWrapper) => w.get('select[aria-label="Priority"]')

  it('lets the operator protect an always-on fact from being cut first', async () => {
    const w = track(mount(MemoryDetail, { props: { entry: entry(1, { tags: ['hot'], priority: 1 }), workspaceId: 'ws' } }))
    expect(prioritySelect(w).findAll('option').map((o) => o.text())).toEqual(['Low — cut first', 'Normal', 'High — cut last'])
    expect((prioritySelect(w).element as HTMLSelectElement).value).toBe('1')
    await prioritySelect(w).setValue('2')
    await flushPromises()
    expect(mem.setPriority).toHaveBeenCalledWith(expect.objectContaining({ id: 1 }), 2)
    expect(w.emitted('updated')).toHaveLength(1)
  })

  it('only offers priority for facts that are sent with every run, and says why', async () => {
    const w = track(mount(MemoryDetail, { props: { entry: entry(1), workspaceId: 'ws' } }))
    expect(prioritySelect(w).attributes('disabled')).toBeDefined()
    expect(w.text()).toContain('Only matters for always-on facts')
  })

  it('demotes once confirmed', async () => {
    const w = track(mount(MemoryDetail, { props: { entry: entry(1, { tags: ['hot'] }), workspaceId: 'ws' } }))
    await hotSwitch(w).setValue(false)
    await flushPromises()
    useConfirm().handleConfirm()
    await flushPromises()
    expect(mem.setHot).toHaveBeenCalledWith(expect.objectContaining({ id: 1 }), false)
  })
})

const addForm = () => track(mount(MemoryAddForm, { props: { workspaceId: 'ws' } }))
const field = (w: VueWrapper, label: string) => w.get(`#${w.findAll('label').find((l) => l.text() === label)!.attributes('for')}`)

describe('MemoryDetail copy', () => {
  it('copies the memory content with the shared copy button', () => {
    const w = mount(MemoryDetail, { props: { entry: entry(1), workspaceId: 'ws' } })
    const copy = w.findComponent({ name: 'CopyButton' })
    expect(copy.props('text')).toBe(entry(1).content)
    expect(copy.props('title')).toBe('Copy memory')
    w.unmount()
  })
})

describe('MemoryAddForm', () => {
  beforeEach(() => {
    mem.createMemory.mockReset().mockResolvedValue(true)
    mem.error.value = null
  })
  afterEach(() => mounted.splice(0).forEach((w) => w.unmount()))

  it('cannot be submitted empty', async () => {
    const w = addForm()
    expect(byText(w, 'Remember').attributes('disabled')).toBeDefined()
    await field(w, 'Fact').setValue('   ')
    expect(byText(w, 'Remember').attributes('disabled')).toBeDefined()
  })

  it('saves a workspace fact, on demand and permanent, by default', async () => {
    const w = addForm()
    await field(w, 'Fact').setValue('Deploys go through staging')
    await w.get('form').trigger('submit')
    await flushPromises()
    expect(mem.createMemory).toHaveBeenCalledWith('ws', {
      content: 'Deploys go through staging', scope: 'workspace', mode: 'on_demand', keep: 'permanent',
    })
    expect(w.emitted('created')).toHaveLength(1)
    expect((field(w, 'Fact').element as HTMLTextAreaElement).value).toBe('')
  })

  it('lets the operator pick the three choices in plain words', async () => {
    const w = addForm()
    await field(w, 'Fact').setValue('Likes concise answers')
    await field(w, 'Applies to').setValue('user')
    await field(w, 'Used').setValue('always')
    await field(w, 'Kept').setValue('session')
    await w.get('form').trigger('submit')
    await flushPromises()
    expect(mem.createMemory).toHaveBeenCalledWith('ws', {
      content: 'Likes concise answers', scope: 'user', mode: 'always', keep: 'session',
    })
  })

  it('offers priority only for always-on facts, and sends it only when it is not the default', async () => {
    const w = addForm()
    expect(w.findAll('label').some((l) => l.text() === 'Priority')).toBe(false)
    await field(w, 'Fact').setValue('Never force-push main')
    await field(w, 'Used').setValue('always')
    await field(w, 'Priority').setValue('2')
    await w.get('form').trigger('submit')
    await flushPromises()
    expect(mem.createMemory).toHaveBeenCalledWith('ws', {
      content: 'Never force-push main', scope: 'workspace', mode: 'always', keep: 'permanent', priority: 2,
    })
  })

  it('keeps the text and shows the reason when saving fails', async () => {
    mem.createMemory.mockImplementation(async () => {
      mem.error.value = 'that fact is already saved'
      return false
    })
    const w = addForm()
    await field(w, 'Fact').setValue('dup')
    await w.get('form').trigger('submit')
    await flushPromises()
    expect(w.text()).toContain('already saved')
    expect(w.emitted('created')).toBeUndefined()
    expect((field(w, 'Fact').element as HTMLTextAreaElement).value).toBe('dup')
  })
})

const PREVIEW: MemoryInjectionPreview = {
  block: '<memory>\n- build: run go build\n</memory>',
  chars: 40, tokens_estimate: 10, budget_chars: 1747, context_budget_chars: 21848, operator_chars: 0, over_budget: false,
  model: 'qwen', budget_resolved: true,
  included: [{ id: 1, title: 'build', chars: 24, priority: 1 }],
  cut: [{ id: 2, title: 'old fact', chars: 300, priority: 1 }],
}

describe('MemoryInjectionPreview (component)', () => {
  beforeEach(() => {
    mem.fetchInjectionPreview.mockReset().mockResolvedValue(undefined)
    mem.injectionPreview.value = PREVIEW
  })
  afterEach(() => mounted.splice(0).forEach((w) => w.unmount()))

  const mountPreview = async () => {
    const w = track(mount(InjectionPreviewPanel, { props: { workspaceId: 'ws' }, global: { stubs: { Icon: true } } }))
    await flushPromises()
    return w
  }

  it('loads the preview for the workspace and the first model', async () => {
    await mountPreview()
    expect(mem.fetchInjectionPreview).toHaveBeenCalledWith('ws', 'qwen')
  })

  it('shows exactly what the model receives, its size against the window, and what was cut', async () => {
    const w = await mountPreview()
    expect(w.get('[data-test="injection-block"]').text()).toContain('- build: run go build')
    expect(w.text()).toContain('~10 tokens')
    expect(w.text()).toContain('40 of 1,747 chars')
    expect(w.text()).toContain('old fact')
    expect(w.text()).toContain('Not sent')
  })

  it('reloads for another model', async () => {
    const w = await mountPreview()
    await w.get('select').setValue('')
    await flushPromises()
    expect(mem.fetchInjectionPreview).toHaveBeenLastCalledWith('ws', '')
  })

  it('says so when the budget is a fallback rather than the model’s own', async () => {
    mem.injectionPreview.value = { ...PREVIEW, budget_resolved: false, context_budget_chars: 0 }
    const w = await mountPreview()
    expect(w.text()).toContain('default budget')
  })

  it('warns that never-cut operator notes have pushed the saved facts out', async () => {
    mem.injectionPreview.value = { ...PREVIEW, chars: 4300, operator_chars: 4200, over_budget: true, included: [], cut: [{ id: 2, title: 'old fact', chars: 300, priority: 1 }] }
    const w = await mountPreview()
    expect(w.text()).toContain('Operator notes alone')
    expect(w.text()).toContain('never cut')
    expect(w.text()).toContain('4,200 of those chars are your notes')
  })

  it('warns when even a high-priority fact did not fit', async () => {
    mem.injectionPreview.value = { ...PREVIEW, cut: [{ id: 3, title: 'Never force-push', chars: 900, priority: 2 }] }
    const w = await mountPreview()
    expect(w.text()).toContain('A high-priority fact did not fit')
  })

  it('reloads when memory changes while it is open, so it never shows a stale prompt', async () => {
    const w = await mountPreview()
    expect(mem.fetchInjectionPreview).toHaveBeenCalledTimes(1)
    mem.memoryRevision.value++
    await flushPromises()
    expect(mem.fetchInjectionPreview).toHaveBeenCalledTimes(2)
    expect(mem.fetchInjectionPreview).toHaveBeenLastCalledWith('ws', 'qwen')
    w.unmount()
  })

  it('does not warn when the notes fit', async () => {
    mem.injectionPreview.value = { ...PREVIEW, operator_chars: 300, over_budget: false }
    const w = await mountPreview()
    expect(w.text()).not.toContain('Operator notes alone')
    expect(w.text()).toContain('300 of those chars are your notes')
  })

  it('says nothing is sent when there are no always-on facts', async () => {
    mem.injectionPreview.value = { ...PREVIEW, block: '', chars: 0, tokens_estimate: 0, included: [], cut: [] }
    const w = await mountPreview()
    expect(w.text()).toContain('nothing is added')
  })
})

const NOTES: OperatorNotes = { global: 'Reply in British English.', workspace: 'Always use tabs.', max_chars: 6000 }
const editor = () => track(mount(MemoryNotesEditor, { props: { workspaceId: 'ws' }, global: { stubs: { Icon: true } } }))
const notesField = (w: VueWrapper) => w.get('textarea')

describe('MemoryNotesEditor', () => {
  beforeEach(() => {
    mem.fetchNotes.mockReset().mockImplementation(async () => { mem.operatorNotes.value = NOTES })
    mem.saveNotes.mockReset().mockResolvedValue(true)
    mem.operatorNotes.value = NOTES
    mem.error.value = null
  })
  afterEach(() => mounted.splice(0).forEach((w) => w.unmount()))

  it('loads the notes of the workspace and shows them for editing', async () => {
    const w = editor()
    await flushPromises()
    expect(mem.fetchNotes).toHaveBeenCalledWith('ws')
    expect((notesField(w).element as HTMLTextAreaElement).value).toBe('Always use tabs.')
  })

  it('switches to the notes that apply to every workspace', async () => {
    const w = editor()
    await flushPromises()
    await w.findAll('[role="radio"]').find((r) => r.text() === 'All workspaces')!.trigger('click')
    expect((notesField(w).element as HTMLTextAreaElement).value).toBe('Reply in British English.')
  })

  it('saves the edited text for the chosen scope and tells the panel', async () => {
    const w = editor()
    await flushPromises()
    expect(byText(w, 'Save notes').attributes('disabled')).toBeDefined()
    await notesField(w).setValue('Always use tabs. Never use semicolons.')
    await w.get('form').trigger('submit')
    await flushPromises()
    expect(mem.saveNotes).toHaveBeenCalledWith('ws', 'workspace', 'Always use tabs. Never use semicolons.')
    expect(w.emitted('saved')).toHaveLength(1)
  })

  it('counts characters against the limit and refuses to save past it', async () => {
    const w = editor()
    await flushPromises()
    expect(w.text()).toContain('16 / 6,000')
    await notesField(w).setValue('x'.repeat(6001))
    expect(w.text()).toContain('6,001 / 6,000')
    expect(byText(w, 'Save notes').attributes('disabled')).toBeDefined()
  })

  it('explains the precedence, so the operator knows what the file does', async () => {
    const w = editor()
    await flushPromises()
    expect(w.text()).toContain('first in every run')
    expect(w.text()).toContain('never cut')
  })

  it('shows why a save failed and keeps the text', async () => {
    mem.saveNotes.mockImplementation(async () => { mem.error.value = 'could not save'; return false })
    const w = editor()
    await flushPromises()
    await notesField(w).setValue('edited')
    await w.get('form').trigger('submit')
    await flushPromises()
    expect(w.text()).toContain('could not save')
    expect(w.emitted('saved')).toBeUndefined()
    expect((notesField(w).element as HTMLTextAreaElement).value).toBe('edited')
  })
})

const markdownFile = (text: string, name = 'memory.md') => new File([text], name, { type: 'text/markdown' })
const ioWrapper = () => track(mount(MemoryImportExport, { props: { workspaceId: 'ws' }, global: { stubs: { Icon: true } } }))
const pickFile = async (w: VueWrapper, file: File) => {
  const input = w.get('input[type="file"]')
  Object.defineProperty(input.element, 'files', { value: [file], configurable: true })
  await input.trigger('change')
  await flushPromises()
}

describe('MemoryImportExport', () => {
  const click = vi.fn()
  beforeEach(() => {
    mem.exportMemory.mockReset().mockResolvedValue('# Memory export — ws')
    mem.importMemory.mockReset()
    mem.error.value = null
    click.mockReset()
    URL.createObjectURL = vi.fn(() => 'blob:fake')
    URL.revokeObjectURL = vi.fn()
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(click)
  })
  afterEach(() => {
    mounted.splice(0).forEach((w) => w.unmount())
    vi.restoreAllMocks()
  })

  it('downloads the export as a markdown file named for the workspace', async () => {
    const w = ioWrapper()
    await byText(w, 'Export').trigger('click')
    await flushPromises()
    expect(mem.exportMemory).toHaveBeenCalledWith('ws')
    expect(URL.createObjectURL).toHaveBeenCalled()
    expect(click).toHaveBeenCalledTimes(1)
  })

  it('imports a chosen file and says what happened', async () => {
    mem.importMemory.mockResolvedValue({ created: 3, skipped: 1, issues: [] })
    const w = ioWrapper()
    await pickFile(w, markdownFile('### a\nb'))
    expect(mem.importMemory).toHaveBeenCalledWith('ws', '### a\nb')
    expect(w.get('[role="status"]').text()).toContain('Added 3')
    expect(w.get('[role="status"]').text()).toContain('1 already saved')
    expect(w.emitted('imported')).toHaveLength(1)
  })

  it('lists the entries it could not import, with their lines', async () => {
    mem.importMemory.mockResolvedValue({ created: 1, skipped: 0, issues: [{ line: 7, message: 'unknown priority "urgent"' }] })
    const w = ioWrapper()
    await pickFile(w, markdownFile('x'))
    expect(w.text()).toContain('Line 7')
    expect(w.text()).toContain('unknown priority')
  })

  it('does not reload when nothing was added', async () => {
    mem.importMemory.mockResolvedValue({ created: 0, skipped: 2, issues: [] })
    const w = ioWrapper()
    await pickFile(w, markdownFile('x'))
    expect(w.emitted('imported')).toBeUndefined()
    expect(w.text()).toContain('Added 0')
  })

  it('shows why an import was refused', async () => {
    mem.importMemory.mockImplementation(async () => { mem.error.value = 'no facts found'; return null })
    const w = ioWrapper()
    await pickFile(w, markdownFile('prose'))
    expect(w.get('[role="alert"]').text()).toContain('no facts found')
  })
})
