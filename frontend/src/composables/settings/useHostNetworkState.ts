// Read-only host network state for surfaces that only need to know whether the
// host master switch is OFF (the automation form warns that per-run grants are
// inert while it is). State stays 'unknown' when the fetch fails or the backend
// is unreachable — the consumer then shows nothing instead of guessing.
import { computed, ref } from 'vue'
import { AdminApiService } from '../../services/admin/adminService'
import { isHostNetworkOff } from '../../utils/sandboxing'
import type { HostNetworkState } from '../../types/admin'

export function useHostNetworkState() {
  const state = ref<HostNetworkState>('unknown')

  const hostNetworkOff = computed(() => state.value === 'off')
  const hostNetworkKnown = computed(() => state.value !== 'unknown')

  async function load(): Promise<void> {
    try {
      const settings = await AdminApiService.fetchHostSettings()
      state.value = isHostNetworkOff(settings.sandboxing) ? 'off' : 'on'
    } catch {
      state.value = 'unknown'
    }
  }

  return { state, hostNetworkOff, hostNetworkKnown, load }
}
