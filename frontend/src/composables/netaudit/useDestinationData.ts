import { ref } from 'vue'
import { NetauditService } from '../../services/netaudit/netauditService'
import { errorMessage } from '../../utils/errors'
import { DESTINATION_DATA_DOWNLOAD_FAILED, DESTINATION_DATA_LOAD_FAILED, DESTINATION_DATA_REMOVE_FAILED } from '../../constants/netaudit'
import type { DestinationDataState, DestinationTable } from '../../types/netaudit'

// Each table has its own three calls; the rest of the card is the same for both.
const CALLS: Readonly<Record<DestinationTable, { load: () => Promise<DestinationDataState>; download: () => Promise<DestinationDataState>; remove: () => Promise<DestinationDataState> }>> = {
  owners: { load: () => NetauditService.destinationData(), download: () => NetauditService.downloadDestinationData(), remove: () => NetauditService.removeDestinationData() },
  places: { load: () => NetauditService.destinationPlaces(), download: () => NetauditService.downloadDestinationPlaces(), remove: () => NetauditService.removeDestinationPlaces() },
}

/** One destination table (the address owners, or the cities) as its card shows it. Every change answers with the whole new state, which replaces the old. */
export function useDestinationData(table: DestinationTable = 'owners') {
  const calls = CALLS[table]
  const state = ref<DestinationDataState | null>(null)
  const error = ref('')
  const busy = ref(false)

  async function load(): Promise<void> {
    try {
      state.value = await calls.load()
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
    download: () => run(calls.download, DESTINATION_DATA_DOWNLOAD_FAILED),
    remove: () => run(calls.remove, DESTINATION_DATA_REMOVE_FAILED),
  }
}
