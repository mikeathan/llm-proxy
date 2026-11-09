import { usePersistedState } from '../ui/usePersistedState'
import { DEFAULT_PROBLEM_LAYOUT, PROBLEM_LAYOUT_KEY, PROBLEM_LAYOUT_VERSION } from '../../constants/netaudit'
import type { ProblemLayout } from '../../types/netaudit'

const parse = (data: unknown): ProblemLayout | null => (data === 'check' || data === 'device' ? data : null)

// How the problems are laid out (one card per kind, or one per device): one preference for the page, remembered per person.
const layout = usePersistedState<ProblemLayout>(PROBLEM_LAYOUT_KEY, { version: PROBLEM_LAYOUT_VERSION, parse, fallback: DEFAULT_PROBLEM_LAYOUT })

export function useProblemLayout() {
  return { layout, set: (next: ProblemLayout) => (layout.value = next) }
}
