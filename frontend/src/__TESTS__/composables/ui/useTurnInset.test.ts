import { describe, it, expect } from 'vitest'
import { ref, nextTick } from 'vue'
import { useTurnInset } from '../../../composables/ui/useTurnInset'
import type { Turn } from '../../../utils/message/turnGrouper'
import type { InsetPhase } from '../../../utils/message/messageBuilder'

const dummyTurn = (): Turn => ({ userMessage: 'u', finalAnswer: '', segments: [], messages: [] })

// Watchers use flush:'pre' and the callback awaits nextTick itself, so the
// auto expand/collapse needs two ticks to settle.
async function flush() {
  await nextTick()
  await nextTick()
}

function setup() {
  const phase = ref<InsetPhase>('idle')
  const turns = ref<Turn[]>([dummyTurn()])
  const inset = useTurnInset(phase, turns)
  return { phase, turns, ...inset }
}

describe('useTurnInset', () => {
  it('defaults all turns to expanded', () => {
    const { isInsetCollapsed } = setup()
    expect(isInsetCollapsed(0)).toBe(false)
  })

  it('expands the active turn on the idle→thinking run start', async () => {
    const { phase, isInsetCollapsed } = setup()
    phase.value = 'thinking'
    await flush()
    expect(isInsetCollapsed(0)).toBe(false)
  })

  it('expands on idle→working when the run starts with a tool call', async () => {
    const { phase, isInsetCollapsed } = setup()
    phase.value = 'working'
    await flush()
    expect(isInsetCollapsed(0)).toBe(false)
  })

  it('collapses the active turn when the run finishes', async () => {
    const { phase, isInsetCollapsed } = setup()
    phase.value = 'thinking'
    await flush()
    phase.value = 'done'
    await flush()
    expect(isInsetCollapsed(0)).toBe(true)
  })

  it('respects a manual collapse for the rest of the run (no re-expand on phase hops)', async () => {
    const { phase, toggleInset, isInsetCollapsed } = setup()
    phase.value = 'thinking'
    await flush()
    toggleInset(0)
    expect(isInsetCollapsed(0)).toBe(true)
    phase.value = 'working'
    await flush()
    expect(isInsetCollapsed(0)).toBe(true)
    phase.value = 'generating'
    await flush()
    expect(isInsetCollapsed(0)).toBe(true)
  })

  it('manual toggle still works', async () => {
    const { toggleInset, isInsetCollapsed } = setup()
    toggleInset(0)
    expect(isInsetCollapsed(0)).toBe(true)
    toggleInset(0)
    expect(isInsetCollapsed(0)).toBe(false)
  })

  it('collapseAllInsets collapses every turn', () => {
    const { turns, collapseAllInsets, isInsetCollapsed } = setup()
    turns.value = [dummyTurn(), dummyTurn()]
    collapseAllInsets()
    expect(isInsetCollapsed(0)).toBe(true)
    expect(isInsetCollapsed(1)).toBe(true)
  })

  it('does nothing on phase transitions when there are no turns', async () => {
    const phase = ref<InsetPhase>('idle')
    const turns = ref<Turn[]>([])
    const { isInsetCollapsed } = useTurnInset(phase, turns)
    phase.value = 'thinking'
    await flush()
    expect(isInsetCollapsed(0)).toBe(false)
    phase.value = 'done'
    await flush()
    expect(isInsetCollapsed(0)).toBe(false)
  })
})
