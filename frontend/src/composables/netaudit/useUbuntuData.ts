import { ref } from 'vue'
import { NetauditService } from '../../services/netaudit/netauditService'
import { errorMessage } from '../../utils/errors'
import { UBUNTU_CLEAR_FAILED, UBUNTU_LOAD_FAILED, UBUNTU_UPDATE_FAILED } from '../../constants/netaudit'
import type { UbuntuDataStatus } from '../../types/netaudit'

/**
 * The Ubuntu security data card's state. Nothing runs by itself: the status is read when the card opens, and an update is one
 * click, one request that answers when the (small) download is done. A second click while it runs does nothing, and the server
 * refuses a second update anyway.
 */
export function useUbuntuData() {
  const status = ref<UbuntuDataStatus | null>(null)
  const error = ref('')
  const actionError = ref('')
  const busy = ref(false)

  async function refresh(): Promise<void> {
    try {
      status.value = await NetauditService.ubuntuData()
      error.value = ''
    } catch (err) {
      error.value = errorMessage(err, UBUNTU_LOAD_FAILED)
    }
  }

  async function act(call: () => Promise<UbuntuDataStatus>, failed: string): Promise<void> {
    if (busy.value) return
    busy.value = true
    try {
      status.value = await call()
      actionError.value = ''
    } catch (err) {
      actionError.value = errorMessage(err, failed)
      await refresh()
    } finally {
      busy.value = false
    }
  }

  return {
    status,
    error,
    actionError,
    busy,
    refresh,
    update: () => act(NetauditService.updateUbuntuData, UBUNTU_UPDATE_FAILED),
    clear: () => act(NetauditService.clearUbuntuData, UBUNTU_CLEAR_FAILED),
  }
}
