import { ref } from 'vue'
import { RadioService } from '../../services/radio/radioService'
import { errorMessage } from '../../utils/errors'
import type { RadioView } from '../../types/radio'

const LOAD_FAILED = 'The radios could not be read.'

/** Radio · Sensors: the radios on this computer, read fresh each time (a look is a few small files, so a plugged-in adapter shows on the next one). */
export function useRadioSensors() {
  const view = ref<RadioView | null>(null)
  const loading = ref(false)
  const error = ref('')

  async function load(): Promise<void> {
    loading.value = true
    try {
      view.value = await RadioService.sensors()
      error.value = ''
    } catch (err) {
      error.value = errorMessage(err, LOAD_FAILED)
    } finally {
      loading.value = false
    }
  }

  return { view, loading, error, load }
}
