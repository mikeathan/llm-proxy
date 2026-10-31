import { getCurrentScope, onScopeDispose, ref, type Ref } from 'vue'
import { NetauditService } from '../../services/netaudit/netauditService'
import { errorMessage } from '../../utils/errors'
import { NETWORK_OVERVIEW_LOAD_FAILED, NETWORK_REFRESH_MS } from '../../constants/netaudit'
import type { NetworkAudit, NetworkOverview } from '../../types/netaudit'

/**
 * The Network page's overview. It reads one cheap endpoint (nothing in it starts a scan) and refreshes while the page is
 * open and visible: the timer stops with the page, and a hidden tab does not poll.
 */
export function useNetworkOverview(audit: Ref<NetworkAudit | null>) {
  const overview = ref<NetworkOverview | null>(null)
  const error = ref('')
  const loading = ref(false)
  let timer: ReturnType<typeof setInterval> | undefined
  let inFlight = false
  let again = false

  async function load(): Promise<void> {
    if (inFlight) {
      // Asked again while reading (the audit became known): read once more when this one ends, so the answer is never stale.
      again = true
      return
    }
    inFlight = true
    loading.value = !overview.value
    try {
      overview.value = await NetauditService.overview(audit.value ?? undefined)
      error.value = ''
    } catch (err) {
      error.value = errorMessage(err, NETWORK_OVERVIEW_LOAD_FAILED)
    } finally {
      inFlight = false
      loading.value = false
    }
    if (again) {
      again = false
      await load()
    }
  }

  const onVisible = () => {
    if (document.visibilityState === 'visible') void load()
  }

  function start(): void {
    stop()
    void load()
    timer = setInterval(() => {
      if (document.visibilityState === 'visible') void load()
    }, NETWORK_REFRESH_MS)
    document.addEventListener('visibilitychange', onVisible)
  }

  function stop(): void {
    clearInterval(timer)
    timer = undefined
    document.removeEventListener('visibilitychange', onVisible)
  }

  if (getCurrentScope()) onScopeDispose(stop)
  return { overview, error, loading, load, start, stop }
}
