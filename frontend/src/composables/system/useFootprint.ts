import { onScopeDispose, ref, watch, type Ref } from 'vue'
import { MetricsApiService } from '../../services/monitoring/metricsService'
import type { ServiceFootprint } from '../../types/metrics'

// The service footprint (the System stats panel's "This service" section): read only while `open` is true, so a closed panel
// costs the server nothing. The first read fires when the panel opens; CPU % is a delta on the server, so it shows "measuring" until
// the second read, FOOTPRINT_POLL_MS later. The poll pauses on a hidden tab, like useMetrics.
export const FOOTPRINT_POLL_MS = 5000

export function useFootprint(open: Ref<boolean>) {
  const footprint = ref<ServiceFootprint | null>(null)
  const failed = ref(false)
  let timer: ReturnType<typeof setInterval> | null = null
  let reqId = 0 // a response that arrives after the panel closed (or a newer read) is dropped

  const refresh = async (): Promise<void> => {
    const mine = ++reqId
    try {
      const data = await MetricsApiService.fetchFootprint()
      if (mine !== reqId) return
      footprint.value = data
      failed.value = false
    } catch {
      if (mine !== reqId) return
      failed.value = true
    }
  }

  const tabHidden = () => typeof document !== 'undefined' && document.visibilityState === 'hidden'
  const stop = () => {
    if (timer) clearInterval(timer)
    timer = null
  }
  const start = () => {
    void refresh()
    if (!tabHidden()) timer = setInterval(refresh, FOOTPRINT_POLL_MS)
  }
  const onVisibility = () => {
    if (!open.value) return
    stop()
    if (!tabHidden()) start()
  }

  watch(
    open,
    (isOpen) => {
      stop()
      if (isOpen) start()
      else reqId++
    },
    { immediate: true },
  )
  if (typeof document !== 'undefined') {
    document.addEventListener('visibilitychange', onVisibility)
    onScopeDispose(() => document.removeEventListener('visibilitychange', onVisibility))
  }
  onScopeDispose(() => {
    stop()
    reqId++
  })

  return { footprint, failed, refresh }
}
