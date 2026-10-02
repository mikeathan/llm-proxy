import { describe, it, expect } from 'vitest'
import { formatMemoryUsage, isMemoryUnused } from '../../../utils/format/memoryUsage'
import type { MemoryEntry } from '../../../types/memory'

const entry = (over: Partial<MemoryEntry> = {}): MemoryEntry => ({
  id: 1, workspace_id: 'ws', memory_type: 'long_term', title: 't', content: 'c', source: 'agent',
  created_at: '2026-09-29T10:00:00Z', updated_at: '2026-09-29T10:00:00Z', tags: [], ...over,
})

describe('formatMemoryUsage', () => {
  it('says a fact was never used when nothing sent or found it', () => {
    expect(formatMemoryUsage(entry())).toBe('Never used')
    expect(formatMemoryUsage(entry({ injected_count: 0, searched_count: 0 }))).toBe('Never used')
  })

  it('counts the runs an always-on fact was sent in', () => {
    expect(formatMemoryUsage(entry({ injected_count: 1 }))).toBe('Sent in 1 run')
    expect(formatMemoryUsage(entry({ injected_count: 12 }))).toBe('Sent in 12 runs')
  })

  it('counts how often search returned it', () => {
    expect(formatMemoryUsage(entry({ searched_count: 1 }))).toBe('Found by search 1 time')
    expect(formatMemoryUsage(entry({ searched_count: 3 }))).toBe('Found by search 3 times')
  })

  it('joins both when both apply', () => {
    expect(formatMemoryUsage(entry({ injected_count: 12, searched_count: 3 }))).toBe('Sent in 12 runs · Found by search 3 times')
  })

  it('groups thousands', () => {
    expect(formatMemoryUsage(entry({ injected_count: 1500 }))).toBe('Sent in 1,500 runs')
  })
})

describe('isMemoryUnused', () => {
  it('is true only when neither counter has moved', () => {
    expect(isMemoryUnused(entry())).toBe(true)
    expect(isMemoryUnused(entry({ injected_count: 1 }))).toBe(false)
    expect(isMemoryUnused(entry({ searched_count: 1 }))).toBe(false)
  })
})
