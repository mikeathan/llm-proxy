import { ref } from 'vue'
import { RadioService } from '../../services/radio/radioService'
import { errorMessage } from '../../utils/errors'
import { RADIO_LIVE_FAILED, RADIO_CALIBRATE_FAILED } from '../../constants/radio'
import type { RadioLive } from '../../types/radio'

/** Radio · the radar: what the sensor hears now, and the one-metre calibration. */
export function useRadioLive() {
  const view = ref<RadioLive | null>(null)
  const error = ref('')
  const busy = ref(false)

  async function load(): Promise<void> {
    try {
      view.value = await RadioService.live()
      error.value = ''
    } catch (err) {
      error.value = errorMessage(err, RADIO_LIVE_FAILED)
    }
  }

  async function calibrate(id: string): Promise<void> {
    busy.value = true
    try {
      await RadioService.calibrate(id)
      error.value = ''
      await load()
    } catch (err) {
      error.value = errorMessage(err, RADIO_CALIBRATE_FAILED)
    } finally {
      busy.value = false
    }
  }

  return { view, error, busy, load, calibrate }
}
