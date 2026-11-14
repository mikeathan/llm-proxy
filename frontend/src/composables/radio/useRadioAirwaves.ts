import { ref } from 'vue'
import { RadioService } from '../../services/radio/radioService'
import { errorMessage } from '../../utils/errors'
import { RADIO_AIR_FAILED } from '../../constants/radio'
import type { RadioAirwaves } from '../../types/radio'

/** Radio · Airwaves: how channels and kinds of device use the air, as counts. */
export function useRadioAirwaves() {
  const view = ref<RadioAirwaves | null>(null)
  const error = ref('')

  async function load(): Promise<void> {
    try {
      view.value = await RadioService.airwaves()
      error.value = ''
    } catch (err) {
      error.value = errorMessage(err, RADIO_AIR_FAILED)
    }
  }

  return { view, error, load }
}
