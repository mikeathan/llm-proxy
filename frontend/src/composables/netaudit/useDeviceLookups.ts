import { computed, ref } from 'vue'
import { NetauditService } from '../../services/netaudit/netauditService'
import { errorMessage } from '../../utils/errors'
import { LOOKUPS_LOAD_FAILED } from '../../constants/netaudit'
import type { DeviceLookups } from '../../types/netaudit'

/**
 * What each device on the network asked the router's DNS for, read from the phone-home watch (Constitution I.8). One read per visit: the
 * names change slowly, and the screen offers a refresh. `enabled` is false while the watch is off, so the screen can say so instead of
 * showing an empty list as if nothing had been asked.
 */
export function useDeviceLookups() {
  const devices = ref<DeviceLookups[]>([])
  const enabled = ref(true)
  const since = ref<string | null>(null)
  const loaded = ref(false)
  const loading = ref(false)
  const error = ref('')

  async function load(): Promise<void> {
    if (loading.value) return
    loading.value = true
    try {
      const state = await NetauditService.deviceLookups()
      devices.value = Array.isArray(state?.devices) ? state.devices : []
      enabled.value = state?.enabled !== false
      since.value = state?.since ?? null
      error.value = ''
    } catch (err) {
      error.value = errorMessage(err, LOOKUPS_LOAD_FAILED)
    } finally {
      loading.value = false
      loaded.value = true
    }
  }
  const byMac = (mac: string): DeviceLookups | undefined => devices.value.find((d) => d.mac.toLowerCase() === mac.toLowerCase())
  return { devices, enabled, since, loaded, loading, error, load, byMac, count: computed(() => devices.value.length) }
}
