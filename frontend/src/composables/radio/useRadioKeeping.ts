import { ref } from 'vue'
import { RadioService } from '../../services/radio/radioService'
import { errorMessage } from '../../utils/errors'
import { RADIO_FORGET_DONE, RADIO_KEEP_DROPPED, RADIO_KEEP_FAILED, RADIO_KEEP_TURNED_OFF } from '../../constants/radio'
import type { RadioKeeping } from '../../types/radio'

/** Radio · What is kept: how long signal tracks stay on disk (off by default), deleting them, and forgetting the neighbours. */
export function useRadioKeeping() {
  const keeping = ref<RadioKeeping | null>(null)
  const error = ref('')
  const notice = ref('')
  const busy = ref(false)

  async function run(work: () => Promise<RadioKeeping | void>, done = ''): Promise<void> {
    busy.value = true
    notice.value = ''
    try {
      const next = await work()
      keeping.value = next ?? (await RadioService.keeping())
      error.value = ''
      notice.value = done
    } catch (err) {
      error.value = errorMessage(err, RADIO_KEEP_FAILED)
    } finally {
      busy.value = false
    }
  }

  return {
    keeping, error, notice, busy,
    load: () => run(() => RadioService.keeping()),
    setHours: (hours: number) => run(() => RadioService.setKeeping(hours), hours === 0 ? RADIO_KEEP_TURNED_OFF : ''),
    dropTracks: () => run(() => RadioService.dropTracks(), RADIO_KEEP_DROPPED),
    forget: () => run(async () => { await RadioService.forgetNeighbours() }, RADIO_FORGET_DONE),
  }
}
