import { computed, onMounted, ref } from 'vue'
import { useDispatcher } from '../automation/useDispatcher'
import { AUDITOR_PROFILE } from '../../constants/netaudit'
import type { NetworkAudit } from '../../types/netaudit'

/**
 * The network audit the owner has set up: the automation that runs the auditor profile, or null when there is none. `loaded` is false until
 * the automations have been read, so nothing offers "set up" while an audit exists. The Network page and the dashboard's cards that read
 * the audit's results share this one way of finding it.
 */
export function useNetworkAudit() {
  const { automations, fetchAutomations } = useDispatcher()
  const loaded = ref(false)
  const audit = computed<NetworkAudit | null>(() => {
    const a = automations.value.find((x) => x.profile === AUDITOR_PROFILE)
    return a ? { workspaceId: a.workspace, automation: a.name } : null
  })
  onMounted(async () => {
    await fetchAutomations(true)
    loaded.value = true
  })
  return { audit, loaded }
}
