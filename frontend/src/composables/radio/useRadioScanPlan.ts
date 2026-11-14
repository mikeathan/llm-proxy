import { ref } from 'vue'
import { RadioService } from '../../services/radio/radioService'
import { errorMessage } from '../../utils/errors'
import { RADIO_SCAN_FAILED, RADIO_SCAN_SAVE_FAILED } from '../../constants/radio'
import type { RadioScan, RadioScanPlan } from '../../types/radio'

/** Radio · Scan plan: which channels a listening run visits, how long on each and how often. */
export function useRadioScanPlan() {
  const scan = ref<RadioScan | null>(null)
  const error = ref('')
  const busy = ref(false)
  const saved = ref(false)
  const running = ref(false)

  async function load(): Promise<void> {
    try {
      scan.value = await RadioService.scan()
      running.value = (await RadioService.listening()).listening
      error.value = ''
    } catch (err) {
      error.value = errorMessage(err, RADIO_SCAN_FAILED)
    }
  }

  async function save(plan: RadioScanPlan): Promise<void> {
    busy.value = true
    saved.value = false
    try {
      scan.value = await RadioService.saveScan(plan)
      error.value = ''
      saved.value = true
    } catch (err) {
      error.value = errorMessage(err, RADIO_SCAN_SAVE_FAILED)
    } finally {
      busy.value = false
    }
  }

  return { scan, error, busy, saved, running, load, save, reset: () => save({ channels: [], window_seconds: 0, cycle_seconds: 0 }) }
}
