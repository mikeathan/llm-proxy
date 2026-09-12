// Per-turn tool/step segment expansion state, shared by the assistant chat and
// the automation run-details views (keyed `${turnIdx}-${segIdx}`). Mirrors the
// sibling useTurnInset pattern: one source of truth so the two consumers cannot
// drift.
import { ref } from 'vue'

export function useExpandedSegments() {
  const expandedSegments = ref<Record<string, boolean>>({})

  function isSegExpanded(turnIdx: number, segIdx: number): boolean {
    return expandedSegments.value[`${turnIdx}-${segIdx}`] === true
  }

  function toggleSegment(turnIdx: number, segIdx: number): void {
    const key = `${turnIdx}-${segIdx}`
    expandedSegments.value = { ...expandedSegments.value, [key]: !expandedSegments.value[key] }
  }

  return { expandedSegments, isSegExpanded, toggleSegment }
}
