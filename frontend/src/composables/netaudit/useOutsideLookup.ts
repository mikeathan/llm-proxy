import { ref } from 'vue'
import { NetauditService } from '../../services/netaudit/netauditService'
import { errorMessage } from '../../utils/errors'
import { OUTSIDE_LOOKUP_LOAD_FAILED, OUTSIDE_LOOKUP_SAVE_FAILED } from '../../constants/netaudit'
import type { OutsideLookupState, OutsideLookupUpdate } from '../../types/netaudit'

/** The outside lookup as its card shows it. A change answers with the whole new state, which replaces the old. */
export function useOutsideLookup() {
  const state = ref<OutsideLookupState | null>(null)
  const error = ref('')
  const busy = ref(false)

  async function load(): Promise<void> {
    try {
      state.value = await NetauditService.outsideLookup()
      error.value = ''
    } catch (err) {
      error.value = errorMessage(err, OUTSIDE_LOOKUP_LOAD_FAILED)
    }
  }

  async function save(update: OutsideLookupUpdate): Promise<boolean> {
    busy.value = true
    try {
      state.value = await NetauditService.setOutsideLookup(update)
      error.value = ''
      return true
    } catch (err) {
      error.value = errorMessage(err, OUTSIDE_LOOKUP_SAVE_FAILED)
      return false
    } finally {
      busy.value = false
    }
  }

  return { state, error, busy, load, setUrl: (url: string) => save({ url }), setAddress: (address: string) => save({ address: address.trim() }) }
}
