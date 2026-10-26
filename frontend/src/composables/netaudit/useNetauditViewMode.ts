import { computed } from 'vue'
import { usePersistedState } from '../ui/usePersistedState'
import { DEFAULT_VIEW_MODE, NETAUDIT_VIEW_KEY, NETAUDIT_VIEW_VERSION } from '../../constants/netaudit'
import type { NetauditViewMode } from '../../types/netaudit'

const parse = (data: unknown): NetauditViewMode | null => (data === 'simple' || data === 'advanced' ? data : null)

// One module-level preference shared by every network scan screen: switching on one screen switches them all, and
// the choice is remembered per person. Nothing is lost on switching; both views read the same data.
const mode = usePersistedState<NetauditViewMode>(NETAUDIT_VIEW_KEY, {
  version: NETAUDIT_VIEW_VERSION,
  parse,
  fallback: DEFAULT_VIEW_MODE,
})

export function useNetauditViewMode() {
  return {
    mode,
    advanced: computed(() => mode.value === 'advanced'),
    set: (next: NetauditViewMode) => (mode.value = next),
  }
}
