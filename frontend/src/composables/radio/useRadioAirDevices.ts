import { ref } from 'vue'
import { RadioService } from '../../services/radio/radioService'
import { errorMessage } from '../../utils/errors'
import { RADIO_NEIGH_FAILED } from '../../constants/radio'
import type { RadioAirDevices, RadioAirSort, RadioClass } from '../../types/radio'

/** Radio · Neighbourhood: the devices of one kind (or all of them), as short ids with words and counts. */
export function useRadioAirDevices() {
  const list = ref<RadioAirDevices | null>(null)
  const error = ref('')
  const busy = ref(false)

  async function load(cls?: RadioClass, sort?: RadioAirSort): Promise<void> {
    busy.value = true
    try {
      list.value = await RadioService.airDevices(cls, sort)
      error.value = ''
    } catch (err) {
      error.value = errorMessage(err, RADIO_NEIGH_FAILED)
    } finally {
      busy.value = false
    }
  }

  return { list, error, busy, load }
}
