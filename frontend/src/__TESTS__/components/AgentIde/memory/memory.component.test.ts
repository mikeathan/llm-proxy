import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { ref } from 'vue'
import { useConfirm } from '../../../../composables/ui/useConfirm'
import type { MemoryEntry } from '../../../../types/memory'

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
}
vi.mock('../../../../composables/memory/useMemory', () => ({ useMemory: () => mem }))

import MemoryPanel from '../../../../components/AgentIde/memory/MemoryPanel.vue'
import MemoryDetail from '../../../../components/AgentIde/memory/MemoryDetail.vue'

const entry = (id: number, over: Partial<MemoryEntry> = {}): MemoryEntry => ({
  id, workspace_id: 'ws', memory_type: 'long_term', title: `Fact ${id}`, content: `content ${id}`, source: 'agent',
  created_at: '2026-09-29T10:00:00Z', updated_at: '2026-09-29T10:00:00Z', ...over,
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
  beforeEach(() => mem.updateMemory.mockReset().mockResolvedValue(undefined))
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
    await w.get('input').setValue('New title')
    await w.get('textarea').setValue('New content')
    expect(byText(w, 'Save').attributes('type')).toBe('submit')
    await w.get('form').trigger('submit')
    await flushPromises()
    expect(mem.updateMemory).toHaveBeenCalledWith('ws', 1, 'New title', 'New content')
    expect(w.emitted('updated')).toHaveLength(1)
  })
})
