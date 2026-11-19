import { computed, effectScope, onUnmounted, type ComputedRef, type EffectScope, type Ref } from 'vue'
import { useDispatcher } from '../automation/useDispatcher'
import { useNetworkOverview } from './useNetworkOverview'
import { AUDITOR_PROFILE } from '../../constants/netaudit'
import type { NetworkAudit, NetworkOverview } from '../../types/netaudit'

/** What the Network cards read: the overview the server already knows and whether an audit is set up. */
interface Snapshot {
  overview: Ref<NetworkOverview | null>
  error: Ref<string>
  hasAudit: ComputedRef<boolean>
}

// One reader for every Network card on screen: they share one overview and one refresh timer, which starts with the first card and
// stops with the last (a board with six Network cards does not poll six times). The state lives in a detached scope so it outlives
// the card that happened to create it.
let scope: EffectScope | null = null
let shared: (Snapshot & { start: () => void; stop: () => void; fetchAutomations: (force?: boolean) => Promise<unknown> }) | null = null
let consumers = 0

function create() {
  scope = effectScope(true)
  shared = scope.run(() => {
    const { automations, fetchAutomations } = useDispatcher()
    const audit = computed<NetworkAudit | null>(() => {
      const a = automations.value.find((x) => x.profile === AUDITOR_PROFILE)
      return a ? { workspaceId: a.workspace, automation: a.name } : null
    })
    const o = useNetworkOverview(audit)
    return { overview: o.overview, error: o.error, hasAudit: computed(() => audit.value !== null), start: o.start, stop: o.stop, fetchAutomations }
  })!
}

export function useNetworkSnapshot(): Snapshot {
  if (!shared) create()
  const s = shared!
  // Taken in setup, not on mount: when a board changes, the new cards are set up before or after the old ones are removed, and in
  // neither order may the count reach zero while a card still needs the reader.
  if (consumers++ === 0) {
    s.start()
    void s.fetchAutomations(true)
  }
  onUnmounted(() => {
    if (--consumers === 0) {
      s.stop()
      scope?.stop()
      scope = null
      shared = null
    }
  })
  return { overview: s.overview, error: s.error, hasAudit: s.hasAudit }
}
