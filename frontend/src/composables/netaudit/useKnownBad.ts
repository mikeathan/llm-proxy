import { ref } from 'vue'
import { NetauditService } from '../../services/netaudit/netauditService'
import { errorMessage } from '../../utils/errors'
import { KNOWN_BAD_DOWNLOAD_FAILED, KNOWN_BAD_LOAD_FAILED, KNOWN_BAD_SAVE_FAILED } from '../../constants/netaudit'
import type { KnownBadState } from '../../types/netaudit'

/** The malware host-name list as its card shows it. Every change answers with the whole new state, which replaces the old. */
export function useKnownBad() {
  const state = ref<KnownBadState | null>(null)
  const error = ref('')
  const busy = ref(false)

  async function load(): Promise<void> {
    try {
      state.value = await NetauditService.knownBad()
      error.value = ''
    } catch (err) {
      error.value = errorMessage(err, KNOWN_BAD_LOAD_FAILED)
    }
  }

  async function run(call: () => Promise<KnownBadState>, failed: string): Promise<boolean> {
    busy.value = true
    try {
      state.value = await call()
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
    state, error, busy, load,
    saveAddress: (address: string) => run(() => NetauditService.saveKnownBadAddress(address.trim()), KNOWN_BAD_SAVE_FAILED),
    removeAddress: () => run(() => NetauditService.removeKnownBadAddress(), KNOWN_BAD_SAVE_FAILED),
    download: () => run(() => NetauditService.downloadKnownBad(), KNOWN_BAD_DOWNLOAD_FAILED),
  }
}
