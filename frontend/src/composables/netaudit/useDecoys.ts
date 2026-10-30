import { ref } from 'vue'
import { NetauditService } from '../../services/netaudit/netauditService'
import { errorMessage } from '../../utils/errors'
import { DECOYS_LOAD_FAILED, DECOYS_SAVE_FAILED } from '../../constants/netaudit'
import type { DecoyState } from '../../types/netaudit'

/** The decoy services as the settings card shows them. A change answers with the whole new state, which replaces the old. */
export function useDecoys() {
  const state = ref<DecoyState | null>(null)
  const error = ref('')
  const busy = ref(false)

  async function load(): Promise<void> {
    try {
      state.value = await NetauditService.decoys()
      error.value = ''
    } catch (err) {
      error.value = errorMessage(err, DECOYS_LOAD_FAILED)
    }
  }

  async function set(enabled: boolean): Promise<void> {
    busy.value = true
    try {
      state.value = await NetauditService.setDecoys(enabled)
      error.value = ''
    } catch (err) {
      error.value = errorMessage(err, DECOYS_SAVE_FAILED)
    } finally {
      busy.value = false
    }
  }

  return { state, error, busy, load, set }
}
