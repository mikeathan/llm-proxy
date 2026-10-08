import { describe, it, expect, vi, beforeEach } from 'vitest'
import { ref } from 'vue'
import type { MemorySuggestion } from '../../../types/memory'

const { reviewMock, createMock } = vi.hoisted(() => ({ reviewMock: vi.fn(), createMock: vi.fn() }))
vi.mock('../../../services/assistant/assistantService', () => ({ AssistantService: { reviewMemories: reviewMock } }))
vi.mock('../../../composables/memory/useMemory', () => ({ useMemory: () => ({ createMemory: createMock, error: ref(null) }) }))

import { useMemoryReview } from '../../../composables/memory/useMemoryReview'

const suggestion = (content: string, over: Partial<MemorySuggestion> = {}): MemorySuggestion =>
  ({ content, scope: 'workspace', mode: 'on_demand', duplicate: false, ...over })

const make = (session: string | null = 'conv_1') => useMemoryReview(ref('ws'), ref(session))

describe('useMemoryReview', () => {
  beforeEach(() => {
    reviewMock.mockReset().mockResolvedValue([suggestion('We deploy through vertex'), suggestion('Answer briefly', { scope: 'user', mode: 'always' }), suggestion('Already known', { duplicate: true })])
    createMock.mockReset().mockResolvedValue(true)
  })

  it('opens, reads the chat once, and ticks everything that is new', async () => {
    const r = make()
    await r.start()
    expect(r.open.value).toBe(true)
    expect(reviewMock).toHaveBeenCalledWith('ws', 'conv_1')
    expect(r.items.value.map((i) => [i.content, i.selected])).toEqual([['We deploy through vertex', true], ['Answer briefly', true], ['Already known', false]])
    expect(r.loading.value).toBe(false)
  })

  it('says why it failed and lets the operator try again', async () => {
    reviewMock.mockRejectedValueOnce(new Error('The model is busy right now. Try again in a moment.'))
    const r = make()
    await r.start()
    expect(r.error.value).toBe('The model is busy right now. Try again in a moment.')
    expect(r.items.value).toEqual([])
    await r.start()
    expect(r.error.value).toBe('')
    expect(r.items.value).toHaveLength(3)
  })

  it('does nothing without a saved conversation', async () => {
    const r = make(null)
    await r.start()
    expect(reviewMock).not.toHaveBeenCalled()
    expect(r.open.value).toBe(false)
  })

  it('lets the operator tick and untick, but never an already-saved fact', async () => {
    const r = make()
    await r.start()
    r.toggle(0)
    expect(r.items.value[0]!.selected).toBe(false)
    r.toggle(2)
    expect(r.items.value[2]!.selected).toBe(false)
    expect(r.selectedCount.value).toBe(1)
  })

  it('saves only what is ticked, as permanent facts with the proposed scope and mode, then closes', async () => {
    const r = make()
    await r.start()
    r.toggle(0)
    const saved = await r.save()
    expect(saved).toBe(1)
    expect(createMock).toHaveBeenCalledTimes(1)
    expect(createMock).toHaveBeenCalledWith('ws', { content: 'Answer briefly', scope: 'user', mode: 'always', keep: 'permanent' })
    expect(r.open.value).toBe(false)
  })

  it('writes nothing when nothing is ticked', async () => {
    const r = make()
    await r.start()
    r.items.value.forEach((_, i) => { if (r.items.value[i]!.selected) r.toggle(i) })
    expect(await r.save()).toBe(0)
    expect(createMock).not.toHaveBeenCalled()
  })

  it('keeps a fact that failed to save in the list, marked, and drops the ones that saved', async () => {
    createMock.mockResolvedValueOnce(true).mockResolvedValueOnce(false)
    const r = make()
    await r.start()
    const saved = await r.save()
    expect(saved).toBe(1)
    expect(r.open.value).toBe(true)
    expect(r.items.value.map((i) => [i.content, i.failed])).toEqual([['Answer briefly', true], ['Already known', false]])
    expect(r.error.value).toContain('could not be saved')
  })

  it('forgets everything on close', async () => {
    const r = make()
    await r.start()
    r.close()
    expect(r.open.value).toBe(false)
    expect(r.items.value).toEqual([])
  })
})
