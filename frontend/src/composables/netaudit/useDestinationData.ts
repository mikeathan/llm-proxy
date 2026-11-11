import { ref } from 'vue'
import { NetauditService } from '../../services/netaudit/netauditService'
import { errorMessage } from '../../utils/errors'
import { DESTINATION_DATA_DOWNLOAD_FAILED, DESTINATION_DATA_LOAD_FAILED, DESTINATION_DATA_REMOVE_FAILED } from '../../constants/netaudit'
import type { DestinationDataState } from '../../types/netaudit'

/** The table of address owners as its card shows it. Every change answers with the whole new state, which replaces the old. */
export function useDestinationData() {
  const state = ref<DestinationDataState | null>(null)
  const error = ref('')
  const busy = ref(false)

  async function load(): Promise<void> {
    try {
      state.value = await NetauditService.destinationData()
      error.value = ''
    } catch (err) {
      error.value = errorMessage(err, DESTINATION_DATA_LOAD_FAILED)
    }
  }

  async function run(call: () => Promise<DestinationDataState>, failed: string): Promise<boolean> {
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
    download: () => run(() => NetauditService.downloadDestinationData(), DESTINATION_DATA_DOWNLOAD_FAILED),
    remove: () => run(() => NetauditService.removeDestinationData(), DESTINATION_DATA_REMOVE_FAILED),
  }
}
