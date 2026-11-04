import { ref, watch, type Ref } from 'vue'
import { NetauditService } from '../../services/netaudit/netauditService'
import { errorMessage } from '../../utils/errors'
import { ACTIVITY_LOAD_FAILED } from '../../constants/netaudit'
import type { ActivityRow, NetworkAudit } from '../../types/netaudit'

/**
 * The Activity tab's timeline. It reads one cheap endpoint (nothing in it starts a scan or a probe) when the tab opens, when
 * the audit becomes known or the number of rows changes, and when asked to refresh. An older answer that arrives after a
 * newer one is dropped, so a slow read never replaces what the screen already shows.
 */
export function useNetworkActivity(audit: Ref<NetworkAudit | null>, limit: Ref<number>) {
  const rows = ref<ActivityRow[]>([])
  const loaded = ref(false)
  const loading = ref(false)
  const error = ref('')
  let latest = 0

  async function load(): Promise<void> {
    const mine = ++latest
    loading.value = true
    try {
      const activity = await NetauditService.activity(audit.value ?? undefined, limit.value)
      if (mine !== latest) return
      rows.value = activity.rows
      loaded.value = true
      error.value = ''
    } catch (err) {
      if (mine !== latest) return
      error.value = errorMessage(err, ACTIVITY_LOAD_FAILED)
    } finally {
      if (mine === latest) loading.value = false
    }
  }

  watch([() => audit.value?.automation, () => audit.value?.workspaceId, limit], () => void load())
  return { rows, loaded, loading, error, load }
}
