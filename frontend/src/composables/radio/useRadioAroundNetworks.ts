import { ref } from 'vue'
import { RadioService } from '../../services/radio/radioService'
import { errorMessage } from '../../utils/errors'
import { RADIO_LIVE_FAILED } from '../../constants/radio'
import type { RadioAroundNetworks } from '../../types/radio'

/** Radio · every router around the house with the devices that use it (read when the Networks view is open). */
export function useRadioAroundNetworks() {
  const view = ref<RadioAroundNetworks | null>(null)
  const error = ref('')
  async function load(): Promise<void> {
    try {
      view.value = await RadioService.aroundNetworks()
      error.value = ''
    } catch (err) {
      error.value = errorMessage(err, RADIO_LIVE_FAILED)
    }
  }
  return { view, error, load }
}
