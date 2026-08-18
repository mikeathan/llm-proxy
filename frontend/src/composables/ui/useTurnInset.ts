import { ref, watch, nextTick, type Ref } from 'vue'
import type { ComputedRef } from 'vue'
import type { Turn } from '../../types/message'
import { isRunningPhase, type InsetPhase } from '../../types/inset'

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

  // resetInsets clears every per-turn collapse flag (all expanded). Used when
  // switching sessions / starting a new chat so stale collapse state from the
  // previous turn set cannot carry over to freshly loaded turns.
  function resetInsets() {
    insetCollapsed.value = {}
  }

  const lastTurnIdx = () => turns.value.length - 1

  watch(phase, async (p, prev) => {
    const running = isRunningPhase(p)
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

  return { insetCollapsed, isInsetCollapsed, toggleInset, collapseAllInsets, resetInsets }
}
