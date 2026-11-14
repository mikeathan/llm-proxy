import { ref } from 'vue'
import { RadioService } from '../../services/radio/radioService'
import { errorMessage } from '../../utils/errors'
import type { RadioAir } from '../../types/radio'

const LOAD_FAILED = 'What is around you could not be read.'

/** Radio · Around you: other people's devices as counts by class, and how the sorting rules stand. */
export function useRadioAir() {
  const air = ref<RadioAir | null>(null)
  const error = ref('')

  async function load(): Promise<void> {
    try {
      air.value = await RadioService.air()
      error.value = ''
    } catch (err) {
      error.value = errorMessage(err, LOAD_FAILED)
    }
  }

  return { air, error, load }
}
