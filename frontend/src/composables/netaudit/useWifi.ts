import { ref } from 'vue'
import { NetauditService } from '../../services/netaudit/netauditService'
import { errorMessage } from '../../utils/errors'
import { WIFI_LOAD_FAILED } from '../../constants/netaudit'
import type { WifiState } from '../../types/netaudit'

/**
 * The Wi-Fi look-alikes card. Every change answers with the whole new state, which replaces the old; a refusal leaves the state as the
 * server last said and puts its words in `error`, beside the control it was about.
 */
export function useWifi() {
  const state = ref<WifiState | null>(null)
  const error = ref('')
  const busy = ref(false)

  async function run(change: () => Promise<WifiState>, failed: string): Promise<boolean> {
    busy.value = true
    try {
      state.value = await change()
      error.value = ''
      return true
    } catch (err) {
      error.value = errorMessage(err, failed)
      return false
    } finally {
      busy.value = false
    }
  }

  return {
    state, error, busy,
    load: () => run(() => NetauditService.wifi(), WIFI_LOAD_FAILED),
    look: () => run(() => NetauditService.wifiLook(), 'The look could not be made.'),
    addNetwork: (name: string) => run(() => NetauditService.addWifiNetwork(name), 'The name could not be added.'),
    removeNetwork: (name: string) => run(() => NetauditService.removeWifiNetwork(name), 'The name could not be removed.'),
    confirm: (name: string, addresses: string[]) => run(() => NetauditService.confirmWifi(name, addresses), 'The access points could not be confirmed.'),
  }
}
