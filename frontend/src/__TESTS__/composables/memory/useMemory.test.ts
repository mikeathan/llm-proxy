import { describe, it, expect, vi, beforeEach } from 'vitest'
import type { MemoryEntry } from '../../../types/memory'

const svc = vi.hoisted(() => ({
  list: vi.fn(), search: vi.fn(), update: vi.fn(), delete: vi.fn(), clearAll: vi.fn(), create: vi.fn(),
  saveNotes: vi.fn(), getNotes: vi.fn(), importMarkdown: vi.fn(), exportMarkdown: vi.fn(), injectionPreview: vi.fn(),
}))
vi.mock('../../../services/memory/memoryService', () => ({ MemoryService: svc }))

import { useMemory } from '../../../composables/memory/useMemory'

const entry = { id: 1, workspace_id: 'ws', memory_type: 'long_term', title: 't', content: 'c', source: 'agent', tags: [], created_at: '', updated_at: '' } as MemoryEntry

// The open "what the model receives" preview reloads whenever this moves, so
// every change to what the model would see must move it — and a failed or
// empty change must not.
describe('useMemory change signal', () => {
  beforeEach(() => Object.values(svc).forEach((f) => f.mockReset().mockResolvedValue(undefined)))

  it('moves after each successful change', async () => {
    const m = useMemory()
    const changes: Array<[string, () => Promise<unknown>]> = [
      ['create', () => m.createMemory('ws', { content: 'x', scope: 'workspace', mode: 'always', keep: 'permanent' })],
      ['promote or demote', () => m.setHot(entry, true)],
      ['priority', () => m.setPriority(entry, 2)],
      ['edit', () => m.updateMemory('ws', 1, 't', 'c')],
      ['delete', () => m.deleteMemory('ws', 1)],
      ['clear', () => m.clearAllMemories('ws')],
      ['save notes', () => m.saveNotes('ws', 'workspace', 'n')],
    ]
    for (const [name, change] of changes) {
      const before = m.memoryRevision.value
      await change()
      expect(m.memoryRevision.value, name).toBeGreaterThan(before)
    }
  })

  it('moves after an import that added facts, not after one that added none', async () => {
    const m = useMemory()
    svc.importMarkdown.mockResolvedValue({ created: 0, skipped: 2, issues: [] })
    let before = m.memoryRevision.value
    await m.importMemory('ws', 'x')
    expect(m.memoryRevision.value).toBe(before)

    svc.importMarkdown.mockResolvedValue({ created: 2, skipped: 0, issues: [] })
    before = m.memoryRevision.value
    await m.importMemory('ws', 'x')
    expect(m.memoryRevision.value).toBeGreaterThan(before)
  })

  it('does not move when a change fails', async () => {
    const m = useMemory()
    svc.create.mockRejectedValue(new Error('already saved'))
    svc.update.mockRejectedValue(new Error('boom'))
    svc.saveNotes.mockRejectedValue(new Error('too long'))
    const before = m.memoryRevision.value
    await m.createMemory('ws', { content: 'x', scope: 'workspace', mode: 'on_demand', keep: 'permanent' })
    await m.setHot(entry, true)
    await m.setPriority(entry, 2)
    await m.saveNotes('ws', 'workspace', 'n')
    expect(m.memoryRevision.value).toBe(before)
  })
})

// "All" must show everything that applies to a workspace — its own facts and the
// user-wide ones (stored under "global") that are sent to the model in every
// workspace. Before this, the user-wide facts were only visible under "User".
describe('useMemory.fetchMemories', () => {
  const fact = (id: number, ws: string, updated: string): MemoryEntry => ({ ...entry, id, workspace_id: ws, updated_at: updated })
  beforeEach(() => Object.values(svc).forEach((f) => f.mockReset().mockResolvedValue(undefined)))

  it('merges the workspace facts and the user-wide facts, newest first', async () => {
    svc.list.mockImplementation(async (ws: string) =>
      ws === 'global' ? [fact(1, 'global', '2026-08-20 10:00:00')] : [fact(2, 'ws', '2026-09-01 10:00:00'), fact(3, 'ws', '2026-08-01 10:00:00')])
    const m = useMemory()
    await m.fetchMemories('ws')
    expect(svc.list).toHaveBeenCalledWith('ws', undefined, undefined)
    expect(svc.list).toHaveBeenCalledWith('global', undefined, undefined)
    expect(m.memories.value.map((e) => e.id)).toEqual([2, 1, 3])
  })

  it('stays a single request when a type, a view or the global workspace is chosen', async () => {
    svc.list.mockResolvedValue([])
    const m = useMemory()
    await m.fetchMemories('ws', 'daily')
    await m.fetchMemories('ws', undefined, 'hot')
    await m.fetchMemories('ws', undefined, 'unused')
    await m.fetchMemories('global', 'user_profile')
    expect(svc.list).toHaveBeenCalledTimes(4)
  })

  it('still shows the workspace facts when the user-wide ones cannot be loaded', async () => {
    svc.list.mockImplementation(async (ws: string) => {
      if (ws === 'global') throw new Error('boom')
      return [fact(2, 'ws', '2026-09-01 10:00:00')]
    })
    const m = useMemory()
    await m.fetchMemories('ws')
    expect(m.memories.value.map((e) => e.id)).toEqual([2])
  })
})
