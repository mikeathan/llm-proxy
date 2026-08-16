import { ref, watch, nextTick, type Ref } from 'vue'
import type { ComputedRef } from 'vue'
import type { Turn } from '../../utils/message/turnGrouper'
import type { InsetPhase } from '../../utils/message/messageBuilder'

// Shared reasoning-inset collapse state + auto behavior for the assistant chat
// and automation run views (single consumer of useMessageBuilder drives both).
//
// Behavior:
//   - Auto-expand the active (last) turn's inset on the run-start transition
//     (idle → thinking/working/generating), so streaming reasoning is visible.
//   - Auto-collapse it once the turn finalizes (done), leaving the final answer.
//   - Manual toggles are respected mid-run: only the idle→running hop re-expands,
//     so a user-collapsed inset stays collapsed through the rest of the turn.
export function useTurnInset(
  phase: Ref<InsetPhase>,
  turns: Ref<Turn[]> | ComputedRef<Turn[]>,
) {
  const insetCollapsed = ref<Record<number, boolean>>({})

  function isInsetCollapsed(turnIdx: number): boolean {
    return !!insetCollapsed.value[turnIdx]
  }

  function toggleInset(turnIdx: number) {
    const current = !!insetCollapsed.value[turnIdx]
    insetCollapsed.value = { ...insetCollapsed.value, [turnIdx]: !current }
  }

  function collapseAllInsets() {
    const collapsed: Record<number, boolean> = {}
    turns.value.forEach((_, idx) => { collapsed[idx] = true })
    insetCollapsed.value = collapsed
  }

  const lastTurnIdx = () => turns.value.length - 1

  watch(phase, async (p, prev) => {
    const running = p === 'thinking' || p === 'working' || p === 'generating'
    if (!running && p !== 'done') return
    await nextTick()
    const idx = lastTurnIdx()
    if (idx < 0) return
    if (running && prev === 'idle') {
      insetCollapsed.value = { ...insetCollapsed.value, [idx]: false }
    } else if (p === 'done') {
      insetCollapsed.value = { ...insetCollapsed.value, [idx]: true }
    }
  })

  return { insetCollapsed, isInsetCollapsed, toggleInset, collapseAllInsets }
}
